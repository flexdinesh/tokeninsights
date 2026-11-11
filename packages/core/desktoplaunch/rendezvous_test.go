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
		finished <- launch(ctx, browser.DesktopApp, spec, func(ctx context.Context, raw string) error {
			if !strings.HasPrefix(raw, "tokeninsights-desktop://launch?id=") || strings.Contains(raw, "private") {
				return errors.New("secret or wrong app in OS link")
			}
			links <- raw
			go func() {
				handoff, err := Receive(ctx, browser.DesktopApp, raw)
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
	if err := handoff.Acknowledge(browser.ModeDesktop); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	raw := <-links
	id, err := ID(browser.DesktopApp, raw)
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
		err := launch(ctx, browser.DesktopApp, Spec{}, func(ctx context.Context, raw string) error {
			if !reject {
				cancel()
				return nil
			}
			go func() {
				handoff, err := Receive(ctx, browser.DesktopApp, raw)
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
	err := launch(t.Context(), browser.DesktopApp, Spec{}, func(context.Context, string) error { return browser.ErrAppUnavailable })
	if !errors.Is(err, browser.ErrAppUnavailable) {
		t.Fatal(err)
	}
}

func TestWrongDesktopCannotAcknowledgeSelectedApplication(t *testing.T) {
	err := launch(t.Context(), browser.DesktopApp, Spec{}, func(ctx context.Context, raw string) error {
		go func() {
			handoff, err := Receive(ctx, browser.DesktopApp, raw)
			if err != nil {
				t.Error(err)
				return
			}
			_ = handoff.Acknowledge(browser.ModeBrowser)
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
		"tokeninsights-desktop://launch?id=" + id + "&id=" + id,
		"tokeninsights-desktop://launch?id=" + id + "&token=secret",
		"tokeninsights-desktop://launch?id=" + id + "#secret",
		"tokeninsights-desktop://user@launch?id=" + id,
		"tokeninsights-desktop://launch?id=../socket",
		"tokeninsights://connect/v1?address=host:1#token=secret",
		"tokeninsights-desktop://open?url=http://localhost",
	} {
		if _, err := ID(browser.DesktopApp, raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	if _, err := Receive(t.Context(), browser.DesktopApp, "tokeninsights-desktop://launch?id="+id); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatal(err)
	}
}

func TestNativeLaunchIsAddressedOnlyToNative(t *testing.T) {
	var link string
	err := launch(t.Context(), browser.NativeApp, Spec{Connection: "local"}, func(ctx context.Context, raw string) error {
		link = raw
		if _, err := ID(browser.DesktopApp, raw); err == nil {
			t.Error("desktop accepted native launch link")
		}
		go func() {
			handoff, err := Receive(ctx, browser.NativeApp, raw)
			if err != nil {
				t.Error(err)
				return
			}
			// The default desktop cannot acknowledge a native request.
			_ = handoff.Acknowledge(browser.ModeDesktop)
		}()
		return nil
	})
	if err == nil || !strings.HasPrefix(link, browser.NativeScheme+"://launch?id=") {
		t.Fatal(link, err)
	}
	err = launch(t.Context(), browser.NativeApp, Spec{Connection: "local"}, func(ctx context.Context, raw string) error {
		go func() {
			handoff, err := Receive(ctx, browser.NativeApp, raw)
			if err != nil {
				t.Error(err)
				return
			}
			_ = handoff.Acknowledge(browser.ModeNative)
		}()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
