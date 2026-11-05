package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/localstorage"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 1 && args[0] == "internal-serve" {
		input, ack := os.NewFile(3, "settings"), os.NewFile(4, "readiness")
		if input == nil || ack == nil {
			return errors.New("private startup pipes required")
		}
		defer func() { _ = input.Close(); _ = ack.Close() }()
		var settings localprotocol.Settings
		decoder := json.NewDecoder(io.LimitReader(input, 16384))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&settings); err != nil {
			return errors.New("invalid backend settings")
		}
		return localbackend.Serve(ctx, settings, os.Stderr, func(descriptor localprotocol.Descriptor) error {
			err := json.NewEncoder(ack).Encode(descriptor)
			_ = ack.Close()
			return err
		})
	}
	if (len(args) != 2 && len(args) != 4) || args[0] != "connect" || args[1] != "--json" || len(args) == 4 && args[2] != "--launch" {
		return errors.New("usage: tokeninsights-backend connect --json [--launch RENDEZVOUS_LINK]")
	}
	dashboard, handoff, err := connect(ctx, args)
	if err != nil {
		return err
	}
	defer func() { _ = dashboard.Close() }()
	if handoff != nil {
		defer func() { _ = handoff.Close() }()
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		URL             string `json:"url"`
		InstanceID      string `json:"instanceId"`
		ProtocolVersion int    `json:"protocolVersion"`
	}{dashboard.URL, dashboard.Session.InstanceID, localprotocol.ProtocolVersion}); err != nil {
		return err
	}
	// The Electron main process confirms it accepted the live connector before
	// acknowledging the CLI. stdin stays open for the window's entire lifetime.
	input := bufio.NewReader(os.Stdin)
	if handoff != nil {
		accepted := make(chan bool, 1)
		go func() { ready, err := input.ReadString('\n'); accepted <- err == nil && ready == "ready\n" }()
		timer := time.NewTimer(time.Until(handoff.Deadline()))
		defer timer.Stop()
		select {
		case <-timer.C:
			return errors.New("desktop acknowledgement timed out; run tokeninsights again")
		case ready := <-accepted:
			if !ready {
				return errors.New("desktop closed before accepting the requested session")
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-dashboard.Done():
			return errors.New("backend disconnected before desktop acknowledgement")
		}
		select {
		case <-dashboard.Done():
			return errors.New("backend disconnected before desktop acknowledgement")
		default:
		}
		if err := handoff.Acknowledge(browser.ModeElectron); err != nil {
			return err
		}
	}
	closed := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, input); close(closed) }()
	select {
	case <-ctx.Done():
		return nil
	case <-closed:
		return nil
	case <-dashboard.Done():
		return errors.New("backend disconnected")
	}
}

func connect(ctx context.Context, args []string) (*desktoplaunch.Dashboard, *desktoplaunch.Handoff, error) {
	var spec desktoplaunch.Spec
	var handoff *desktoplaunch.Handoff
	if len(args) == 4 {
		var err error
		handoff, err = desktoplaunch.Receive(ctx, args[3])
		if err != nil {
			return nil, nil, err
		}
		spec = handoff.Spec
	} else {
		path, err := config.Path("")
		if err != nil {
			return nil, nil, err
		}
		settings, err := config.ResolveLocal(path, true, config.Overrides{})
		if err != nil {
			return nil, nil, err
		}
		spec = desktoplaunch.Specification(settings, true, false, "")
	}
	executable, err := os.Executable()
	if err != nil {
		if handoff != nil {
			_ = handoff.Close()
		}
		return nil, nil, err
	}
	spec.BackendPath = executable
	dashboard, err := desktoplaunch.Open(ctx, spec, "127.0.0.1", 0, func() error {
		return localstorage.RequireCurrent(ctx, spec.CollectorDBPath, spec.ServerDBPath)
	})
	if err != nil && handoff != nil {
		_ = handoff.Close()
	}
	return dashboard, handoff, err
}
