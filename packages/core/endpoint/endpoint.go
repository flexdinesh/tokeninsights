// Package endpoint defines connection values without transport or storage ownership.
package endpoint

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const Local = "local"
const DefaultCommand = "tokeninsights"

// SecretBytes is the entropy and wire size of a temporary gateway credential.
const SecretBytes = 32

type Info struct {
	Transport string `json:"transport"`
	Target    string `json:"target"`
	State     string `json:"state"`
	Hostname  string `json:"hostname,omitempty"`
}
type Endpoint struct {
	SSH *SSH
	TCP *TCP
}
type SSH struct {
	User, Host string
	Port       int
	Command    string
}
type TCP struct{ Address, Pin, Token string }

func (e Endpoint) IsLocal() bool       { return e.SSH == nil && e.TCP == nil }
func (t TCP) String() string           { return "tcp://" + t.Address }
func (t TCP) GoString() string         { return t.String() }
func (t TCP) ConnectionString() string { return (Endpoint{TCP: &t}).ConnectionString() }
func (e Endpoint) GoString() string    { return e.String() }

// String never includes a TCP credential or key pin.
func (e Endpoint) String() string {
	if e.TCP != nil {
		return e.TCP.String()
	}
	if e.SSH == nil {
		return Local
	}
	u := url.URL{Scheme: "ssh", Host: e.SSH.Host}
	if strings.Contains(e.SSH.Host, ":") {
		u.Host = "[" + e.SSH.Host + "]"
	}
	if e.SSH.Port != 0 {
		u.Host = net.JoinHostPort(e.SSH.Host, strconv.Itoa(e.SSH.Port))
	}
	if e.SSH.User != "" {
		u.User = url.User(e.SSH.User)
	}
	return u.String()
}

// ConnectionString intentionally serializes the temporary TCP credential.
func (e Endpoint) ConnectionString() string {
	if e.TCP == nil {
		return e.String()
	}
	u := url.URL{Scheme: "tokeninsights", Host: "connect", Path: "/v1", RawQuery: url.Values{"address": {e.TCP.Address}}.Encode(), Fragment: url.Values{"pin": {e.TCP.Pin}, "token": {e.TCP.Token}}.Encode()}
	return u.String()
}

func Parse(value string) (Endpoint, error) {
	invalid := errors.New("invalid connection; use local, ssh://[user@]host[:port], or the complete temporary TCP connection string")
	if value == "" || value == Local {
		return Endpoint{}, nil
	}
	u, err := url.Parse(value)
	if err != nil || strings.TrimSpace(value) != value || u.Opaque != "" {
		return Endpoint{}, invalid
	}
	if u.Scheme == "tokeninsights" {
		query, qe := url.ParseQuery(u.RawQuery)
		fragment, fe := url.ParseQuery(u.Fragment)
		if qe != nil || fe != nil || u.Host != "connect" || u.Path != "/v1" || u.RawPath != "" || u.User != nil || len(query) != 1 || len(query["address"]) != 1 || len(fragment) != 2 || len(fragment["pin"]) != 1 || len(fragment["token"]) != 1 {
			return Endpoint{}, invalid
		}
		tcp := TCP{Address: query.Get("address"), Pin: fragment.Get("pin"), Token: fragment.Get("token")}
		if tcp.Validate() != nil {
			return Endpoint{}, invalid
		}
		return Endpoint{TCP: &tcp}, nil
	}
	if u.Scheme != "ssh" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return Endpoint{}, invalid
	}
	host := u.Hostname()
	if !safeToken(host) || strings.HasSuffix(u.Host, ":") {
		return Endpoint{}, invalid
	}
	remote := SSH{Host: host}
	if u.User != nil {
		if _, password := u.User.Password(); password {
			return Endpoint{}, invalid
		}
		remote.User = u.User.Username()
		if !safeToken(remote.User) {
			return Endpoint{}, invalid
		}
	}
	if port := u.Port(); port != "" {
		remote.Port, err = strconv.Atoi(port)
		if err != nil || remote.Port < 1 || remote.Port > 65535 {
			return Endpoint{}, invalid
		}
	}
	return Endpoint{SSH: &remote}, nil
}

func (t TCP) Validate() error {
	invalid := errors.New("invalid TCP connection address, pin, or credential")
	host, port, err := net.SplitHostPort(t.Address)
	if err != nil || !safeToken(host) || strings.ContainsAny(host, "?#%") {
		return invalid
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() {
			return invalid
		}
	} else if !hostname(host) {
		return invalid
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return invalid
	}
	pin, err := hex.DecodeString(t.Pin)
	if err != nil || len(pin) != 32 {
		return invalid
	}
	secret, err := base64.RawURLEncoding.Strict().DecodeString(t.Token)
	if err != nil || len(secret) != SecretBytes {
		return invalid
	}
	return nil
}

func hostname(host string) bool {
	if len(host) > 253 {
		return false
	}
	host = strings.TrimSuffix(host, ".")
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func safeToken(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	for _, r := range value {
		if r <= 32 || r >= 127 || strings.ContainsRune("@/\\'\"`$;&|<>(){}[]!#?%", r) {
			return false
		}
	}
	return true
}

func ValidateCommand(command string) error {
	if !safeToken(strings.ReplaceAll(command, "/", "")) {
		return errors.New("invalid remote.command; use the remote tokeninsights executable path")
	}
	return nil
}
