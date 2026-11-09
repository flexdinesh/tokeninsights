package endpoint

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestParseAcceptsLocalAndSSHOnly(t *testing.T) {
	for value, want := range map[string]string{
		"":                       Local,
		"local":                  Local,
		"ssh://box":              "ssh://box",
		"ssh://dee@box":          "ssh://dee@box",
		"ssh://dee@box:2222":     "ssh://dee@box:2222",
		"ssh://box.example.com/": "ssh://box.example.com",
		"ssh://10.0.0.5:22":      "ssh://10.0.0.5:22",
	} {
		parsed, err := Parse(value)
		if err != nil || parsed.String() != want || parsed.IsLocal() != (want == Local) {
			t.Fatalf("Parse(%q) = %v, %v; want %s", value, parsed, err, want)
		}
	}
	for _, value := range []string{
		"remote", "tcp://box:8765", "http://box", "ssh://", "ssh://-oProxyCommand=evil",
		"ssh://-box", "ssh://dee:secret@box", "ssh://-l@box", "ssh://box/path", "ssh://box?x=1",
		"ssh://box#f", "ssh://box:0", "ssh://box:70000", " ssh://box", "ssh://box:abc", "ssh://dee;rm@box",
	} {
		if parsed, err := Parse(value); err == nil {
			t.Fatalf("Parse(%q) accepted %v", value, parsed)
		}
	}
}

func TestTCPConnectionRoundTripAndRedaction(t *testing.T) {
	target := Endpoint{TCP: &TCP{Address: "127.0.0.1:12345", Pin: strings.Repeat("ab", 32), Token: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}}
	value := target.ConnectionString()
	parsed, err := Parse(value)
	if err != nil || parsed.TCP == nil || *parsed.TCP != *target.TCP || parsed.IsLocal() {
		t.Fatal("TCP round trip failed", err)
	}
	if strings.Contains(parsed.String(), target.TCP.Token) || strings.Contains(parsed.String(), target.TCP.Pin) {
		t.Fatal("safe label leaked credentials")
	}
	for _, label := range []string{fmt.Sprint(*parsed.TCP), fmt.Sprintf("%+v", parsed.TCP), fmt.Sprintf("%#v", parsed.TCP), fmt.Sprintf("%#v", parsed)} {
		if strings.Contains(label, target.TCP.Token) || strings.Contains(label, target.TCP.Pin) {
			t.Fatal("formatted connection leaked credentials")
		}
	}
	for _, bad := range []string{
		value + "&token=" + target.TCP.Token,
		strings.Replace(value, "/v1?", "/v2?", 1),
		strings.Replace(value, "address=", "extra=x&address=", 1),
		strings.Replace(value, "#pin=", "#extra=x&pin=", 1),
		strings.Replace(value, target.TCP.Pin, "bad", 1),
		strings.Replace(value, target.TCP.Token, "short", 1),
		strings.Replace(value, "127.0.0.1", "0.0.0.0", 1),
		strings.Replace(value, "127.0.0.1", "*", 1),
		strings.Replace(value, "127.0.0.1", "host%3Abad", 1),
		strings.Replace(value, ":12345", ":0", 1),
	} {
		if _, err := Parse(bad); err == nil {
			t.Fatal("malformed TCP accepted")
		} else if strings.Contains(err.Error(), target.TCP.Token) {
			t.Fatal("parse error leaked secret")
		}
	}
}

func TestTCPConnectionAddressIsReadable(t *testing.T) {
	pin, token := strings.Repeat("ab", 32), base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	for address, query := range map[string]string{
		"10.0.1.151:37619":  "?address=10.0.1.151:37619#",
		"devbox.test:9000":  "?address=devbox.test:9000#",
		"[2001:db8::1]:443": "?address=%5B2001:db8::1%5D:443#",
	} {
		target := TCP{Address: address, Pin: pin, Token: token}
		value := target.ConnectionString()
		if !strings.Contains(value, query) {
			t.Fatalf("ConnectionString() = %q, want query %q", value, query)
		}
		parsed, err := Parse(value)
		if err != nil || parsed.TCP == nil || *parsed.TCP != target {
			t.Fatal("readable TCP round trip failed", address, err)
		}
	}
	escaped := "tokeninsights://connect/v1?address=10.0.1.151%3A37619#pin=" + pin + "&token=" + token
	parsed, err := Parse(escaped)
	if err != nil || parsed.TCP == nil || parsed.TCP.Address != "10.0.1.151:37619" {
		t.Fatal("percent-encoded address rejected", err)
	}
}
