package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBrowserCommand(t *testing.T) {
	const url = "http://localhost:8765"
	for _, test := range []struct {
		name, platform string
		env            map[string]string
		want           []string
	}{
		{"macOS", "darwin", nil, []string{"open", url}},
		{"Windows", "windows", nil, []string{"rundll32", "url.dll,FileProtocolHandler", url}},
		{"X11", "linux", map[string]string{"DISPLAY": ":0"}, []string{"xdg-open", url}},
		{"Wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"xdg-open", url}},
		{"headless", "linux", nil, nil},
		{"unsupported", "plan9", nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			getenv := func(key string) string { return test.env[key] }
			if got := browserCommand(test.platform, getenv, url); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("command = %v, want %v", got, test.want)
			}
		})
	}
}

func TestBrowserSkipsSSH(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		for _, sshKey := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
			t.Run(platform+"/"+sshKey, func(t *testing.T) {
				getenv := func(key string) string {
					if key == sshKey || key == "DISPLAY" || key == "WAYLAND_DISPLAY" {
						return "present"
					}
					return ""
				}
				if got := browserCommand(platform, getenv, "http://localhost:8765"); got != nil {
					t.Fatalf("SSH session must not open browser: %v", got)
				}
			})
		}
	}
}

func TestServeOpensBrowserAfterStartup(t *testing.T) {
	path := fixture(t)
	for _, test := range []struct {
		name, host, wantHost string
		openErr              error
	}{
		{"default", "", "localhost", nil},
		{"explicit", "127.0.0.1", "127.0.0.1", nil},
		{"all interfaces", "0.0.0.0", "localhost", nil},
		{"launch failure", "", "localhost", errors.New("no browser launcher")},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var output, log bytes.Buffer
			calls := 0
			open := func(url string) error {
				defer cancel()
				calls++
				if !strings.HasPrefix(url, "http://"+test.wantHost+":") || strings.HasSuffix(url, ":0") {
					t.Errorf("browser URL must use reachable host and assigned port: %q", url)
				}
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/v1/instance", nil)
				if err != nil {
					t.Fatal(err)
				}
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					t.Fatalf("server must serve before opening browser: %v", err)
				}
				_ = response.Body.Close()
				if response.StatusCode != http.StatusOK {
					t.Errorf("server status = %d", response.StatusCode)
				}
				return test.openErr
			}
			if err := runServer(ctx, Options{DBPath: path, NoSync: true, Host: test.host, Port: 0}, &output, &log, open); err != nil {
				t.Fatalf("browser launch must not fail server: %v", err)
			}
			if calls != 1 {
				t.Fatalf("browser opens = %d, want 1", calls)
			}
			if test.openErr != nil && !strings.Contains(log.String(), "open http://localhost:") {
				t.Fatalf("launch failure must give manual URL: %q", log.String())
			}
			if test.openErr == nil && log.Len() != 0 {
				t.Fatalf("unexpected browser warning: %q", log.String())
			}
		})
	}
}

func TestServeDoesNotOpenBrowserWhenStartupFails(t *testing.T) {
	open := func(string) error {
		t.Fatal("failed startup must not open browser")
		return nil
	}
	if err := runServer(context.Background(), Options{Host: "invalid"}, io.Discard, io.Discard, open); err == nil {
		t.Fatal("invalid host must fail startup")
	}
}
