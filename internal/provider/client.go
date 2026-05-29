// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
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
	Data json.RawMessage `json:"data"`
}

type proxmoxBackupServerTicketResponse struct {
	Ticket              string `json:"ticket"`
	CSRFPreventionToken string `json:"CSRFPreventionToken"`
}

type proxmoxBackupServerAPIError struct {
	method string
	path   string
	status string
	code   int
	body   string
}

func (e *proxmoxBackupServerAPIError) Error() string {
	return fmt.Sprintf("%s %s failed with %s: %s", e.method, e.path, e.status, e.body)
}

func (e *proxmoxBackupServerAPIError) notFound() bool {
	return e.code == http.StatusNotFound
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

func (c *proxmoxBackupServerClient) delete(ctx context.Context, path string) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
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

func decodeProxmoxBackupServerResponse(resp *http.Response, out any) error {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &proxmoxBackupServerAPIError{
			method: resp.Request.Method,
			path:   resp.Request.URL.Path,
			status: resp.Status,
			code:   resp.StatusCode,
			body:   strings.TrimSpace(string(responseBody)),
		}
	}
	if out == nil || len(responseBody) == 0 {
		return nil
	}

	var wrapped proxmoxBackupServerResponse
	if err := json.Unmarshal(responseBody, &wrapped); err != nil {
		return fmt.Errorf("decode response wrapper: %w", err)
	}
	if len(wrapped.Data) == 0 || string(wrapped.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(wrapped.Data, out); err != nil {
		return fmt.Errorf("decode response data: %w", err)
	}

	return nil
}
