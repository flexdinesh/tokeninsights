package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
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
	if len(args) != 1 || args[0] != "internal-serve" {
		return errors.New("usage: tokeninsights-backend internal-serve")
	}
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
