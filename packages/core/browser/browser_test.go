package browser

import (
	"context"
	"errors"
	"flag"
	"io"
	"reflect"
	"testing"
)

const dashboard = "http://127.0.0.1:8765"

// The desktop viewer parses this exact link (packages/desktop-mygo/internal/link).
const dashboardLink = "tokeninsights://open?url=http%3A%2F%2F127.0.0.1%3A8765"

type fakeSystem struct {
	env       map[string]string
	appBundle bool   // macOS: open -b succeeds
	handler   string // Linux: xdg-mime default handler
	calls     [][]string
}

func (f *fakeSystem) system(platform string) system {
	return system{
		platform: platform,
		getenv:   func(key string) string { return f.env[key] },
		run: func(_ context.Context, name string, args ...string) error {
			f.calls = append(f.calls, append([]string{"run", name}, args...))
			if !f.appBundle {
				return errors.New("no application")
			}
			return nil
		},
		output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			f.calls = append(f.calls, append([]string{"output", name}, args...))
			return []byte(f.handler + "\n"), nil
		},
		start: func(name string, args ...string) error {
			f.calls = append(f.calls, append([]string{"start", name}, args...))
			return nil
		},
	}
}

func TestAppLink(t *testing.T) {
	if got := AppLink(dashboard); got != dashboardLink {
		t.Fatalf("AppLink = %q, want %q", got, dashboardLink)
	}
	if got := AppLink("https://usage.example.com/?a=1&b=2"); got != "tokeninsights://open?url=https%3A%2F%2Fusage.example.com%2F%3Fa%3D1%26b%3D2" {
		t.Fatalf("AppLink escaped query = %q", got)
	}
}

func TestOpen(t *testing.T) {
	x11 := map[string]string{"DISPLAY": ":0"}
	for _, test := range []struct {
		name, platform string
		mode           Mode
		fake           fakeSystem
		want           Result
		wantErr        error
		calls          [][]string
	}{
		{"macOS app", "darwin", ModeAuto, fakeSystem{appBundle: true}, Result{Via: ViaApp}, nil,
			[][]string{{"run", "open", "-b", appBundleID, DesktopLink(ModeMyGo, dashboard)}}},
		{"macOS fallback", "darwin", ModeAuto, fakeSystem{}, Result{Via: ViaBrowser}, nil,
			[][]string{{"run", "open", "-b", appBundleID, DesktopLink(ModeMyGo, dashboard)}, {"start", "open", dashboard}}},
		{"macOS app required", "darwin", ModeApp, fakeSystem{}, Result{}, ErrAppUnavailable,
			[][]string{{"run", "open", "-b", appBundleID, DesktopLink(ModeMyGo, dashboard)}}},
		{"macOS browser", "darwin", ModeBrowser, fakeSystem{appBundle: true}, Result{Via: ViaBrowser}, nil,
			[][]string{{"start", "open", dashboard}}},
		{"Linux app", "linux", ModeAuto, fakeSystem{env: x11, handler: "tokeninsights.desktop"}, Result{Via: ViaApp}, nil,
			[][]string{{"output", "xdg-mime", "query", "default", "x-scheme-handler/tokeninsights-mygo"}, {"start", "xdg-open", DesktopLink(ModeMyGo, dashboard)}}},
		{"Wayland fallback", "linux", ModeAuto, fakeSystem{env: map[string]string{"WAYLAND_DISPLAY": "wayland-0"}}, Result{Via: ViaBrowser}, nil,
			[][]string{{"output", "xdg-mime", "query", "default", "x-scheme-handler/tokeninsights-mygo"}, {"start", "xdg-open", dashboard}}},
		{"Linux headless", "linux", ModeAuto, fakeSystem{handler: "tokeninsights.desktop"}, Result{}, nil, nil},
		{"Linux headless app required", "linux", ModeApp, fakeSystem{handler: "tokeninsights.desktop"}, Result{}, ErrAppUnavailable, nil},
		{"Windows browser", "windows", ModeAuto, fakeSystem{}, Result{Via: ViaBrowser}, nil,
			[][]string{{"start", "rundll32", "url.dll,FileProtocolHandler", dashboard}}},
		{"unsupported", "plan9", ModeAuto, fakeSystem{}, Result{}, nil, nil},
		{"off", "darwin", ModeOff, fakeSystem{appBundle: true}, Result{}, nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := test.fake
			got, err := fake.system(test.platform).open(t.Context(), test.mode, dashboard)
			if !errors.Is(err, test.wantErr) || got != test.want {
				t.Fatalf("open = %+v, %v; want %+v, %v", got, err, test.want, test.wantErr)
			}
			if !reflect.DeepEqual(fake.calls, test.calls) {
				t.Fatalf("calls = %v, want %v", fake.calls, test.calls)
			}
		})
	}
}

