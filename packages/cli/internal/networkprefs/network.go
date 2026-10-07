// Package networkprefs owns listener preferences without importing HTTP servers.
package networkprefs

import (
	"fmt"
	"net/netip"
)

const DefaultHost = "127.0.0.1"
const DefaultPort = 8765

func ValidateHost(host string) error {
	if host == "" {
		return nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.Is4() || (!ip.IsGlobalUnicast() && !ip.IsLoopback() && !ip.IsUnspecified()) {
		return fmt.Errorf("--host must be a unicast IPv4 address or 0.0.0.0")
	}
	return nil
}
