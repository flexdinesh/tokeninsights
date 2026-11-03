package localclient

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"io"
	"os"
	"os/exec"
	"syscall"
)

func start(ctx context.Context, executable string, settings localprotocol.Settings) error {
	if executable == "" {
		var err error
		executable, err = BundledBackend()
		if err != nil {
			return err
		}
	}
	input, inputWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = input.Close(); _ = inputWriter.Close() }()
	ack, ackWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = ack.Close(); _ = ackWriter.Close() }()
	command := exec.Command(executable, "internal-serve")
	command.ExtraFiles = []*os.File{input, ackWriter}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return errors.New("cannot start bundled local backend")
	}
	_ = input.Close()
	_ = ackWriter.Close()
	// Neither settings nor credentials are exposed in argv or stdout.
	if err := json.NewEncoder(inputWriter).Encode(settings); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	_ = inputWriter.Close()
	type result struct {
		descriptor localprotocol.Descriptor
		err        error
	}
	ready := make(chan result, 1)
	go func() {
		var descriptor localprotocol.Descriptor
		err := json.NewDecoder(io.LimitReader(ack, 16384)).Decode(&descriptor)
		ready <- result{descriptor, err}
	}()
	select {
	case result := <-ready:
		if result.err == nil {
			result.err = result.descriptor.Compatible(settings)
		}
		if result.err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return errors.New("bundled local backend readiness failed")
		}
		go func() { _ = command.Wait() }()
		return nil
	case <-ctx.Done():
		_ = command.Process.Kill()
		_ = command.Wait()
		return ctx.Err()
	}
}