func TestOpenSkipsSSH(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		for _, sshKey := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
			for _, mode := range []Mode{ModeAuto, ModeApp, ModeBrowser} {
				t.Run(platform+"/"+sshKey+"/"+string(mode), func(t *testing.T) {
					fake := fakeSystem{env: map[string]string{sshKey: "present", "DISPLAY": ":0"}, appBundle: true, handler: "tokeninsights.desktop"}
					got, err := fake.system(platform).open(t.Context(), mode, dashboard)
					if err != nil || got != (Result{Remote: true}) || len(fake.calls) != 0 {
						t.Fatalf("SSH session opened %v: %+v, %v", fake.calls, got, err)
					}
				})
			}
		}
	}
}

func TestModeFlag(t *testing.T) {
	for _, test := range []struct {
		args []string
		want Mode
	}{
		{nil, ModeMyGo},
		{[]string{"--open"}, ModeAuto},
		{[]string{"--open=true"}, ModeAuto},
		{[]string{"--open=false"}, ModeOff},
		{[]string{"--open=app"}, ModeApp},
		{[]string{"--open=mygo"}, ModeMyGo},
		{[]string{"--open=electron"}, ModeElectron},
		{[]string{"--open=BROWSER"}, ModeBrowser},
		{[]string{"-open=auto"}, ModeAuto},
	} {
		flags := flag.NewFlagSet("test", flag.ContinueOnError)
		value := ModeFlag{Mode: ModeMyGo}
		flags.Var(&value, "open", "")
		if err := flags.Parse(test.args); err != nil || value.Mode != test.want || flags.NArg() != 0 {
			t.Fatalf("%v = %q, %v; want %q", test.args, value.Mode, err, test.want)
		}
	}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var value ModeFlag
	flags.Var(&value, "open", "")
	if err := flags.Parse([]string{"--open=sometimes"}); err == nil {
		t.Fatal("invalid mode accepted")
	}
}

func TestExplicitDesktopTargetsUseSeparateRegistrations(t *testing.T) {
	for _, target := range []struct {
		mode           Mode
		scheme, bundle string
		via            Via
		unavailable    error
	}{
		{ModeMyGo, MyGoScheme, appBundleID, ViaApp, ErrAppUnavailable},
		{ModeElectron, ElectronScheme, electronBundleID, ViaElectron, ErrElectronUnavailable},
	} {
		for _, platform := range []string{"darwin", "linux"} {
			t.Run(platform+"/"+string(target.mode), func(t *testing.T) {
				fake := fakeSystem{env: map[string]string{"DISPLAY": ":0"}, appBundle: true, handler: "registered.desktop"}
				got, err := fake.system(platform).open(t.Context(), target.mode, dashboard)
				if err != nil || got.Via != target.via {
					t.Fatalf("open = %+v, %v", got, err)
				}
				link := target.scheme + "://open?url=http%3A%2F%2F127.0.0.1%3A8765"
				want := [][]string{{"run", "open", "-b", target.bundle, link}}
				if platform == "linux" {
					want = [][]string{{"output", "xdg-mime", "query", "default", "x-scheme-handler/" + target.scheme}, {"start", "xdg-open", link}}
				}
				if !reflect.DeepEqual(fake.calls, want) {
					t.Fatalf("calls %v, want %v", fake.calls, want)
				}
				fake = fakeSystem{env: map[string]string{"DISPLAY": ":0"}}
				if _, err := fake.system(platform).open(t.Context(), target.mode, dashboard); !errors.Is(err, target.unavailable) {
					t.Fatalf("unavailable = %v", err)
				}
				for _, call := range fake.calls {
					if call[0] == "start" {
						t.Fatalf("explicit target fell back: %v", fake.calls)
					}
				}
			})
		}
	}
}
