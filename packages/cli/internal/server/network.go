package server

import (
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Connecting UDP selects the kernel's preferred source address without sending
// a datagram. The destination is an RFC 5737 documentation address.
const routeProbeAddress = "192.0.2.1:9"
const DefaultHost = "0.0.0.0"

type interfaceAddress struct {
	name  string
	flags net.Flags
	ip    netip.Addr
}

func primaryLANIPv4() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	var addresses []interfaceAddress
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			prefix, err := netip.ParsePrefix(addr.String())
			if err == nil {
				addresses = append(addresses, interfaceAddress{iface.Name, iface.Flags, prefix.Addr().Unmap()})
			}
		}
	}
	var preferred netip.Addr
	if conn, err := net.DialTimeout("udp4", routeProbeAddress, time.Second); err == nil {
		address, err := netip.ParseAddrPort(conn.LocalAddr().String())
		conn.Close()
		if err == nil {
			preferred = address.Addr().Unmap()
		}
	}
	ip, err := selectLANIPv4(addresses, preferred)
	if err != nil {
		return "", err
	}
	return ip.String(), nil
}

func selectLANIPv4(addresses []interfaceAddress, preferred netip.Addr) (netip.Addr, error) {
	var candidates []interfaceAddress
	for _, address := range addresses {
		if address.flags&net.FlagUp == 0 || address.flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 || !address.ip.Is4() || !address.ip.IsGlobalUnicast() || address.ip.IsLoopback() || virtualInterface(address.name) {
			continue
		}
		if address.ip == preferred {
			return address.ip, nil
		}
		candidates = append(candidates, address)
	}
	if len(candidates) == 0 {
		return netip.Addr{}, fmt.Errorf("no LAN IPv4 address found")
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].name != candidates[j].name {
			return candidates[i].name < candidates[j].name
		}
		return candidates[i].ip.Less(candidates[j].ip)
	})
	return candidates[0].ip, nil
}

func virtualInterface(name string) bool {
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "cni", "flannel", "tun", "tap", "utun", "tailscale", "wg", "zt", "vboxnet", "vmnet"} {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			return true
		}
	}
	return false
}

func ValidateHost(host string) error {
	if host == "" || host == DefaultHost {
		return nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.Is4() || (!ip.IsGlobalUnicast() && !ip.IsLoopback()) {
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

func displayURLs(host, port, lan string) []string {
	url := func(host string) string { return "http://" + net.JoinHostPort(host, port) }
	if host != DefaultHost {
		return []string{url(host)}
	}
	urls := []string{}
	if lan != "" {
		urls = append(urls, url(lan))
	}
	return append(urls, url("localhost"), url(DefaultHost))
}
