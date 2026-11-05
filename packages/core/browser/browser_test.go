package browser

import (
	"context"
	"errors"
	"testing"
)

type fakeSystem struct {
	env       map[string]string
	appBundle bool
	handler   string
	calls     [][]string
}

func (f *fakeSystem) system(platform string) system {
	return system{platform: platform, getenv: func(key string) string { return f.env[key] },
		run: func(_ context.Context, name string, args ...string) error {
			f.calls = append(f.calls, append([]string{name}, args...))
			if !f.appBundle {
				return errors.New("unavailable")
			}
			return nil
		},
		output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			f.calls = append(f.calls, append([]string{name}, args...))
			return []byte(f.handler), nil
		},
		start: func(name string, args ...string) error {
			f.calls = append(f.calls, append([]string{name}, args...))
			return nil
		}}
}
func TestSelectedDesktopNeverFallsBack(t *testing.T) {
	fake := fakeSystem{}
	for _, mode := range []Mode{ModeMyGo, ModeElectron} {
		if fake.system("darwin").openApp(t.Context(), mode, "opaque-link") {
			t.Fatal("unavailable app opened")
		}
	}
	for _, call := range fake.calls {
		if len(call) < 3 || call[1] != "-b" {
			t.Fatal("browser fallback", call)
		}
	}
}
func TestDistinctDesktopRegistrations(t *testing.T) {
	fake := fakeSystem{appBundle: true}
	if !fake.system("darwin").openApp(t.Context(), ModeMyGo, "mygo-link") || !fake.system("darwin").openApp(t.Context(), ModeElectron, "electron-link") {
		t.Fatal("desktop unavailable")
	}
	if fake.calls[0][2] != appBundleID || fake.calls[1][2] != electronBundleID {
		t.Fatal(fake.calls)
	}
}
func TestExplicitBrowserAndSSHBehavior(t *testing.T) {
	fake := fakeSystem{env: map[string]string{"DISPLAY": ":1"}}
	result, err := fake.system("linux").open(t.Context(), ModeBrowser, "http://127.0.0.1:1234/")
	if err != nil || result.Via != ViaBrowser || len(fake.calls) != 1 {
		t.Fatal(result, err, fake.calls)
	}
	fake = fakeSystem{env: map[string]string{"SSH_CONNECTION": "remote"}}
	result, err = fake.system("linux").open(t.Context(), ModeBrowser, "http://127.0.0.1:1234/")
	if err != nil || !result.Remote || len(fake.calls) != 0 {
		t.Fatal(result, err, fake.calls)
	}
}
func TestOpenModes(t *testing.T) {
	for value, want := range map[string]Mode{"": ModeMyGo, "mygo": ModeMyGo, "electron": ModeElectron, "browser": ModeBrowser, "false": ModeOff} {
		mode, err := ParseMode(value)
		if err != nil || mode != want {
			t.Fatal(value, mode, err)
		}
	}
	for _, value := range []string{"auto", "app", "unknown"} {
		if _, err := ParseMode(value); err == nil {
			t.Fatal("accepted", value)
		}
	}
}
