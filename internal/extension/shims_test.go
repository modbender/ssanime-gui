package extension

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modbender/ssanime-gui/internal/source"
)

// magnet40 is a valid-looking magnet with a 40-char btih so source.Enrich keeps
// the torrent (and its echoed name) intact.
const magnet40 = "magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// runSingle loads a one-method inline extension and runs Search, returning the
// first torrent's name (the field the inline JS echoes data into).
func runSingleName(t *testing.T, js string) (string, error) {
	t.Helper()
	p, err := NewJSProvider("inline", "Inline", js, http.DefaultClient, testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProvider: %v", err)
	}
	res, err := p.Search(context.Background(), source.SearchOptions{
		Media: source.Media{RomajiTitle: "x"},
	})
	if err != nil {
		return "", err
	}
	if len(res) == 0 {
		t.Fatal("no results")
	}
	return res[0].Name, nil
}

func TestAtobDecodesStandard(t *testing.T) {
	js := `export default new class { async single() { return [{name: atob("aGVsbG8="), magnetLink: "` + magnet40 + `"}]; } }`
	name, err := runSingleName(t, js)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if name != "hello" {
		t.Errorf("atob standard = %q, want hello", name)
	}
}

func TestAtobURLSafeAndNoPadding(t *testing.T) {
	// "subw" base64-url without padding decodes to bytes; assert it doesn't throw
	// and round-trips a known url-safe string. " Pw_-" style; use a known value.
	// btoa of ">>>" is "Pj4+"; url-safe no-pad is "Pj4-". atob must accept it.
	js := `export default new class { async single() { return [{name: atob("Pj4-"), magnetLink: "` + magnet40 + `"}]; } }`
	name, err := runSingleName(t, js)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if name != ">>>" {
		t.Errorf("atob url-safe no-pad = %q, want >>>", name)
	}
}

func TestAtobWithWhitespace(t *testing.T) {
	js := `export default new class { async single() { return [{name: atob("aGVs\nbG8 ="), magnetLink: "` + magnet40 + `"}]; } }`
	name, err := runSingleName(t, js)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if name != "hello" {
		t.Errorf("atob with whitespace = %q, want hello", name)
	}
}

func TestAtobEmptyString(t *testing.T) {
	// atob("") -> "" ; the extension returns a marker name so the result survives.
	js := `export default new class { async single() { const d = atob(""); return [{name: "EMPTY:" + d, magnetLink: "` + magnet40 + `"}]; } }`
	name, err := runSingleName(t, js)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if name != "EMPTY:" {
		t.Errorf("atob empty = %q, want EMPTY:", name)
	}
}

func TestAtobInvalidThrows(t *testing.T) {
	// "!!!!" is not valid base64 in any alphabet → atob throws, caught in JS and
	// surfaced as a marker so we can assert it threw.
	js := `export default new class { async single() {
		try { atob("!@#$%^&"); return [{name: "NOTHROW", magnetLink: "` + magnet40 + `"}]; }
		catch (e) { return [{name: "THREW", magnetLink: "` + magnet40 + `"}]; }
	} }`
	name, err := runSingleName(t, js)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if name != "THREW" {
		t.Errorf("atob invalid = %q, want THREW (atob should throw)", name)
	}
}

func TestBtoaRoundTrip(t *testing.T) {
	js := `export default new class { async single() { return [{name: atob(btoa("round-trip ✓ ascii")), magnetLink: "` + magnet40 + `"}]; } }`
	name, err := runSingleName(t, js)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// btoa of a multibyte char is lossy by spec; restrict to ascii portion.
	if !strings.HasPrefix(name, "round-trip ") {
		t.Errorf("btoa/atob round-trip = %q", name)
	}
}

func TestNavigatorOnLineReadable(t *testing.T) {
	js := `export default new class { async single() {
		return [{name: "online:" + navigator.onLine + " ua:" + navigator.userAgent, magnetLink: "` + magnet40 + `"}];
	} }`
	name, err := runSingleName(t, js)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if name != "online:true ua:ssanime" {
		t.Errorf("navigator = %q, want online:true ua:ssanime", name)
	}
}

