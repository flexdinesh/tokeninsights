package clientaddress

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyTrustBoundary(t *testing.T) {
	policy, err := Parse("10.0.0.0/24,2001:db8:1::/64")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, peer, forwarded, want string }{
		{"direct forgery", "192.0.2.1:4000", "198.51.100.1", "192.0.2.1:4000"},
		{"trusted proxy", "10.0.0.2:4000", "198.51.100.1", "198.51.100.1:0"},
		{"forged left prefix", "10.0.0.2:4000", "192.0.2.99,198.51.100.1", "198.51.100.1:0"},
		{"trusted chain", "10.0.0.2:4000", "198.51.100.1,10.0.0.3", "198.51.100.1:0"},
		{"untrusted middle", "10.0.0.2:4000", "192.0.2.99,198.51.100.1,10.0.0.3", "198.51.100.1:0"},
		{"missing", "10.0.0.2:4000", "", "10.0.0.2:4000"},
		{"malformed", "10.0.0.2:4000", "198.51.100.1,invalid", "10.0.0.2:4000"},
		{"empty hop", "10.0.0.2:4000", "198.51.100.1,", "10.0.0.2:4000"},
		{"ipv6", "[2001:db8:1::1]:4000", "2001:db8:2::1", "[2001:db8:2::1]:0"},
		{"mapped peer", "[::ffff:10.0.0.2]:4000", "::ffff:198.51.100.1", "198.51.100.1:0"},
		{"zone", "10.0.0.2:4000", "fe80::1%eth0", "10.0.0.2:4000"},
		{"oversized chain", "10.0.0.2:4000", strings.Repeat("10.0.0.3,", maxForwardedHops) + "10.0.0.4", "10.0.0.2:4000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://usage.example/", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			r.Header.Set("X-Forwarded-Host", "evil.example")
			r.Header.Set("Forwarded", "for=192.0.2.99")
			policy.Handler(http.HandlerFunc(func(_ http.ResponseWriter, got *http.Request) {
				if got.RemoteAddr != tc.want || got.Host != "usage.example" {
					t.Fatal(got.RemoteAddr, got.Host)
				}
			})).ServeHTTP(httptest.NewRecorder(), r)
			if r.RemoteAddr != tc.peer {
				t.Fatal("mutated caller request")
			}
			if (Policy{}).address(r) != tc.peer {
				t.Fatal("default trusted a header")
			}
		})
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.2:4000"
	r.Header.Add("X-Forwarded-For", "192.0.2.99")
	r.Header.Add("X-Forwarded-For", "198.51.100.1")
	if policy.address(r) != "198.51.100.1:0" {
		t.Fatal("duplicate headers ignored")
	}
	for _, raw := range []string{"*", "10.0.0.1", "10.0.0.0/24,", "::ffff:10.0.0.0/120"} {
		if _, err := Parse(raw); err == nil {
			t.Fatal("invalid trust", raw)
		}
	}
}
