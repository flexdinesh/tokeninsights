package desktoplaunch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
)

func TestLaunchRequiresDesktopAcknowledgementAndKeepsSecretsOffURL(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	received := make(chan *Handoff, 1)
	links := make(chan string, 1)
	finished := make(chan error, 1)
	spec := Spec{Connection: "private-token", SourceDir: "private-source", Capture: false}
	go func() {
		finished <- launch(ctx, browser.ModeElectron, spec, func(ctx context.Context, mode browser.Mode, raw string) error {
			if mode != browser.ModeElectron || !strings.HasPrefix(raw, "tokeninsights-electron://launch?id=") || strings.Contains(raw, "private") {
				return errors.New("secret or wrong app in OS link")
			}
			links <- raw
			go func() {
				handoff, err := Receive(ctx, raw)
				if err != nil {
					t.Error(err)
					cancel()
					return
				}
				received <- handoff
			}()
			return nil
		})
	}()
	handoff := <-received
	if handoff.Spec != spec {
		t.Fatal("private request changed", handoff.Spec)
	}
	select {
	case err := <-finished:
		t.Fatal("opener success returned before live-session ack", err)
	default:
	}
	if err := handoff.Acknowledge(browser.ModeElectron); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	raw := <-links
	id, err := ID(raw)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := rendezvousDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, id+".sock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rendezvous not removed", err)
	}
}

func TestFailedOrCancelledDesktopNeverReportsSuccess(t *testing.T) {
	for _, reject := range []bool{true, false} {
		ctx, cancel := context.WithCancel(t.Context())
		err := launch(ctx, browser.ModeMyGo, Spec{}, func(ctx context.Context, _ browser.Mode, raw string) error {
			if !reject {
				cancel()
				return nil
			}
			go func() {
				handoff, err := Receive(ctx, raw)
				if err != nil {
					t.Error(err)
					cancel()
					return
				}
				_ = handoff.Close()
			}()
			return nil
		})
		cancel()
		if err == nil {
			t.Fatal("failed desktop reported success")
		}
	}
}
func TestUnavailableApplicationPreservesActionableError(t *testing.T) {
	err := launch(t.Context(), browser.ModeMyGo, Spec{}, func(context.Context, browser.Mode, string) error { return browser.ErrAppUnavailable })
	if !errors.Is(err, browser.ErrAppUnavailable) {
		t.Fatal(err)
	}
}

func TestWrongDesktopCannotAcknowledgeSelectedApplication(t *testing.T) {
	err := launch(t.Context(), browser.ModeMyGo, Spec{}, func(ctx context.Context, _ browser.Mode, raw string) error {
		go func() {
			handoff, err := Receive(ctx, raw)
			if err != nil {
				t.Error(err)
				return
			}
			_ = handoff.Acknowledge(browser.ModeElectron)
		}()
		return nil
	})
	if err == nil {
		t.Fatal("wrong desktop satisfied requested application")
	}
}
func TestRendezvousRejectsCredentialAndDuplicateURLFields(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, raw := range []string{
		"tokeninsights-mygo://launch?id=" + id + "&id=" + id,
		"tokeninsights-mygo://launch?id=" + id + "&token=secret",
		"tokeninsights-mygo://launch?id=" + id + "#secret",
		"tokeninsights-mygo://user@launch?id=" + id,
		"tokeninsights-mygo://launch?id=../socket",
		"tokeninsights://connect/v1?address=host:1#token=secret",
		"tokeninsights-mygo://open?url=http://localhost",
	} {
		if _, err := ID(raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	if _, err := Receive(t.Context(), "tokeninsights-mygo://launch?id="+id); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatal(err)
	}
}