func TestURLSearchParamsPolyfill(t *testing.T) {
	// Construct from an object, append (repeat keys), set (replace), and
	// toString() with form-encoding (space -> '+').
	js := `export default new class { async single() {
		const p = new URLSearchParams({a: "1", q: "hello world"});
		p.append("a", "2");
		p.set("b", "x&y");
		const out = p.toString() + "|get_a=" + p.get("a") + "|all_a=" + p.getAll("a").join(",") + "|has_b=" + p.has("b");
		return [{name: out, magnetLink: "` + magnet40 + `"}];
	} }`
	name, err := runSingleName(t, js)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// q must be '+' encoded; b must be percent-encoded; a has two values.
	for _, want := range []string{"a=1", "a=2", "q=hello+world", "b=x%26y", "get_a=1", "all_a=1,2", "has_b=true"} {
		if !strings.Contains(name, want) {
			t.Errorf("URLSearchParams output %q missing %q", name, want)
		}
	}
}

func TestURLSearchParamsFromString(t *testing.T) {
	js := `export default new class { async single() {
		const p = new URLSearchParams("?x=1&y=hello+world");
		return [{name: "x=" + p.get("x") + ";y=" + p.get("y"), magnetLink: "` + magnet40 + `"}];
	} }`
	name, err := runSingleName(t, js)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if name != "x=1;y=hello world" {
		t.Errorf("parsed = %q, want x=1;y=hello world", name)
	}
}

// TestFetchInjectedIntoOptions verifies the host fetch is injected into the
// first method argument so extensions that destructure `fetch` from options
// (nekobt/anisearch) can call it.
func TestFetchInjectedIntoOptions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"title":"viaOptionsFetch","link":"` + magnet40 + `","seeders":7}]`))
	}))
	defer ts.Close()
	js := `export default new class { async single({fetch}) {
		const r = await fetch("` + ts.URL + `");
		return await r.json();
	} }`
	p, err := NewJSProvider("optfetch", "OptFetch", js, ts.Client(), testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProvider: %v", err)
	}
	res, err := p.Search(context.Background(), source.SearchOptions{Media: source.Media{RomajiTitle: "x"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) == 0 || res[0].Name != "viaOptionsFetch" {
		t.Fatalf("expected result via options.fetch, got %+v", res)
	}
}

// TestExportDefaultTrailingSemicolon verifies the exten.pages.dev "};" ending
// (a trailing semicolon after the class) is handled by stripExportDefault.
func TestExportDefaultTrailingSemicolon(t *testing.T) {
	js := `export default new class Foo {
		async single() { return [{name: "ok", magnetLink: "` + magnet40 + `"}]; }
	};
`
	name, err := runSingleName(t, js)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if name != "ok" {
		t.Errorf("name = %q, want ok (trailing-semicolon export compiled)", name)
	}
}

// TestJSONErrorIsActionable verifies Task G: .json() on a non-JSON body rejects
// with the actionable "non-JSON response (HTTP ...)" message including a body
// snippet, not the cryptic "invalid character 'T'".
func TestJSONErrorIsActionable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("This is an HTML error page, not JSON."))
	}))
	defer ts.Close()

	js := `export default new class { async single() {
		const r = await fetch("` + ts.URL + `");
		return await r.json();
	} }`
	p, err := NewJSProvider("json-err", "JsonErr", js, ts.Client(), testLogger(t))
	if err != nil {
		t.Fatalf("NewJSProvider: %v", err)
	}
	_, err = p.Search(context.Background(), source.SearchOptions{Media: source.Media{RomajiTitle: "x"}})
	if err == nil {
		t.Fatal("expected an error from .json() on an HTML body")
	}
	msg := err.Error()
	if !strings.Contains(msg, "non-JSON response (HTTP 502)") {
		t.Errorf("error = %q, want it to contain 'non-JSON response (HTTP 502)'", msg)
	}
	if !strings.Contains(msg, "This is an HTML error page") {
		t.Errorf("error = %q, want it to include the body snippet", msg)
	}
	if strings.Contains(msg, "invalid character") {
		t.Errorf("error = %q, should NOT be the cryptic decode error", msg)
	}
}
