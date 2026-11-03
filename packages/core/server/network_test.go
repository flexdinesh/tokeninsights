package server

import (
	"net"
	"net/url"
	"testing"
	"time"
)

func TestExplicitHostRequiresIPv4(t *testing.T) {
	for _, host := range []string{"", "0.0.0.0", "127.0.0.1", "10.0.1.151", "192.168.0.2"} {
		if err := ValidateHost(host); err != nil {
			t.Errorf("%s: %v", host, err)
		}
	}
	for _, host := range []string{"::", "::1", "localhost", "not-an-ip", "224.0.0.1", "255.255.255.255"} {
		if err := ValidateHost(host); err == nil {
			t.Errorf("accepted invalid host %q", host)
		}
	}
}

func TestDefaultListenerUsesLoopback(t *testing.T) {
	listener, err := listen("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if host != DefaultHost || port == "0" {
		t.Fatalf("unexpected default listener %s", listener.Addr())
	}
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort(DefaultHost, port), time.Second)
	if err != nil {
		t.Fatalf("localhost must be reachable: %v", err)
	}
	_ = connection.Close()
	if got := displayURL("", port); got != "http://localhost:"+port {
		t.Fatalf("URL = %q", got)
	}
	parsed, err := url.Parse(displayURL("10.0.1.151", port))
	if err != nil || parsed.Host != "10.0.1.151:"+port {
		t.Fatalf("explicit URL = %q, %v", parsed, err)
	}
}
