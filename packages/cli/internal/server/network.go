package server

import (
	"fmt"
	"net"
	"strconv"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/networkprefs"
)

const DefaultHost = networkprefs.DefaultHost
const defaultDisplayHost = "localhost"

func ValidateHost(host string) error { return networkprefs.ValidateHost(host) }

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

// Listen binds only the web/API server.
func Listen(host string, port int) (net.Listener, error) { return listen(host, port) }
