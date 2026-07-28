// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type proxmoxBackupServerClient struct {
	endpoint   string
	username   string
	password   string
	httpClient *http.Client

	authMu              sync.Mutex
	authCookie          *http.Cookie
	csrfPreventionToken string
}

type proxmoxBackupServerResponse struct {
	Data   json.RawMessage `json:"data"`
	Digest *string         `json:"digest"`
}

type proxmoxBackupServerTicketResponse struct {
	Ticket              string `json:"ticket"`
	CSRFPreventionToken string `json:"CSRFPreventionToken"`
}

type proxmoxBackupServerTaskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
}

type proxmoxBackupServerTaskLogLine struct {
	Line string `json:"t"`
}

type proxmoxBackupServerAPIError struct {
	method string
	path   string
	status string
	code   int
	body   string
}

const (
	proxmoxBackupServerTaskWaitTimeout  = 10 * time.Minute
	proxmoxBackupServerTaskWaitInterval = 2 * time.Second
)

func (e *proxmoxBackupServerAPIError) Error() string {
	return fmt.Sprintf("%s %s failed with %s: %s", e.method, e.path, e.status, e.body)
}

func (e *proxmoxBackupServerAPIError) notFound() bool {
	if e.code == http.StatusNotFound {
		return true
	}

	return e.code == http.StatusBadRequest && strings.Contains(strings.ToLower(e.body), "no such ")
}

func newProxmoxBackupServerClient(endpoint, username, password string, insecureTLS bool) (*proxmoxBackupServerClient, error) {
	endpoint = strings.TrimRight(endpoint, "/")
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("endpoint must use http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("endpoint must include a host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("endpoint must not include query parameters or fragments")
	}
	if strings.HasSuffix(parsed.Path, "/api2/json") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/api2/json")
		endpoint = strings.TrimRight(parsed.String(), "/")
	}

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("unexpected default transport type %T", http.DefaultTransport)
	}
	transport := defaultTransport.Clone()
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // User opt-in for self-signed Proxmox Backup Server certificates.
	}

	return &proxmoxBackupServerClient{
		endpoint: endpoint,
		username: username,
		password: password,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}, nil
}

func (c *proxmoxBackupServerClient) authenticate(ctx context.Context) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	if c.authCookie != nil && c.csrfPreventionToken != "" {
		return nil
	}

	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/api2/json/access/ticket", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send auth request: %w", err)
	}
	defer resp.Body.Close()

	var data proxmoxBackupServerTicketResponse
	if err := decodeProxmoxBackupServerResponse(resp, &data); err != nil {
		return err
	}
	if data.Ticket == "" || data.CSRFPreventionToken == "" {
		return fmt.Errorf("auth response missing ticket or CSRF prevention token")
	}

	c.authCookie = &http.Cookie{Name: "PBSAuthCookie", Value: data.Ticket}
	c.csrfPreventionToken = data.CSRFPreventionToken

	return nil
}

func (c *proxmoxBackupServerClient) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *proxmoxBackupServerClient) post(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

func (c *proxmoxBackupServerClient) put(ctx context.Context, path string, body any) error {
	return c.do(ctx, http.MethodPut, path, body, nil)
}

func (c *proxmoxBackupServerClient) postForm(ctx context.Context, path string, form url.Values, out any) error {
	return c.doForm(ctx, http.MethodPost, path, form, out)
}

func (c *proxmoxBackupServerClient) putForm(ctx context.Context, path string, form url.Values) error {
	return c.doForm(ctx, http.MethodPut, path, form, nil)
}

func (c *proxmoxBackupServerClient) deleteForm(ctx context.Context, path string, form url.Values) error {
	return c.doForm(ctx, http.MethodDelete, path, form, nil)
}

func (c *proxmoxBackupServerClient) delete(ctx context.Context, path string) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *proxmoxBackupServerClient) getWithDigest(ctx context.Context, path string, out any) (*string, error) {
	return c.doWithDigest(ctx, http.MethodGet, path, out)
}

func (c *proxmoxBackupServerClient) certificateFingerprint(ctx context.Context) (string, error) {
	parsed, err := url.Parse(c.endpoint)
	if err != nil {
		return "", fmt.Errorf("parse endpoint: %w", err)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("endpoint must use https to read certificate fingerprint")
	}

	address := parsed.Host
	if parsed.Port() == "" {
		address = net.JoinHostPort(parsed.Hostname(), "443")
	}

	dialer := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 30 * time.Second},
		Config: &tls.Config{
			ServerName:         parsed.Hostname(),
			InsecureSkipVerify: true, //nolint:gosec // The fingerprint data source intentionally reads untrusted self-signed certificates.
		},
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return "", fmt.Errorf("connect to endpoint: %w", err)
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return "", fmt.Errorf("unexpected connection type %T", conn)
	}
	certificates := tlsConn.ConnectionState().PeerCertificates
	if len(certificates) == 0 {
		return "", fmt.Errorf("endpoint did not present a certificate")
	}

	return sha256Fingerprint(certificates[0].Raw), nil
}

