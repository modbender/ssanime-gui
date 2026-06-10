package doh

import "testing"

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		// CGNAT (RFC 6598) — the gap this guard closes.
		{"100.64.0.1", true},
		{"100.127.255.255", true},
		// Previously-covered ranges must stay blocked.
		{"169.254.169.254", true}, // cloud metadata (link-local)
		{"::ffff:10.0.0.1", true}, // IPv4-mapped RFC1918
		{"fc00::1", true},         // IPv6 ULA (private)
		{"127.0.0.1", true},       // loopback
		{"::1", true},             // loopback
		{"0.0.0.0", true},         // unspecified
		{"192.0.0.1", true},       // RFC 6890 IETF protocol assignments
		{"198.19.0.1", true},      // RFC 2544 benchmarking
		{"not-an-ip", true},       // unparseable → blocked
		// Public addresses must remain reachable.
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2606:4700:4700::1111", false}, // Cloudflare public IPv6
	}
	for _, c := range cases {
		if got := isBlockedIP(c.ip); got != c.blocked {
			t.Errorf("isBlockedIP(%q) = %v, want %v", c.ip, got, c.blocked)
		}
	}
}
