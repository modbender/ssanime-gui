package version

import (
	"testing"
	"time"
)

// TestInstanceIDStable asserts InstanceID() returns the same value across calls
// within one process (it is memoized and the underlying inputs don't change).
func TestInstanceIDStable(t *testing.T) {
	first := InstanceID()
	if first == "" {
		t.Fatal("InstanceID() returned empty string")
	}
	for i := 0; i < 5; i++ {
		if got := InstanceID(); got != first {
			t.Fatalf("InstanceID() not stable: call %d = %q, want %q", i, got, first)
		}
	}
}

// TestComposeInstanceIDEqualInputs asserts identical inputs hash to identical ids.
func TestComposeInstanceIDEqualInputs(t *testing.T) {
	mod := time.Unix(1_700_000_000, 0)
	a := ComposeInstanceID("dev", "", "/path/to/ssanime", 12345, mod)
	b := ComposeInstanceID("dev", "", "/path/to/ssanime", 12345, mod)
	if a != b {
		t.Fatalf("identical inputs yielded different ids: %q != %q", a, b)
	}
}

// TestComposeInstanceIDDistinguishesFields asserts every field that should change
// the identity actually does — in particular the dev-rebuild case where only the
// exe size or modtime differs (Version=="dev", Commit=="" both times).
func TestComposeInstanceIDDistinguishesFields(t *testing.T) {
	base := ComposeInstanceID("dev", "", "/path/to/ssanime", 12345, time.Unix(1_700_000_000, 0))

	cases := []struct {
		name string
		id   string
	}{
		{"different version", ComposeInstanceID("v1.0.0", "", "/path/to/ssanime", 12345, time.Unix(1_700_000_000, 0))},
		{"different commit", ComposeInstanceID("dev", "abc123", "/path/to/ssanime", 12345, time.Unix(1_700_000_000, 0))},
		{"different path", ComposeInstanceID("dev", "", "/other/ssanime", 12345, time.Unix(1_700_000_000, 0))},
		{"different size (dev rebuild)", ComposeInstanceID("dev", "", "/path/to/ssanime", 99999, time.Unix(1_700_000_000, 0))},
		{"different modtime (dev rebuild)", ComposeInstanceID("dev", "", "/path/to/ssanime", 12345, time.Unix(1_700_000_001, 0))},
	}
	for _, c := range cases {
		if c.id == base {
			t.Errorf("%s: id matched base %q, want different", c.name, base)
		}
	}
}
