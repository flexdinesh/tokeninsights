package server

import (
	"net"
	"net/netip"
	"reflect"
	"testing"
	"time"
)

func TestLANSelectionExcludesVirtualAndNonIPv4Addresses(t *testing.T) {
	lan := netip.MustParseAddr("10.0.1.151")
	addresses := []interfaceAddress{
		{"wlan0", net.FlagUp | net.FlagBroadcast, lan},
		{"lo", net.FlagUp | net.FlagLoopback, netip.MustParseAddr("127.0.0.1")},
		{"eth0", 0, netip.MustParseAddr("192.168.1.2")},
		{"wlan0", net.FlagUp, netip.MustParseAddr("fd91:8395:ec1b::1")},
		{"other", net.FlagUp | net.FlagPointToPoint, netip.MustParseAddr("10.2.0.1")},
	}
	for _, name := range []string{"docker0", "br-2596fe8eb7ed", "veth55a4fff", "virbr0", "cni0", "tun0", "utun0", "tailscale0", "wg0"} {
		addresses = append(addresses, interfaceAddress{name, net.FlagUp | net.FlagBroadcast, netip.MustParseAddr("172.17.0.1")})
	}
	for _, preferred := range []netip.Addr{lan, netip.MustParseAddr("172.17.0.1"), {}} {
		got, err := selectLANIPv4(addresses, preferred)
		if err != nil || got != lan {
			t.Fatalf("selected %v, %v; want %v", got, err, lan)
		}
	}
	if _, err := selectLANIPv4(addresses[1:], netip.Addr{}); err == nil {
		t.Fatal("missing LAN must return a discovery error")
	}
}

func TestLANSelectionPrefersDefaultRouteAmongPhysicalInterfaces(t *testing.T) {
	wired, wifi := netip.MustParseAddr("192.168.1.2"), netip.MustParseAddr("10.0.1.151")
	addresses := []interfaceAddress{{"en0", net.FlagUp, wired}, {"wlan0", net.FlagUp, wifi}}
	got, err := selectLANIPv4(addresses, wifi)
	if err != nil || got != wifi {
		t.Fatalf("preferred LAN: %v, %v", got, err)
	}
}

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

func TestWildcardListenerAndDisplayedURLs(t *testing.T) {
	listener, err := listen("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if host != DefaultHost || port == "0" {
		t.Fatalf("unexpected default listener %s", listener.Addr())
	}
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", port), time.Second)
	if err != nil {
		t.Fatalf("localhost must be reachable: %v", err)
	}
	connection.Close()
	want := []string{"http://10.0.1.151:" + port, "http://localhost:" + port, "http://0.0.0.0:" + port}
	if got := displayURLs(host, port, "10.0.1.151"); !reflect.DeepEqual(got, want) {
		t.Fatalf("URLs = %v, want %v", got, want)
	}
	if got := displayURLs(host, port, ""); !reflect.DeepEqual(got, want[1:]) {
		t.Fatalf("offline URLs = %v", got)
	}
	want = []string{"http://127.0.0.1:" + port}
	if got := displayURLs("127.0.0.1", port, "10.0.1.151"); !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit host URLs = %v", got)
	}
}
