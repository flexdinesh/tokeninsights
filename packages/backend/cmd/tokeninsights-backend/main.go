package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
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
	if (len(args) != 2 && len(args) != 4) || args[0] != "connect" || args[1] != "--json" || len(args) == 4 && args[2] != "--url" {
		return errors.New("usage: tokeninsights-backend connect --json [--url URL]")
	}
	session, err := connect(ctx, args)
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		URL             string `json:"url"`
		InstanceID      string `json:"instanceId"`
		ProtocolVersion int    `json:"protocolVersion"`
	}{session.URL, session.InstanceID, localprotocol.ProtocolVersion}); err != nil {
		return err
	}
	closed := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, os.Stdin); close(closed) }()
	select {
	case <-ctx.Done():
		return nil
	case <-closed:
		return nil
	case <-session.Done():
		return errors.New("local backend disconnected")
	}
}

func connect(ctx context.Context, args []string) (*localclient.Session, error) {
	if len(args) == 4 {
		// The originating launcher already applied capture/source policy.
		return localclient.ConnectEndpoint(ctx, args[3], false)
	}
	path, err := config.Path("")
	if err != nil {
		return nil, err
	}
	settings, err := config.ResolveLocal(path, true, config.Overrides{})
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return localclient.Connect(ctx, localclient.Options{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath, AppDBPath: settings.ApplicationPath(), Host: settings.Host, Port: settings.Port, BackendPath: executable, Capture: true})
}
