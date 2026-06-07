package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsLoopbackHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1:4773", true},
		{"127.0.0.1", true},
		{"localhost:4773", true},
		{"LocalHost:4773", true},
		{"[::1]:4773", true},
		{"[::1]", true},
		{"", false},
		{"example.com", false},
		{"evil.com:4773", false},
		{"169.254.169.254", false}, // cloud metadata — link-local, not loopback
		{"10.0.0.5:80", false},
		{"192.168.1.10:4773", false},
	}
	for _, c := range cases {
		if got := isLoopbackHost(c.host); got != c.want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

// TestGuardRejectsRebindHost asserts a non-loopback Host (the DNS-rebinding case)
// is refused even on an otherwise valid GET.
func TestGuardRejectsRebindHost(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	req.Host = "anime-tools.example.com" // rebinds to 127.0.0.1 in the victim's browser
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("rebind host: status = %d, want 403", rec.Code)
	}
}

// TestGuardRejectsCrossOriginMutation asserts a cross-site Origin on a
// state-changing request (the CSRF case) is refused.
func TestGuardRejectsCrossOriginMutation(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/extension-repos",
		strings.NewReader(`{"name":"x","url":"http://evil/index.json"}`))
	req.Host = loopbackHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example") // attacker page
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST: status = %d, want 403", rec.Code)
	}
}

// TestGuardAllowsSameOriginMutation asserts a same-origin (loopback) Origin on a
// state-changing request passes the guard (it may then fail validation, but the
// guard itself must not 403 it).
func TestGuardAllowsSameOriginMutation(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/extension-repos",
		strings.NewReader(`{"name":"local","url":"http://example.org/index.json"}`))
	req.Host = loopbackHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:4773")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("same-origin POST was rejected by the guard (status 403)")
	}
}
