// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProxmoxBackupServerClientAuthenticatesAndDecodesData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/access/ticket" {
			if got, want := r.FormValue("username"), "root@pam"; got != want {
				t.Fatalf("unexpected username: got %q, want %q", got, want)
			}
			if got, want := r.FormValue("password"), "secret"; got != want {
				t.Fatalf("unexpected password: got %q, want %q", got, want)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"ticket":"ticket-value","CSRFPreventionToken":"csrf-value"}}`))
			return
		}

		if r.URL.Path != "/api2/json/config/s3/test" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		cookie, err := r.Cookie("PBSAuthCookie")
		if err != nil {
			t.Fatalf("missing auth cookie: %s", err)
		}
		if got, want := cookie.Value, "ticket-value"; got != want {
			t.Fatalf("unexpected auth cookie: got %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"test","access-key":"access","endpoint":"s3.example.com"}}`))
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "secret", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	var data s3ConfigAPIModel
	if err := client.get(context.Background(), "/config/s3/test", &data); err != nil {
		t.Fatalf("get returned error: %s", err)
	}
	if got, want := data.Endpoint, "s3.example.com"; got != want {
		t.Fatalf("unexpected endpoint: got %q, want %q", got, want)
	}
}

func TestProxmoxBackupServerClientTrimsAPIPathFromEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"ticket":"ticket-value","CSRFPreventionToken":"csrf-value"}}`))
		case "/api2/json/config/s3/test":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"test","access-key":"access","endpoint":"s3.example.com"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL+"/api2/json", "root@pam", "secret", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	var data s3ConfigAPIModel
	if err := client.get(context.Background(), "/config/s3/test", &data); err != nil {
		t.Fatalf("get returned error: %s", err)
	}
}

func TestNewProxmoxBackupServerClientRejectsInvalidEndpoints(t *testing.T) {
	tests := map[string]string{
		"ftp://backup.example.com":                "endpoint must use http or https",
		"https:///api2/json":                      "endpoint must include a host",
		"https://backup.example.com:8007?debug=1": "endpoint must not include query parameters or fragments",
		"https://backup.example.com:8007#debug":   "endpoint must not include query parameters or fragments",
	}

	for endpoint, wantErr := range tests {
		t.Run(endpoint, func(t *testing.T) {
			_, err := newProxmoxBackupServerClient(endpoint, "root@pam", "secret", false)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); got != wantErr {
				t.Fatalf("unexpected error: got %q, want %q", got, wantErr)
			}
		})
	}
}

func TestNewProxmoxBackupServerClientAllowsReverseProxyBasePath(t *testing.T) {
	client, err := newProxmoxBackupServerClient("https://proxy.example.com/backup-server", "root@pam", "secret", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	if got, want := client.endpoint, "https://proxy.example.com/backup-server"; got != want {
		t.Fatalf("unexpected endpoint: got %q, want %q", got, want)
	}
}

func TestProxmoxBackupServerClientReadsCertificateFingerprint(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "secret", true)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	fingerprint, err := client.certificateFingerprint(context.Background())
	if err != nil {
		t.Fatalf("certificateFingerprint returned error: %s", err)
	}
	if got, want := fingerprint, sha256Fingerprint(server.Certificate().Raw); got != want {
		t.Fatalf("unexpected fingerprint: got %q, want %q", got, want)
	}
}

func TestProxmoxBackupServerClientRejectsHTTPFingerprintEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "secret", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	_, err = client.certificateFingerprint(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "endpoint must use https to read certificate fingerprint"; got != want {
		t.Fatalf("unexpected error: got %q, want %q", got, want)
	}
}

func TestProxmoxBackupServerClientSendsCSRFPreventionTokenForWrites(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/access/ticket" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"ticket":"ticket-value","CSRFPreventionToken":"csrf-value"}}`))
			return
		}

		if r.URL.Path != "/api2/json/config/s3" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got, want := r.Header.Get("CSRFPreventionToken"), "csrf-value"; got != want {
			t.Fatalf("unexpected CSRF token: got %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "secret", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	if err := client.post(context.Background(), "/config/s3", s3ConfigAPIModel{ID: "test"}, nil); err != nil {
		t.Fatalf("post returned error: %s", err)
	}
}

