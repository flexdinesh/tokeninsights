// Package browser opens dashboards in desktop or the system browser.
package browser

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Mode selects how a dashboard opens.
type Mode string

const (
	// ModeDesktop requires the desktop application.
	ModeDesktop Mode = "desktop"
	// ModeNative requires the experimental native desktop application.
	ModeNative Mode = "native"
	// ModeBrowser always uses the system browser.
	ModeBrowser Mode = "browser"
	// ModeOff prints the dashboard URL without opening anything.
	ModeOff Mode = "false"
)

// Desktop identities registered by the installers.
const (
	DesktopScheme  = "tokeninsights-desktop"
	appBundleID    = "com.flexdinesh.tokeninsights.desktop"
	NativeScheme   = "tokeninsights-native"
	nativeBundleID = "com.flexdinesh.tokeninsights.native"
)

// ErrAppUnavailable reports an unavailable desktop target.
var ErrAppUnavailable = errors.New("TokenInsights Desktop is unavailable; install the desktop product or run tokeninsights tui")

// ErrNativeUnavailable reports an unavailable experimental native target.
var ErrNativeUnavailable = errors.New("TokenInsights Native is unavailable; install it from the edge channel or omit --app=native")

// App identifies one installed desktop application. Launch links carry only
// its scheme; the application acknowledges with its Mode.
type App struct {
	Mode        Mode
	Scheme      string
	bundleID    string
	unavailable error
}

var (
	// DesktopApp is the default webview desktop application.
	DesktopApp = App{Mode: ModeDesktop, Scheme: DesktopScheme, bundleID: appBundleID, unavailable: ErrAppUnavailable}
	// NativeApp is the experimental native desktop application.
	NativeApp = App{Mode: ModeNative, Scheme: NativeScheme, bundleID: nativeBundleID, unavailable: ErrNativeUnavailable}
)

// AppFor selects exactly the named desktop application.
func AppFor(mode Mode) (App, error) {
	switch mode {
	case ModeDesktop:
		return DesktopApp, nil
	case ModeNative:
		return NativeApp, nil
	}
	return App{}, errors.New("desktop app must be desktop or native")
}

// Via names what opened a dashboard.
type Via int

const (
	// ViaNone means nothing was opened.
	ViaNone Via = iota
	// ViaBrowser means the system browser received the dashboard.
	ViaBrowser
)

// Result describes an Open call.
type Result struct {
	Via Via
	// Remote reports an SSH session: nothing opens on the server, and the
	// caller should print links for the user's own machine.
	Remote bool
}

// Open opens dashboardURL according to mode.
func Open(ctx context.Context, mode Mode, dashboardURL string) (Result, error) {
	return hostSystem().open(ctx, mode, dashboardURL)
}

type system struct {
	platform string
	getenv   func(string) string
	// run waits for a short-lived command, such as `open -b`.
	run func(ctx context.Context, name string, args ...string) error
	// output returns a short-lived command's standard output.
	output func(ctx context.Context, name string, args ...string) ([]byte, error)
	// start launches a command without waiting for it.
	start func(name string, args ...string) error
}

func hostSystem() system {
	return system{
		platform: runtime.GOOS,
		getenv:   os.Getenv,
		run: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
		output: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		start: func(name string, args ...string) error {
			command := exec.Command(name, args...)
			if err := command.Start(); err != nil {
				return err
			}
			// Some launchers wait for the browser to close. Reap them without blocking serving.
			go func() { _ = command.Wait() }()
			return nil
		},
	}
}

func (s system) open(ctx context.Context, mode Mode, dashboardURL string) (Result, error) {
	if mode == ModeOff {
		return Result{}, nil
	}
	if s.remote() {
		return Result{Remote: true}, nil
	}
	if mode != ModeBrowser {
		return Result{}, errors.New("desktop launch requires a private session handoff")
	}
	command := s.browserCommand(dashboardURL)
	if len(command) == 0 {
		return Result{}, errors.New("browser unavailable; use tokeninsights web --open=false")
	}
	if err := s.start(command[0], command[1:]...); err != nil {
		return Result{}, err
	}
	return Result{Via: ViaBrowser}, nil
}

// RemoteSession reports an SSH session, where dashboards never open on this host.
func RemoteSession() bool { return hostSystem().remote() }

func (s system) remote() bool {
	for _, key := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		if s.getenv(key) != "" {
			return true
		}
	}
	return false
}

func (s system) display() bool {
	return s.getenv("DISPLAY") != "" || s.getenv("WAYLAND_DISPLAY") != ""
}

// openApp hands link to app when it is registered.
func (s system) openApp(ctx context.Context, app App, link string) bool {
	switch s.platform {
	case "darwin":
		// open -b fails when no application has the bundle identifier.
		return s.run(ctx, "open", "-b", app.bundleID, link) == nil
	case "linux":
		if !s.display() {
			return false
		}
		handler, err := s.output(ctx, "xdg-mime", "query", "default", "x-scheme-handler/"+app.Scheme)
		if err != nil || strings.TrimSpace(string(handler)) == "" {
			return false
		}
		return s.start("xdg-open", link) == nil
	}
	return false
}

func (s system) browserCommand(dashboardURL string) []string {
	switch s.platform {
	case "darwin":
		return []string{"open", dashboardURL}
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", dashboardURL}
	case "linux":
		if s.display() {
			return []string{"xdg-open", dashboardURL}
		}
	}
	return nil
}

// Launch sends an opaque rendezvous link to the installed app.
// The caller must separately await the app's live-session acknowledgement.
func Launch(ctx context.Context, app App, link string) error {
	if hostSystem().openApp(ctx, app, link) {
		return nil
	}
	return app.unavailable
}
