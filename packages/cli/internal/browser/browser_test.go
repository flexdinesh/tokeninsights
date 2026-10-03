package browser

import (
	"reflect"
	"testing"
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
