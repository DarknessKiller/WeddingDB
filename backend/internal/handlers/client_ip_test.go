package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name, remoteAddr, want string
	}{
		{"ipv4 with port", "203.0.113.7:5555", "203.0.113.7"},
		{"ipv4 from proxy, empty port", "203.0.113.7:", "203.0.113.7"},
		{"ipv6 keyed by /64", "[2001:db8:1:2:aaaa:bbbb:cccc:dddd]:443", "2001:db8:1:2::/64"},
		{"ipv6 sibling shares the key", "[2001:db8:1:2::1]:443", "2001:db8:1:2::/64"},
		{"ipv4-mapped ipv6 kept whole", "[::ffff:203.0.113.7]:80", "::ffff:203.0.113.7"},
		{"no port", "203.0.113.7", "203.0.113.7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if got := clientIP(r); got != tt.want {
				t.Fatalf("clientIP(%q) = %q, want %q", tt.remoteAddr, got, tt.want)
			}
		})
	}
}
