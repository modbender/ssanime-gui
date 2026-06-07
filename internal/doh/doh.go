// Package doh provides a DNS-over-HTTPS resolver and an http transport that
// dials resolved IPs directly. This defeats ISP DNS poisoning (e.g. nyaa.si
// resolving to a sinkhole) without needing a proxy — verified to recover full
// nyaa results on a network that DNS-blocks it. TLS SNI keeps the real hostname,
// so certificate validation is unaffected.
package doh

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// DefaultResolver is the Cloudflare DoH endpoint. 1.1.1.1 is an IP literal, so
// reaching it needs no system DNS — the part the ISP can poison.
const DefaultResolver = "https://1.1.1.1/dns-query"

// Resolver resolves hostnames to IPs over DNS-over-HTTPS, caching results.
type Resolver struct {
	endpoint string
	client   *http.Client
	cache    sync.Map // host -> ip
}

// NewResolver builds a resolver using the given DoH endpoint (empty = default).
func NewResolver(endpoint string) *Resolver {
	if endpoint == "" {
		endpoint = DefaultResolver
	}
	return &Resolver{
		endpoint: endpoint,
		// A plain client to the IP-literal endpoint; no custom transport (no recursion).
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Resolve returns an A-record IP for host, cached for the process lifetime.
func (r *Resolver) Resolve(ctx context.Context, host string) (string, error) {
	if net.ParseIP(host) != nil {
		return host, nil
	}
	if ip, ok := r.cache.Load(host); ok {
		return ip.(string), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		r.endpoint+"?type=A&name="+url.QueryEscape(host), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("accept", "application/dns-json")
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	for _, a := range out.Answer {
		if a.Type == 1 && net.ParseIP(a.Data) != nil { // type 1 = A
			r.cache.Store(host, a.Data)
			return a.Data, nil
		}
	}
	return "", fmt.Errorf("doh: no A record for %s", host)
}

// HTTPClient returns an http.Client whose dialer resolves via this resolver.
func (r *Resolver) HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: r.Transport()}
}

// GuardedHTTPClient returns a DoH client that additionally refuses to connect to
// loopback, private, or link-local addresses. Use it for requests driven by
// untrusted input — e.g. third-party JS extensions, whose fetch() must never be
// able to reach the daemon's own localhost API or internal-network hosts (SSRF).
// Because the check runs on the post-DoH dial IP, it also defeats DNS rebinding
// and is re-applied on every redirect hop.
func (r *Resolver) GuardedHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: r.transport(true)}
}

// Transport returns an http transport that resolves every dial through DoH and
// connects to the resolved IP. TLS verification still uses the original host.
func (r *Resolver) Transport() *http.Transport { return r.transport(false) }

func (r *Resolver) transport(blockPrivate bool) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ip, err := r.Resolve(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("doh dial %s: %w", host, err)
			}
			if blockPrivate && isBlockedIP(ip) {
				return nil, fmt.Errorf("doh: refusing non-public address %s (%s)", ip, host)
			}
			var d net.Dialer
			return d.DialContext(ctx, network, net.JoinHostPort(ip, port))
		},
	}
}

// isBlockedIP reports whether an SSRF-guarded client must refuse to reach ipStr:
// loopback (127/8, ::1), private (RFC1918 / RFC4193), link-local (incl. the
// 169.254.169.254 cloud-metadata range), or the unspecified address. An
// unparseable value is treated as blocked.
func isBlockedIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
