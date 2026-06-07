package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// maxBodyBytes caps request bodies on the API. Every payload the API accepts
// (settings, feeds, profiles, series) is small JSON; 1 MiB is generous and
// stops a malicious client from streaming an unbounded body into a decoder.
const maxBodyBytes = 1 << 20

// localGuard hardens the localhost daemon against the browser-pivot attacks a
// 127.0.0.1 bind does NOT stop on its own:
//
//   - DNS rebinding: an attacker domain that rebinds to 127.0.0.1 becomes
//     "same-origin" in the victim's browser. We require the Host header to be a
//     loopback authority; the rebound request still carries the attacker's own
//     hostname in Host, so it is rejected.
//   - CSRF: a malicious page can issue "simple" cross-site POSTs (text/plain, no
//     preflight) to mutate state. Browsers attach an Origin header to every POST,
//     so we reject state-changing requests whose Origin (or Sec-Fetch-Site) is
//     cross-site.
//
// It also bounds the request body. Applied to the /api subtree only; the SPA
// fallback serves static assets and needs neither check.
func localGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			WriteError(w, http.StatusForbidden, "forbidden host")
			return
		}
		if isStateChanging(r.Method) {
			if origin := r.Header.Get("Origin"); origin != "" {
				if !isLoopbackHost(originHost(origin)) {
					WriteError(w, http.StatusForbidden, "cross-origin request rejected")
					return
				}
			} else if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
				WriteError(w, http.StatusForbidden, "cross-site request rejected")
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// isLoopbackHost reports whether a Host/authority refers to the local machine:
// "localhost" or an IP literal in the loopback range, with or without a port.
func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// originHost extracts the authority from an Origin header value
// ("http://127.0.0.1:4773" → "127.0.0.1:4773"); "" if it doesn't parse.
func originHost(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return u.Host
}