func TestProxmoxBackupServerClientWaitTaskPollsUntilOK(t *testing.T) {
	upid := "UPID:pbs-01:00000001:00000002:00000003:00000004:create-datastore:pbs:root@pam:"
	statusCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"ticket":"ticket-value","CSRFPreventionToken":"csrf-value"}}`))
		case "/api2/json/nodes/pbs-01/tasks/" + upid + "/status":
			statusCalls++
			w.Header().Set("Content-Type", "application/json")
			if statusCalls == 1 {
				_, _ = w.Write([]byte(`{"data":{"status":"running"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "secret", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	if err := client.waitTaskWithInterval(context.Background(), upid, time.Second, time.Millisecond); err != nil {
		t.Fatalf("waitTaskWithInterval returned error: %s", err)
	}
	if statusCalls != 2 {
		t.Fatalf("unexpected status calls: got %d, want 2", statusCalls)
	}
}

func TestProxmoxBackupServerClientWaitTaskIncludesFailureLog(t *testing.T) {
	upid := "UPID:pbs-01:00000001:00000002:00000003:00000004:create-datastore:pbs:root@pam:"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"ticket":"ticket-value","CSRFPreventionToken":"csrf-value"}}`))
		case "/api2/json/nodes/pbs-01/tasks/" + upid + "/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"Error: create failed"}}`))
		case "/api2/json/nodes/pbs-01/tasks/" + upid + "/log":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"n":1,"t":"creating datastore pbs"},{"n":2,"t":"permission denied"}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "secret", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	err = client.waitTaskWithInterval(context.Background(), upid, time.Second, time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"Error: create failed", "creating datastore pbs", "permission denied"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %q", want, err.Error())
		}
	}
}

func TestProxmoxBackupServerClientConfigMutationSerializesCallbacks(t *testing.T) {
	client := &proxmoxBackupServerClient{}
	var active atomic.Int32
	var overlap atomic.Bool
	var callbackErrors atomic.Int32

	enter := func() func() {
		if active.Add(1) != 1 {
			overlap.Store(true)
		}
		return func() { active.Add(-1) }
	}

	const mutationCount = 8
	var waitGroup sync.WaitGroup
	waitGroup.Add(mutationCount)
	for i := 0; i < mutationCount; i++ {
		go func() {
			defer waitGroup.Done()

			err := client.withConfigMutation(context.Background(), func(context.Context) (*string, error) {
				leave := enter()
				defer leave()
				time.Sleep(time.Millisecond)
				digest := "fresh-digest"
				return &digest, nil
			}, func(context.Context, *string) error {
				leave := enter()
				defer leave()
				time.Sleep(time.Millisecond)
				return nil
			})
			if err != nil {
				callbackErrors.Add(1)
			}
		}()
	}
	waitGroup.Wait()

	if callbackErrors.Load() != 0 {
		t.Fatalf("unexpected callback errors: %d", callbackErrors.Load())
	}
	if overlap.Load() {
		t.Fatal("configuration mutation callbacks overlapped")
	}
}

func TestProxmoxBackupServerClientConfigMutationPropagatesErrorsAndReleasesLock(t *testing.T) {
	client := &proxmoxBackupServerClient{}
	readErr := errors.New("read failed")
	mutationErr := errors.New("mutation failed")

	mutated := false
	err := client.withConfigMutation(context.Background(), func(context.Context) (*string, error) {
		return nil, readErr
	}, func(context.Context, *string) error {
		mutated = true
		return nil
	})
	if !errors.Is(err, readErr) {
		t.Fatalf("unexpected read error: got %v, want %v", err, readErr)
	}
	if mutated {
		t.Fatal("mutation callback ran after read failure")
	}

	err = client.withConfigMutation(context.Background(), func(context.Context) (*string, error) {
		digest := "digest"
		return &digest, nil
	}, func(context.Context, *string) error {
		return mutationErr
	})
	if !errors.Is(err, mutationErr) {
		t.Fatalf("unexpected mutation error: got %v, want %v", err, mutationErr)
	}

	if err := client.withConfigMutation(context.Background(), func(context.Context) (*string, error) {
		digest := "digest-after-error"
		return &digest, nil
	}, func(context.Context, *string) error { return nil }); err != nil {
		t.Fatalf("configuration mutation lock was not released after error: %v", err)
	}
}

