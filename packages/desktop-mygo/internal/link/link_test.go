package link

import (
	"errors"
	"testing"
)

// The first case is the exact link packages/core/browser.AppLink writes.
func TestParseAcceptsCLILinks(t *testing.T) {
	cases := []struct {
		raw  string
		want Target
	}{
		{"tokeninsights-mygo://open?url=http%3A%2F%2F127.0.0.1%3A8765", Target{URL: "http://127.0.0.1:8765", Origin: "http://127.0.0.1:8765", Loopback: true}},
		{"tokeninsights://open?url=http%3A%2F%2F127.0.0.1%3A8765", Target{URL: "http://127.0.0.1:8765", Origin: "http://127.0.0.1:8765", Loopback: true}},
		{"tokeninsights://open/?url=http%3A%2F%2Flocalhost%3A9000%2F%3Fperiod%3Dweek", Target{URL: "http://localhost:9000/?period=week", Origin: "http://localhost:9000", Loopback: true}},
		{"TokenInsights://OPEN?url=https%3A%2F%2FUsage.Example.com%3A443%2F", Target{URL: "https://Usage.Example.com:443/", Origin: "https://usage.example.com", Loopback: false}},
		{"tokeninsights://open?url=http%3A%2F%2F%5B%3A%3A1%5D%3A8765", Target{URL: "http://[::1]:8765", Origin: "http://[::1]:8765", Loopback: true}},
		{"tokeninsights://open?url=http%3A%2F%2F192.168.1.4%3A80", Target{URL: "http://192.168.1.4:80", Origin: "http://192.168.1.4", Loopback: false}},
	}
	for _, tc := range cases {
		got, err := Parse(tc.raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("Parse(%q) = %+v, want %+v", tc.raw, got, tc.want)
		}
	}
}

func TestParseRejectsOtherLinks(t *testing.T) {
	cases := map[string]error{
		"tokeninsights-electron://open?url=http%3A%2F%2F127.0.0.1": ErrMalformed,
		"tokeninsights://open?url=http%3A%2F%2F127.0.0.1#extra":    ErrMalformed,
		"https://example.com":                                   ErrMalformed,
		"tokeninsights:open?url=http%3A%2F%2F127.0.0.1":         ErrMalformed,
		"tokeninsights://close?url=http%3A%2F%2F127.0.0.1":      ErrMalformed,
		"tokeninsights://open/extra?url=http%3A%2F%2F127.0.0.1": ErrMalformed,
		"tokeninsights://user@open?url=http%3A%2F%2F127.0.0.1":  ErrMalformed,
		"tokeninsights://open":                                  ErrMalformed,
		"tokeninsights://open?url=a&url=b":                      ErrMalformed,
		"tokeninsights://open?url=http%3A%2F%2F127.0.0.1&x=1":   ErrMalformed,
		"tokeninsights://open?url=file%3A%2F%2F%2Fetc%2Fpasswd": ErrTarget,
		"tokeninsights://open?url=javascript%3Aalert(1)":        ErrTarget,
		"tokeninsights://open?url=%2Frelative":                  ErrTarget,
		"tokeninsights://open?url=http%3A%2F%2Fu%3Ap%40host":    ErrTarget,
		"tokeninsights://open?url=http%3A%2F%2F":                ErrTarget,
	}
	for raw, want := range cases {
		if _, err := Parse(raw); !errors.Is(err, want) {
			t.Fatalf("Parse(%q) error = %v, want %v", raw, err, want)
		}
	}
}

func TestLoopbackDoesNotTrustLookalikes(t *testing.T) {
	for _, raw := range []string{"http://localhost.example.com", "http://127.0.0.1.nip.io", "http://10.0.0.1", "http://0.0.0.0:8765"} {
		target, err := Dashboard(raw)
		if err != nil {
			t.Fatalf("Dashboard(%q): %v", raw, err)
		}
		if target.Loopback {
			t.Fatalf("Dashboard(%q) is loopback", raw)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	if !SameOrigin("http://127.0.0.1:8765/sessions?page=2", "http://127.0.0.1:8765") {
		t.Fatal("same dashboard path rejected")
	}
	for _, raw := range []string{"http://127.0.0.1:8766/", "https://127.0.0.1:8765/", "about:blank", "http://evil.example"} {
		if SameOrigin(raw, "http://127.0.0.1:8765") {
			t.Fatalf("SameOrigin(%q) accepted", raw)
		}
	}
}
