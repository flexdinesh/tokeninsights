package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
)

func TestBareDesktopAndExplicitConnectionCarryExactPolicy(t *testing.T) {
	old := launchDesktop
	defer func() { launchDesktop = old }()
	t.Setenv("SSH_CONNECTION", "192.0.2.1 123 192.0.2.2 22")
	path := writeConfigForTest(t, nil)
	for _, test := range []struct {
		args    []string
		target  string
		capture bool
	}{
		{nil, "local", true},
		{[]string{"--capture=false"}, "local", false},
		{[]string{"connect", "ssh://alice@host:2222", "--source-dir=/remote/logs"}, "ssh://alice@host:2222", true},
	} {
		called := false
		launchDesktop = func(_ context.Context, spec desktoplaunch.Spec) error {
			called = true
			if spec.Connection != test.target || spec.Capture != test.capture {
				t.Fatal(spec)
			}
			return nil
		}
		if err := Run(t.Context(), append([]string{"--config-file", path}, test.args...), io.Discard, io.Discard, time.Now()); err != nil || !called {
			t.Fatal(called, err)
		}
	}
}

func TestUnavailableDesktopHasNoFallback(t *testing.T) {
	old := launchDesktop
	defer func() { launchDesktop = old }()
	launchDesktop = func(context.Context, desktoplaunch.Spec) error { return browser.ErrAppUnavailable }
	path := writeConfigForTest(t, nil)
	err := Run(t.Context(), []string{"--config-file", path}, io.Discard, io.Discard, time.Now())
	if !errors.Is(err, browser.ErrAppUnavailable) {
		t.Fatal(err)
	}
}

func TestInvalidConnectionAndCaptureFlagsNeverLaunch(t *testing.T) {
	old := launchDesktop
	defer func() { launchDesktop = old }()
	launchDesktop = func(context.Context, desktoplaunch.Spec) error {
		t.Fatal("invalid request launched desktop")
		return nil
	}
	path := writeConfigForTest(t, nil)
	for _, args := range [][]string{
		{"connect", "https://user:private-secret@example.test"},
		{"connect", "ssh://host", "--connection=local"},
		{"--capture=false", "--full-refresh"},
		{"--open=mygo"},
		{"--sync=false"},
	} {
		err := Run(t.Context(), append([]string{"--config-file", path}, args...), io.Discard, io.Discard, time.Now())
		if err == nil || strings.Contains(err.Error(), "private-secret") {
			t.Fatal(args, err)
		}
	}
}

func TestConnectCommandIsCompletelyShellQuoted(t *testing.T) {
	if got := shellQuote("secret'with space&x"); got != "'secret'\\''with space&x'" {
		t.Fatal(got)
	}
}

func TestRemoteInstructionsKeepStdoutToConnectCommand(t *testing.T) {
	target := endpoint.TCP{Address: "10.0.1.151:37619", Pin: strings.Repeat("ab", 32), Token: base64.RawURLEncoding.EncodeToString(make([]byte, endpoint.SecretBytes))}
	var stdout, stderr strings.Builder
	if err := writeRemoteInstructions(&stdout, &stderr, target.Address, target.ConnectionString()); err != nil {
		t.Fatal(err)
	}
	line, ok := strings.CutSuffix(stdout.String(), "\n")
	if !ok || strings.Contains(line, "\n") {
		t.Fatalf("stdout must be one line: %q", stdout.String())
	}
	quoted, ok := strings.CutPrefix(line, "tokeninsights connect ")
	if !ok || !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
		t.Fatalf("stdout must be a connect command with a quoted URI: %q", line)
	}
	parsed, err := endpoint.Parse(strings.Trim(quoted, "'"))
	if err != nil || parsed.TCP == nil || *parsed.TCP != target {
		t.Fatal("printed connection does not round trip", err)
	}
	guidance := stderr.String()
	for _, want := range []string{target.Address, "On the viewing machine, run:", "tokeninsights tui|web --connection", "Keep this secret", "Ctrl+C"} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("stderr missing %q: %q", want, guidance)
		}
	}
	if strings.Contains(guidance, target.Token) || strings.Contains(guidance, target.Pin) {
		t.Fatal("stderr guidance leaked credentials")
	}
}

func TestRemoteAdvertiseHintUsesOnlySSHServerAddress(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "192.0.2.10 4444 192.0.2.20 22")
	if got := sshAddressHint(); got != "192.0.2.20" {
		t.Fatal(got)
	}
	for _, value := range []string{"", "192.0.2.10 4444 secret 22", "192.0.2.10 4444 0.0.0.0 22", "192.0.2.20"} {
		t.Setenv("SSH_CONNECTION", value)
		if got := sshAddressHint(); got != "" {
			t.Fatal("invalid SSH address hint accepted", got)
		}
	}
}
