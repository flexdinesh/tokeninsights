// Package browser opens dashboards in MyGo, Electron or the system browser.
package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Mode selects how a dashboard opens.
type Mode string

const (
	// ModeMyGo requires the MyGo application.
	ModeMyGo Mode = "mygo"
	// ModeElectron requires the Electron application.
	ModeElectron Mode = "electron"
	// ModeBrowser always uses the system browser.
	ModeBrowser Mode = "browser"
	// ModeOff prints the dashboard URL without opening anything.
	ModeOff Mode = "false"
)

// Desktop identities remain distinct so both applications can coexist.
const (
	MyGoScheme       = "tokeninsights-mygo"
	ElectronScheme   = "tokeninsights-electron"
	appBundleID      = "com.flexdinesh.tokeninsights.mygo"
	electronBundleID = "com.flexdinesh.tokeninsights.electron"
)

// ErrAppUnavailable reports an unavailable explicit MyGo target.
var ErrAppUnavailable = errors.New("TokenInsights MyGo is unavailable; install the MyGo product or run tokeninsights tui")

// ErrElectronUnavailable reports an unavailable explicit Electron target.
var ErrElectronUnavailable = errors.New("TokenInsights Electron is unavailable; install the Electron product or run tokeninsights tui")

// ParseMode accepts explicit desktop, browser and saved-URL modes.
func ParseMode(value string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "true":
		return ModeMyGo, nil
	case string(ModeMyGo):
		return ModeMyGo, nil
	case string(ModeElectron):
		return ModeElectron, nil
	case string(ModeBrowser):
		return ModeBrowser, nil
	case string(ModeOff):
		return ModeOff, nil
	}
	return "", fmt.Errorf("invalid open mode %q; use mygo, electron, browser or false", value)
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

// openApp hands link to the desktop viewer when one is registered.
func (s system) openApp(ctx context.Context, mode Mode, link string) bool {
	bundleID, scheme := appBundleID, MyGoScheme
	if mode == ModeElectron {
		bundleID, scheme = electronBundleID, ElectronScheme
	}
	switch s.platform {
	case "darwin":
		// open -b fails when no application has the bundle identifier.
		return s.run(ctx, "open", "-b", bundleID, link) == nil
	case "linux":
		if !s.display() {
			return false
		}
		handler, err := s.output(ctx, "xdg-mime", "query", "default", "x-scheme-handler/"+scheme)
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

// Launch sends an opaque rendezvous link to the selected installed desktop.
// The caller must separately await the desktop's live-session acknowledgement.
func Launch(ctx context.Context, mode Mode, link string) error {
	if mode != ModeMyGo && mode != ModeElectron {
		return errors.New("desktop must be mygo or electron")
	}
	if hostSystem().openApp(ctx, mode, link) {
		return nil
	}
	if mode == ModeElectron {
		return ErrElectronUnavailable
	}
	return ErrAppUnavailable
}
