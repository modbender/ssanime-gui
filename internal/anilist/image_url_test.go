package anilist

import "testing"

func TestSafeImageURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Allowlisted AniList CDN hosts over https pass through unchanged.
		{"https://s4.anilist.co/file/anilistcdn/media/anime/cover/large/x.jpg",
			"https://s4.anilist.co/file/anilistcdn/media/anime/cover/large/x.jpg"},
		{"https://img.anili.st/media/12345", "https://img.anili.st/media/12345"},
		// Rejected: empty, wrong scheme, off-allowlist host, javascript/data payloads.
		{"", ""},
		{"http://s4.anilist.co/x.jpg", ""}, // not https
		{"https://evil.example.com/x.jpg", ""},
		{"https://anilist.co.evil.com/x.jpg", ""},
		{"javascript:alert(1)", ""},
		{"data:image/png;base64,AAAA", ""},
		{"//s4.anilist.co/x.jpg", ""}, // scheme-relative → no https scheme
	}
	for _, c := range cases {
		if got := safeImageURL(c.in); got != c.want {
			t.Errorf("safeImageURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
