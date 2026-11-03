// Package link parses tokeninsights:// dashboard links.
//
// The CLI writes links as tokeninsights://open?url=<escaped http(s) URL>.
// Parsing is pure: it performs no I/O and trusts nothing about the target.
package link

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// Scheme is the URL scheme the desktop viewer registers.
const Scheme = "tokeninsights"

// NativeScheme identifies MyGo independently of Electron.
const NativeScheme = "tokeninsights-mygo"

const (
	openAction = "open"
	urlParam   = "url"
)

var (
	// ErrMalformed rejects text that is not a tokeninsights:// open link.
	ErrMalformed = errors.New("not a TokenInsights open link")
	// ErrTarget rejects a dashboard URL that is not an absolute http(s) URL.
	ErrTarget = errors.New("dashboard URL must be an absolute http or https URL without credentials")
)

// Target is a validated dashboard URL.
type Target struct {
	// URL is the dashboard URL to load.
	URL string
	// Origin is the normalized scheme://host[:port] of URL; default ports are omitted.
	Origin string
	// Loopback reports whether the host is localhost or a loopback address.
	Loopback bool
}

// Parse validates a tokeninsights://open?url=... link.
func Parse(raw string) (Target, error) {
	u, err := url.Parse(raw)
	if err != nil || (!strings.EqualFold(u.Scheme, Scheme) && !strings.EqualFold(u.Scheme, NativeScheme)) || u.Opaque != "" || u.User != nil || u.Fragment != "" {
		return Target{}, ErrMalformed
	}
	if !strings.EqualFold(u.Host, openAction) || (u.Path != "" && u.Path != "/") {
		return Target{}, ErrMalformed
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Target{}, ErrMalformed
	}
	targets := query[urlParam]
	if len(targets) != 1 || len(query) != 1 {
		return Target{}, ErrMalformed
	}
	return Dashboard(targets[0])
}

// Dashboard validates a dashboard URL.
func Dashboard(raw string) (Target, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.Hostname() == "" {
		return Target{}, ErrTarget
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return Target{}, ErrTarget
	}
	u.Scheme = scheme
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == defaultPort(scheme) {
		port = ""
	}
	authority := host
	if strings.Contains(host, ":") {
		authority = "[" + host + "]"
	}
	if port != "" {
		authority = net.JoinHostPort(host, port)
	}
	return Target{URL: u.String(), Origin: scheme + "://" + authority, Loopback: isLoopback(host)}, nil
}

// SameOrigin reports whether raw is an http(s) URL with the given normalized origin.
func SameOrigin(raw, origin string) bool {
	target, err := Dashboard(raw)
	return err == nil && target.Origin == origin
}

func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}
