// Package browser opens dashboards in the TokenInsights desktop viewer or the
// system browser.
package browser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Mode selects how a dashboard opens.
type Mode string

const (
	// ModeAuto prefers the desktop viewer and falls back to the browser.
	ModeAuto Mode = "auto"
	// ModeApp requires the desktop viewer.
	ModeApp Mode = "app"
	// ModeBrowser always uses the system browser.
	ModeBrowser Mode = "browser"
	// ModeOff prints the dashboard URL without opening anything.
	ModeOff Mode = "false"
)

// Desktop viewer identity shared with packages/desktop (mygo.json).
const (
	AppScheme   = "tokeninsights"
	appBundleID = "com.flexdinesh.tokeninsights"
)

// ErrAppUnavailable reports that --open=app found no desktop viewer.
var ErrAppUnavailable = errors.New("TokenInsights desktop app is not installed; install it or use --open=browser")

// ParseMode accepts auto, app, browser and false. true and the empty string mean auto.
func ParseMode(value string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "true", string(ModeAuto):
		return ModeAuto, nil
	case string(ModeApp):
		return ModeApp, nil
	case string(ModeBrowser):
		return ModeBrowser, nil
	case string(ModeOff):
		return ModeOff, nil
	}
	return "", fmt.Errorf("invalid open mode %q; use auto, app, browser or false", value)
}

// ModeFlag is a flag.Value for --open. A bare --open means auto.
type ModeFlag struct{ Mode Mode }

func (f *ModeFlag) String() string {
	if f == nil || f.Mode == "" {
		return string(ModeAuto)
	}
	return string(f.Mode)
}

func (f *ModeFlag) Set(value string) error {
	mode, err := ParseMode(value)
	if err != nil {
		return err
	}
	f.Mode = mode
	return nil
}

// IsBoolFlag lets the flag package accept a bare --open.
func (*ModeFlag) IsBoolFlag() bool { return true }

// AppLink returns the desktop viewer link for a dashboard URL.
func AppLink(dashboardURL string) string {
	return AppScheme + "://open?" + url.Values{"url": {dashboardURL}}.Encode()
}

// Via names what opened a dashboard.
type Via int

const (
	// ViaNone means nothing was opened.
	ViaNone Via = iota
	// ViaApp means the desktop viewer received the dashboard.
	ViaApp
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
	if mode == ModeAuto || mode == ModeApp {
		if s.openApp(ctx, AppLink(dashboardURL)) {
			return Result{Via: ViaApp}, nil
		}
		if mode == ModeApp {
			return Result{}, ErrAppUnavailable
		}
	}
	command := s.browserCommand(dashboardURL)
	if len(command) == 0 {
		return Result{}, nil
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
func (s system) openApp(ctx context.Context, link string) bool {
	switch s.platform {
	case "darwin":
		// open -b fails when no application has the bundle identifier.
		return s.run(ctx, "open", "-b", appBundleID, link) == nil
	case "linux":
		if !s.display() {
			return false
		}
		handler, err := s.output(ctx, "xdg-mime", "query", "default", "x-scheme-handler/"+AppScheme)
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
