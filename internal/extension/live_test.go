//go:build live

// Live mechanics test: loads the real Hayase nyaa.js from GitHub and invokes
// search. Run with: go test -tags live ./internal/extension/ -v -run TestLive
// Network access is required. The dead Vercel proxy will return no results but
// the LOADING + execution path must succeed.
package extension

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/source"
)

func TestLiveHayaseNyaaLoad(t *testing.T) {
	const nyaaJSURL = "https://raw.githubusercontent.com/ReWelp/HayasexShiru-Extensions/main/hayase/nyaa.js"

	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Download the real nyaa.js.
	resp, err := client.Get(nyaaJSURL)
	if err != nil {
		t.Skipf("network unavailable: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read nyaa.js: %v", err)
	}
	payload := string(body)
	t.Logf("downloaded nyaa.js: %d bytes", len(payload))
	t.Logf("first 200 chars:\n%s", payload[:min(200, len(payload))])

	// 2. Compile and load through goja.
	p, err := NewJSProvider("hayase.extension.nyaa", "Nyaa", payload, client, testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProvider: %v", err)
	}
	t.Log("goja: extension loaded and compiled successfully")

	// 3. Invoke search — the dead proxy will return an error or empty array;
	//    both are acceptable. What must NOT happen is a panic or type error.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := p.Search(ctx, source.SearchOptions{
		Media: source.Media{RomajiTitle: "One Piece"},
		Query: "One Piece",
	})

	if err != nil {
		// Dead proxy failure is expected; log it and pass.
		t.Logf("search returned error (expected if proxy is dead): %v", err)
	} else {
		t.Logf("search returned %d results (dead proxy may return 0)", len(results))
		for i, r := range results {
			if i >= 3 {
				break
			}
			t.Logf("  [%d] name=%q seeders=%d magnet=%q", i, r.Name, r.Seeders, r.Magnet)
		}
	}

	fmt.Printf("\n=== LIVE MECHANICS SUMMARY ===\n")
	fmt.Printf("Extension loaded via goja: YES\n")
	fmt.Printf("Execution path ran:        YES\n")
	if err != nil {
		fmt.Printf("Search result:             ERROR (expected — dead proxy): %v\n", err)
	} else {
		fmt.Printf("Search result:             %d torrents (0 expected — dead proxy)\n", len(results))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
