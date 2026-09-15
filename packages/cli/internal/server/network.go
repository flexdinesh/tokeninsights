package server

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

const DefaultHost = "127.0.0.1"
const defaultDisplayHost = "localhost"

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

func listen(host string, port int) (net.Listener, error) {
	if err := ValidateHost(host); err != nil {
		return nil, err
	}
	if host == "" {
		host = DefaultHost
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}
	return listener, nil
}

func displayURL(host, port string) string {
	if host == "" {
		host = defaultDisplayHost
	}
	return "http://" + net.JoinHostPort(host, port)
}
