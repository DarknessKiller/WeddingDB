package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
)

// ProxyAwareMiddleware extracts the real client IP from X-Forwarded-For,
// but only when the direct peer is listed in TRUSTED_PROXIES (comma-separated
// IPs or CIDRs). Without that allowlist the header is client-controlled
// spoofing data and is ignored — direct deployments must not trust it.
func ProxyAwareMiddleware(next http.Handler) http.Handler {
	trusted := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(trusted) > 0 {
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				if ip, ok := clientFromXFF(xff, r.RemoteAddr, trusted); ok {
					r.RemoteAddr = ip
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func parseTrustedProxies(env string) []netip.Prefix {
	var prefixes []netip.Prefix
	for _, part := range strings.Split(env, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			p, err := netip.ParsePrefix(part)
			if err == nil {
				prefixes = append(prefixes, p)
			}
			continue
		}
		addr, err := netip.ParseAddr(part)
		if err == nil {
			prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}
	return prefixes
}

// clientFromXFF walks the X-Forwarded-For chain right-to-left, skipping
// trusted proxies; the first untrusted hop is the client. Returns host:port
// to match the RemoteAddr format.
func clientFromXFF(xff, remoteAddr string, trusted []netip.Prefix) (string, bool) {
	peer := hostFromAddr(remoteAddr)
	peerAddr, err := netip.ParseAddr(peer)
	if err != nil || !addrTrusted(peerAddr, trusted) {
		return "", false
	}
	hops := strings.Split(xff, ",")
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return "", false
		}
		if !addrTrusted(addr, trusted) {
			return net.JoinHostPort(addr.String(), ""), true
		}
	}
	return "", false
}

func addrTrusted(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func hostFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
