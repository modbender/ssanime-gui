package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/events"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	hub := events.NewHub(nil)
	hub.Start()
	t.Cleanup(hub.Stop)
	// store is nil: the settings route isn't exercised here; the routes that
	// are exercised don't touch it.
	return New(nil, hub, nil, Config{})
}

func TestHealthz(t *testing.T) {
	srv := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp Response[map[string]string]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != "" || resp.Data == nil || (*resp.Data)["status"] != "ok" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestPing(t *testing.T) {
	srv := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "pong") {
		t.Fatalf("expected pong, got %s", rec.Body.String())
	}
}

func TestSPAFallback(t *testing.T) {
	srv := newTestHandler(t)

	// Root serves the embedded SPA index.html.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("root: status %d body %s", rec.Code, rec.Body.String())
	}
	root := rec.Body.String()

	// Unknown client-route path falls back to the same index.html (HTML5 history routing).
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/library/series/42", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != root {
		t.Fatalf("fallback: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestEventsSSEHeartbeat(t *testing.T) {
	hub := events.NewHub(nil, events.WithHeartbeat(10*time.Millisecond))
	hub.Start()
	defer hub.Stop()
	srv := New(nil, hub, nil, Config{})

	ts := httptest.NewServer(srv)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	sc := bufio.NewScanner(resp.Body)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !sc.Scan() {
			break
		}
		if strings.Contains(sc.Text(), string(events.TypeHeartbeat)) {
			return
		}
	}
	t.Fatal("did not receive heartbeat frame")
}
