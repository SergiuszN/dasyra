// Package web reads the internet for the tools: search engines, pages, JSON APIs and PDFs.
package web

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"

	"dasyra/internal/app/tool"
)

const browserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"

// Addresses a model-chosen URL must never reach: SearXNG, other containers, the host and the Tailscale network.
var internal = prefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
	"192.168.0.0/16", "224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
)

func prefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		out[i] = netip.MustParsePrefix(c)
	}
	return out
}

func isPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return false
	}
	for _, p := range internal {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// checkURL allows only http(s) URLs with a host; the address itself is checked when connecting.
func checkURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, tool.ErrNotPublic
	}
	return u, nil
}

// newClient refuses internal addresses at connect time, so DNS tricks and redirects to the internal network
// fail too. allowPrivate is for tests against local servers.
func newClient(timeout time.Duration, allowPrivate, followRedirects bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if !allowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, _ := net.SplitHostPort(address)
			if ip, err := netip.ParseAddr(host); err != nil || !isPublic(ip) {
				return fmt.Errorf("%w: %s", tool.ErrNotPublic, host)
			}
			return nil
		}
	}
	client := &http.Client{Timeout: timeout, Transport: &http.Transport{DialContext: dialer.DialContext, Proxy: nil}}
	if !followRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return client
}
