package download

import (
	"path/filepath"
	"testing"
)

func TestSafeSourcePath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"data", "downloads")
	const hash = "abcdef0123456789"

	t.Run("normal path stays under root", func(t *testing.T) {
		got, err := safeSourcePath(root, hash, "Show Name/Show - 01.mkv")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(root, hash, "Show Name", "Show - 01.mkv")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if !isSubpath(root, got) {
			t.Errorf("result %q is not under root %q", got, root)
		}
	})

	t.Run("traversal input is rejected", func(t *testing.T) {
		if got, err := safeSourcePath(root, hash, "../../evil"); err == nil {
			t.Errorf("expected error for traversal input, got path %q", got)
		}
	})

	t.Run("embedded traversal that escapes is rejected", func(t *testing.T) {
		// ToSafeFilePath cleans this to "../evil", which escapes the root dir.
		if got, err := safeSourcePath(root, hash, "a/../../evil"); err == nil {
			t.Errorf("expected error for escaping input, got path %q", got)
		}
	})
}

func TestIsSubpath(t *testing.T) {
	base := filepath.Join(string(filepath.Separator)+"data", "downloads")
	cases := []struct {
		sub  string
		want bool
	}{
		{filepath.Join(base, "x", "y.mkv"), true},
		{base, true},
		{filepath.Join(base, ".."), false},
		{filepath.Join(string(filepath.Separator)+"data", "other"), false},
	}
	for _, c := range cases {
		if got := isSubpath(base, c.sub); got != c.want {
			t.Errorf("isSubpath(%q, %q) = %v, want %v", base, c.sub, got, c.want)
		}
	}
	// Sanity: a sibling sharing a prefix string but not a path component.
	if isSubpath(base, base+"-sibling") {
		t.Errorf("prefix-sharing sibling should not be a subpath")
	}
}
