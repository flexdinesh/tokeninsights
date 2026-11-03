package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/core/browser"
)

func TestWebReadyRemoteOutputPrintsAppLinkAndTunnel(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("NO_COLOR", "")
	var output bytes.Buffer
	printWebReady(&output, "http://127.0.0.1:8765", true)
	text := output.String()
	for _, want := range []string{
		"Dashboard:  http://127.0.0.1:8765\n",
		"App:        tokeninsights://open?url=http%3A%2F%2F127.0.0.1%3A8765\n",
		"Tunnel:     ssh -L 8765:127.0.0.1:8765 <host>\n",
		"Press Ctrl+C to stop.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("remote output missing %q: %q", want, text)
		}
	}
	if text != ansi.Strip(text) {
		t.Fatalf("redirected output styled: %q", text)
	}

	output.Reset()
	printWebReady(&output, "http://192.168.1.5:8765", true)
	if text := output.String(); strings.Contains(text, "Tunnel:") || !strings.Contains(text, "App:        tokeninsights://open?url=http%3A%2F%2F192.168.1.5%3A8765") {
		t.Fatalf("non-loopback remote output = %q", text)
	}

	output.Reset()
	printWebReady(&output, "http://127.0.0.1:8765", false)
	if text := output.String(); strings.Contains(text, "App:") || strings.Contains(text, "Tunnel:") {
		t.Fatalf("local output has remote hints: %q", text)
	}
}

func TestWebReadyRemoteAppLinkIsHyperlinkOnStyledTerminals(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("NO_COLOR", "")
	var output bytes.Buffer
	printWebReady(&output, "http://127.0.0.1:8765", true)
	if want := ansi.SetHyperlink("tokeninsights://open?url=http%3A%2F%2F127.0.0.1%3A8765"); !strings.Contains(output.String(), want) {
		t.Fatalf("styled output lacks OSC 8 link: %q", output.String())
	}
}

func TestWebOpenModeUsesFlagThenConfigThenAuto(t *testing.T) {
	useSharedBackendForTest(t)
	for _, test := range []struct {
		name  string
		saved string
		args  []string
		want  browser.Mode
	}{
		{name: "default", want: browser.ModeAuto},
		{name: "config", saved: "browser", want: browser.ModeBrowser},
		{name: "flag over config", saved: "browser", args: []string{"--open=app"}, want: browser.ModeApp},
		{name: "bare flag", saved: "browser", args: []string{"--open"}, want: browser.ModeAuto},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			values := map[string]string{
				"collector.db-path":         filepath.Join(root, "collector.sqlite"),
				"in-process.server-db-path": filepath.Join(root, "server.sqlite"),
				"in-process.port":           strconv.Itoa(0),
			}
			if test.saved != "" {
				values["viewer.open"] = test.saved
			}
			path := writeConfigForTest(t, values)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			previousOpen, previousRemote := openDashboard, remoteSession
			t.Cleanup(func() { openDashboard, remoteSession = previousOpen, previousRemote })
			remoteSession = func() bool { return false }
			var got browser.Mode
			openDashboard = func(_ context.Context, mode browser.Mode, _ string) (browser.Result, error) {
				got = mode
				cancel()
				return browser.Result{Via: browser.ViaApp}, nil
			}
			var stderr bytes.Buffer
			args := append([]string{"--config-file", path, "web", "--sync=false"}, test.args...)
			if err := Run(ctx, args, &bytes.Buffer{}, &stderr, time.Now()); err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("open mode = %q, want %q", got, test.want)
			}
			if !strings.Contains(stderr.String(), "Opened in TokenInsights MyGo.") {
				t.Fatalf("app result not reported: %q", stderr.String())
			}
		})
	}
}

func TestBrowseOpenModesAndRemoteLink(t *testing.T) {
	path := writeConfigForTest(t, map[string]string{"distributed.server-url": "https://usage.example.com/", "viewer.open": "app"})
	previousOpen, previousRemote := openDashboard, remoteSession
	t.Cleanup(func() { openDashboard, remoteSession = previousOpen, previousRemote })
	var got browser.Mode
	openDashboard = func(_ context.Context, mode browser.Mode, _ string) (browser.Result, error) {
		got = mode
		return browser.Result{}, errors.New("browser launcher failed")
	}
	remoteSession = func() bool { return false }
	var stdout, stderr bytes.Buffer
	if err := Run(t.Context(), []string{"--config-file", path, "browse"}, &stdout, &stderr, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got != browser.ModeBrowser || stdout.String() != "Dashboard: https://usage.example.com\n" || !strings.Contains(stderr.String(), "Could not open browser") {
		t.Fatalf("mode %q stdout %q stderr %q", got, stdout.String(), stderr.String())
	}

	remoteSession = func() bool { return true }
	openDashboard = func(context.Context, browser.Mode, string) (browser.Result, error) {
		return browser.Result{Remote: true}, nil
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run(t.Context(), []string{"--config-file", path, "browse", "--open=browser"}, &stdout, &stderr, time.Now()); err != nil {
		t.Fatal(err)
	}
	if want := "Dashboard: https://usage.example.com\nApp: tokeninsights://open?url=https%3A%2F%2Fusage.example.com\n"; stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("remote browse stdout %q stderr %q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{"--config-file", path, "browse", "--open=false"}, &stdout, &stderr, time.Now()); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "Dashboard: https://usage.example.com\n" {
		t.Fatalf("--open=false printed remote link: %q", stdout.String())
	}

	if err := Run(t.Context(), []string{"--config-file", path, "browse", "--open=sometimes"}, &stdout, &stderr, time.Now()); !errors.Is(err, ErrUsage) {
		t.Fatal("invalid open mode accepted", err)
	}
}
