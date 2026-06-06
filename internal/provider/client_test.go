// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