func (c *proxmoxBackupServerClient) waitTask(ctx context.Context, upid string) error {
	return c.waitTaskWithInterval(ctx, upid, proxmoxBackupServerTaskWaitTimeout, proxmoxBackupServerTaskWaitInterval)
}

func (c *proxmoxBackupServerClient) waitTaskWithInterval(ctx context.Context, upid string, timeout, interval time.Duration) error {
	node, err := taskNode(upid)
	if err != nil {
		return err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		var status proxmoxBackupServerTaskStatus
		if err := c.get(ctx, "/nodes/"+urlPathEscape(node)+"/tasks/"+urlPathEscape(upid)+"/status", &status); err != nil {
			return fmt.Errorf("read task status: %w", err)
		}

		if status.Status == "stopped" {
			if status.ExitStatus == "OK" {
				return nil
			}
			return c.taskError(ctx, node, upid, status.ExitStatus)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("timed out waiting for task %s", upid)
		case <-ticker.C:
		}
	}
}

func (c *proxmoxBackupServerClient) taskError(ctx context.Context, node, upid, exitStatus string) error {
	if exitStatus == "" {
		exitStatus = "unknown"
	}

	log, err := c.taskLog(ctx, node, upid)
	if err != nil {
		return fmt.Errorf("task %s failed with exit status %q; failed to read task log: %w", upid, exitStatus, err)
	}
	if log == "" {
		return fmt.Errorf("task %s failed with exit status %q", upid, exitStatus)
	}

	return fmt.Errorf("task %s failed with exit status %q:\n%s", upid, exitStatus, log)
}

func (c *proxmoxBackupServerClient) taskLog(ctx context.Context, node, upid string) (string, error) {
	var logLines []proxmoxBackupServerTaskLogLine
	if err := c.get(ctx, "/nodes/"+urlPathEscape(node)+"/tasks/"+urlPathEscape(upid)+"/log?start=0&limit=0", &logLines); err != nil {
		return "", err
	}

	lines := make([]string, 0, len(logLines))
	for _, logLine := range logLines {
		if logLine.Line != "" {
			lines = append(lines, logLine.Line)
		}
	}

	return strings.Join(lines, "\n"), nil
}

func taskNode(upid string) (string, error) {
	parts := strings.Split(upid, ":")
	if len(parts) < 3 || parts[0] != "UPID" || parts[1] == "" {
		return "", fmt.Errorf("invalid task UPID %q", upid)
	}

	return parts[1], nil
}

func sha256Fingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}

	return strings.Join(parts, ":")
}

func (c *proxmoxBackupServerClient) do(ctx context.Context, method, path string, body any, out any) error {
	if err := c.authenticate(ctx); err != nil {
		return err
	}

	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		requestBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+"/api2/json"+path, requestBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.AddCookie(c.authCookie)
	if method != http.MethodGet {
		req.Header.Set("CSRFPreventionToken", c.csrfPreventionToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	return decodeProxmoxBackupServerResponse(resp, out)
}

func (c *proxmoxBackupServerClient) doWithDigest(ctx context.Context, method, path string, out any) (*string, error) {
	if err := c.authenticate(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+"/api2/json"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.AddCookie(c.authCookie)
	if method != http.MethodGet {
		req.Header.Set("CSRFPreventionToken", c.csrfPreventionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	return decodeProxmoxBackupServerResponseWithDigest(resp, out)
}

func (c *proxmoxBackupServerClient) doForm(ctx context.Context, method, path string, form url.Values, out any) error {
	if err := c.authenticate(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+"/api2/json"+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.AddCookie(c.authCookie)
	if method != http.MethodGet {
		req.Header.Set("CSRFPreventionToken", c.csrfPreventionToken)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	return decodeProxmoxBackupServerResponse(resp, out)
}

func decodeProxmoxBackupServerResponse(resp *http.Response, out any) error {
	_, err := decodeProxmoxBackupServerResponseWithDigest(resp, out)
	return err
}

func decodeProxmoxBackupServerResponseWithDigest(resp *http.Response, out any) (*string, error) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &proxmoxBackupServerAPIError{
			method: resp.Request.Method,
			path:   resp.Request.URL.Path,
			status: resp.Status,
			code:   resp.StatusCode,
			body:   strings.TrimSpace(string(responseBody)),
		}
	}
	if out == nil || len(responseBody) == 0 {
		return nil, nil
	}

	var wrapped proxmoxBackupServerResponse
	if err := json.Unmarshal(responseBody, &wrapped); err != nil {
		return nil, fmt.Errorf("decode response wrapper: %w", err)
	}
	if len(wrapped.Data) == 0 || string(wrapped.Data) == "null" {
		return wrapped.Digest, nil
	}
	if err := json.Unmarshal(wrapped.Data, out); err != nil {
		return nil, fmt.Errorf("decode response data: %w", err)
	}

	return wrapped.Digest, nil
}
