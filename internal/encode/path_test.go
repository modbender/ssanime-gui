package encode

import (
	"path/filepath"
	"testing"
)

func TestLibraryPathJellyfin(t *testing.T) {
	got := LibraryPath(PathParams{
		EncodedRoot: filepath.Join("R:", "library"),
		Series:      "Frieren: Beyond Journey's End",
		Season:      1,
		Episode:     5,
		Resolution:  720,
		Ext:         "mkv",
	})
	// Colon is stripped from the title; structure is Series/Season 01/720p/file.
	want := filepath.Join("R:", "library",
		"Frieren Beyond Journey's End", "Season 01", "720p",
		"Frieren Beyond Journey's End - S01E05.mkv")
	if got != want {
		t.Errorf("LibraryPath = %q\n          want %q", got, want)
	}
}

func TestLibraryPathSpecial(t *testing.T) {
	// episode_no NULL -> special -> Season 00 / S00E01.
	got := LibraryPath(PathParams{
		EncodedRoot: "L",
		Series:      "Some OVA",
		Season:      2, // overridden by special
		Episode:     1,
		IsSpecial:   true,
		Resolution:  1080,
		Ext:         "mkv",
	})
	want := filepath.Join("L", "Some OVA", "Season 00", "1080p", "Some OVA - S00E01.mkv")
	if got != want {
		t.Errorf("special LibraryPath = %q, want %q", got, want)
	}
}

func TestLibraryPathSanitizesIllegalChars(t *testing.T) {
	got := LibraryPath(PathParams{
		EncodedRoot: "L",
		Series:      `Re:Zero \ Starting/Life * "Quotes" <tag>?`,
		Season:      1, Episode: 12, Resolution: 480, Ext: "mkv",
	})
	// None of \ / : * ? " < > | may survive in the series segment.
	for _, bad := range []string{`\`, `:`, `*`, `?`, `"`, `<`, `>`, `|`} {
		// The series segment is everything before the first separator we control;
		// just assert the illegal char doesn't appear in any segment except the OS
		// path separator.
		base := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(got))))
		if containsRune(base, bad) {
			t.Errorf("series segment %q still contains %q", base, bad)
		}
	}
}

func TestLibraryPathLongRunnerEpisode(t *testing.T) {
	got := LibraryPath(PathParams{
		EncodedRoot: "L", Series: "One Piece", Season: 1, Episode: 1090,
		Resolution: 1080, Ext: "mkv",
	})
	want := filepath.Join("L", "One Piece", "Season 01", "1080p", "One Piece - S01E1090.mkv")
	if got != want {
		t.Errorf("long-runner path = %q, want %q", got, want)
	}
}

func TestLibraryPathDefaultExt(t *testing.T) {
	got := LibraryPath(PathParams{
		EncodedRoot: "L", Series: "X", Season: 1, Episode: 1, Resolution: 720, Ext: "",
	})
	want := filepath.Join("L", "X", "Season 01", "720p", "X - S01E01.mkv")
	if got != want {
		t.Errorf("default ext path = %q, want %q", got, want)
	}
}

func containsRune(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
