// Package clientaddress resolves client IPs at the hosted HTTP boundary.
package clientaddress

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const maxForwardedHops = 32

// Policy trusts only explicitly configured proxy networks. Its zero value uses
// the socket peer. Forwarded headers never determine origin or authorization.
type Policy struct{ networks []netip.Prefix }

func Parse(raw string) (Policy, error) {
	p := Policy{}
	if raw == "" {
		return p, nil
	}
	for _, part := range strings.Split(raw, ",") {
		network, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil || network.Addr().Is4In6() {
			return Policy{}, fmt.Errorf("trusted proxies must be comma-separated IP CIDRs")
		}
		p.networks = append(p.networks, network.Masked())
	}
	return p, nil
}

func (p Policy) trusted(ip netip.Addr) bool {
	for _, network := range p.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (p Policy) address(r *http.Request) string {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	current := peer.Addr().Unmap()
	if !p.trusted(current) {
		return r.RemoteAddr
	}
	chain := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	if len(chain) > maxForwardedHops {
		return r.RemoteAddr
	}
	for i := len(chain) - 1; i >= 0 && p.trusted(current); i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil || ip.Zone() != "" {
			return r.RemoteAddr
		}
		current = ip.Unmap()
	}
	return net.JoinHostPort(current.String(), "0")
}

func (p Policy) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := r.Clone(r.Context())
		request.RemoteAddr = p.address(r)
		next.ServeHTTP(w, request)
	})
}
