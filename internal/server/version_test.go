package server

import (
	"net/http"
	"testing"

	"github.com/modbender/ssanime-gui/internal/version"
)

// TestVersionEndpoint asserts /api/version returns the injected version package
// vars verbatim through the standard envelope.
func TestVersionEndpoint(t *testing.T) {
	prevVer, prevCommit := version.Version, version.Commit
	t.Cleanup(func() { version.Version, version.Commit = prevVer, prevCommit })

	version.Version = "v1.2.3-test"
	version.Commit = "a1b2c3d"

	srv := newTestServer(t)
	rec := getJSON(t, srv, "/api/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[VersionResponse](t, rec)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Data == nil {
		t.Fatal("data is nil")
	}
	if resp.Data.Version != "v1.2.3-test" {
		t.Errorf("version = %q, want %q", resp.Data.Version, "v1.2.3-test")
	}
	if resp.Data.Commit != "a1b2c3d" {
		t.Errorf("commit = %q, want %q", resp.Data.Commit, "a1b2c3d")
	}
}

// TestVersionDefaults confirms the package defaults to "dev" with no commit when
// nothing is injected.
func TestVersionDefaults(t *testing.T) {
	if version.Version != "dev" {
		t.Skipf("version injected (%q); default check not applicable", version.Version)
	}
	if version.Commit != "" {
		t.Errorf("default Commit = %q, want empty", version.Commit)
	}
}