func TestProxmoxBackupServerClientConfigMutationReleasesLockAfterPanic(t *testing.T) {
	client := &proxmoxBackupServerClient{}
	func() {
		defer func() {
			if got := recover(); got != "callback panic" {
				t.Fatalf("unexpected panic: got %v", got)
			}
		}()

		_ = client.withConfigMutation(context.Background(), func(context.Context) (*string, error) {
			panic("callback panic")
		}, func(context.Context, *string) error { return nil })
	}()

	if err := client.withConfigMutation(context.Background(), func(context.Context) (*string, error) {
		digest := "digest-after-panic"
		return &digest, nil
	}, func(context.Context, *string) error { return nil }); err != nil {
		t.Fatalf("configuration mutation lock was not released after panic: %v", err)
	}
}

func TestProxmoxBackupServerClientConfigMutationPassesFreshDigestToMutation(t *testing.T) {
	client := &proxmoxBackupServerClient{}
	digests := []string{"digest-1", "digest-2"}
	var readCalls int
	var received []string

	for _, wantDigest := range digests {
		err := client.withConfigMutation(context.Background(), func(context.Context) (*string, error) {
			digest := digests[readCalls]
			readCalls++
			return &digest, nil
		}, func(_ context.Context, digest *string) error {
			if digest == nil {
				t.Fatal("mutation received nil digest")
			}
			received = append(received, *digest)
			return nil
		})
		if err != nil {
			t.Fatalf("withConfigMutation returned error: %v", err)
		}
		if got := received[len(received)-1]; got != wantDigest {
			t.Fatalf("unexpected digest handoff: got %q, want %q", got, wantDigest)
		}
	}

	if got, want := readCalls, len(digests); got != want {
		t.Fatalf("unexpected read callback count: got %d, want %d", got, want)
	}
}

func TestProxmoxBackupServerClientPutAndDeleteWithResponseDecodeData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("CSRFPreventionToken"), "csrf-value"; got != want {
			t.Fatalf("unexpected CSRF token: got %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/config/test":
			_, _ = w.Write([]byte(`{"data":{"id":"put-result"},"digest":"put-digest"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/config/test":
			_, _ = w.Write([]byte(`{"data":{"id":"delete-result"},"digest":"delete-digest"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client := &proxmoxBackupServerClient{
		endpoint:            server.URL,
		httpClient:          server.Client(),
		authCookie:          &http.Cookie{Name: "PBSAuthCookie", Value: "ticket-value"},
		csrfPreventionToken: "csrf-value",
	}

	var putResponse struct {
		ID string `json:"id"`
	}
	if err := client.putWithResponse(context.Background(), "/config/test", map[string]string{"id": "test"}, &putResponse); err != nil {
		t.Fatalf("putWithResponse returned error: %v", err)
	}
	if got, want := putResponse.ID, "put-result"; got != want {
		t.Fatalf("unexpected PUT response: got %q, want %q", got, want)
	}

	var deleteResponse struct {
		ID string `json:"id"`
	}
	if err := client.deleteWithResponse(context.Background(), "/config/test", &deleteResponse); err != nil {
		t.Fatalf("deleteWithResponse returned error: %v", err)
	}
	if got, want := deleteResponse.ID, "delete-result"; got != want {
		t.Fatalf("unexpected DELETE response: got %q, want %q", got, want)
	}
}

func TestProxmoxBackupServerClientGetWithDigestDecodesTopLevelDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api2/json/config/test" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"test"},"digest":"fresh-digest"}`))
	}))
	t.Cleanup(server.Close)

	client := &proxmoxBackupServerClient{
		endpoint:            server.URL,
		httpClient:          server.Client(),
		authCookie:          &http.Cookie{Name: "PBSAuthCookie", Value: "ticket-value"},
		csrfPreventionToken: "csrf-value",
	}

	var response struct {
		ID string `json:"id"`
	}
	digest, err := client.getWithDigest(context.Background(), "/config/test", &response)
	if err != nil {
		t.Fatalf("getWithDigest returned error: %v", err)
	}
	if digest == nil || *digest != "fresh-digest" {
		t.Fatalf("unexpected response digest: %v", digest)
	}
	if got, want := response.ID, "test"; got != want {
		t.Fatalf("unexpected response data: got %q, want %q", got, want)
	}
}
