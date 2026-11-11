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
func TestUnavailableDesktopNeverFallsBack(t *testing.T) {
	fake := fakeSystem{}
	if fake.system("darwin").openApp(t.Context(), DesktopApp, "opaque-link") {
		t.Fatal("unavailable app opened")
	}
	for _, call := range fake.calls {
		if len(call) < 3 || call[1] != "-b" {
			t.Fatal("browser fallback", call)
		}
	}
}
func TestDesktopRegistration(t *testing.T) {
	fake := fakeSystem{appBundle: true}
	if !fake.system("darwin").openApp(t.Context(), DesktopApp, "desktop-link") {
		t.Fatal("desktop unavailable")
	}
	if fake.calls[0][2] != appBundleID {
		t.Fatal(fake.calls)
	}
}
func TestNativeRegistrationIsSeparate(t *testing.T) {
	fake := fakeSystem{appBundle: true}
	if !fake.system("darwin").openApp(t.Context(), NativeApp, "native-link") || fake.calls[0][2] != nativeBundleID {
		t.Fatal(fake.calls)
	}
	fake = fakeSystem{env: map[string]string{"DISPLAY": ":1"}}
	if fake.system("linux").openApp(t.Context(), NativeApp, "native-link") {
		t.Fatal("unregistered native app opened")
	}
	if len(fake.calls) != 1 || fake.calls[0][3] != "x-scheme-handler/"+NativeScheme {
		t.Fatal(fake.calls)
	}
}
func TestAppSelectionIsExact(t *testing.T) {
	for mode, want := range map[Mode]App{ModeDesktop: DesktopApp, ModeNative: NativeApp} {
		if app, err := AppFor(mode); err != nil || app != want {
			t.Fatal(mode, app, err)
		}
	}
	for _, mode := range []Mode{ModeBrowser, ModeOff, "", "mygo"} {
		if _, err := AppFor(mode); err == nil {
			t.Fatal("accepted", mode)
		}
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
