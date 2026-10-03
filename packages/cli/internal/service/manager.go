package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
)

type Options struct {
	DBPath        string
	Host          *string
	Port          *int
	ReloadSources bool
}

func configuration(options Options) (Config, error) {
	path, key, err := identify(options.DBPath)
	if err != nil {
		return Config{}, err
	}
	p, err := servicePaths(key, false)
	if err != nil {
		return Config{}, err
	}
	config := Config{Version: 1, DBPath: path, DatabaseKey: key, Host: server.DefaultHost, Port: server.DefaultPort}
	if err := readFile(p.config, &config); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	if config.DBPath != path || config.DatabaseKey != key {
		return Config{}, fmt.Errorf("saved service database identity mismatch")
	}
	if config.Sources == nil || options.ReloadSources {
		config.Sources, err = pipeline.ResolveSources("")
		if err != nil {
			return Config{}, err
		}
	}
	if options.Host != nil {
		config.Host = *options.Host
		if config.Host == "" {
			config.Host = server.DefaultHost
		}
	}
	if options.Port != nil {
		config.Port = *options.Port
	}
	if err := server.ValidateHost(config.Host); err != nil {
		return Config{}, err
	}
	return config, validateConfig(config)
}

func Ensure(ctx context.Context, options Options) (State, error) {
	path, _, err := identify(options.DBPath)
	if err != nil {
		return State{}, err
	}
	release, err := admission(ctx, path)
	if err != nil {
		return State{}, err
	}
	defer release()
	state, err := Probe(ctx, path)
	if err != nil {
		return state, err
	}
	if state.Running {
		if state.Record.ActionVersion != 1 || state.Record.SchemaVersion != db.SupportedSchemaVersion || state.Record.DataGeneration != db.CurrentDataGeneration {
			return state, fmt.Errorf("service contract incompatible; use service restart")
		}
		if options.Host != nil && normalizedHost(*options.Host) != state.Record.Config.Host || options.Port != nil && *options.Port != state.Record.Config.Port {
			return state, fmt.Errorf("service already running at %s; use service restart to change binding", state.Record.URL)
		}
		return state, nil
	}
	config, err := configuration(options)
	if err != nil {
		return State{}, err
	}
	return spawn(ctx, config)
}

func normalizedHost(host string) string {
	if host == "" {
		return server.DefaultHost
	}
	return host
}

func spawn(ctx context.Context, config Config) (State, error) {
	p, err := servicePaths(config.DatabaseKey, true)
	if err != nil {
		return State{}, err
	}
	log, err := os.OpenFile(p.log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return State{}, err
	}
	defer func() { _ = log.Close() }()
	inputRead, inputWrite, err := os.Pipe()
	if err != nil {
		return State{}, err
	}
	defer func() { _ = inputRead.Close(); _ = inputWrite.Close() }()
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		return State{}, err
	}
	defer func() { _ = readyRead.Close(); _ = readyWrite.Close() }()
	executable, err := os.Executable()
	if err != nil {
		return State{}, err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return State{}, err
	}
	command := exec.Command(executable, "__service-run")
	command.ExtraFiles = []*os.File{inputRead, readyWrite}
	command.Stdout = log
	command.Stderr = log
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return State{}, err
	}
	exited := make(chan struct{})
	go func() { _ = command.Wait(); close(exited) }()
	abort := func() {
		_ = command.Process.Kill()
		// Keep startup admission until our child exits and releases ownership.
		select {
		case <-exited:
		case <-time.After(callTimeout):
		}
	}
	_ = inputRead.Close()
	_ = readyWrite.Close()
	if err := json.NewEncoder(inputWrite).Encode(config); err != nil {
		abort()
		return State{}, err
	}
	_ = inputWrite.Close()
	type result struct {
		value ready
		err   error
	}
	response := make(chan result, 1)
	go func() { var value ready; err := decode(readyRead, &value); response <- result{value, err} }()
	startupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	select {
	case <-startupCtx.Done():
		abort()
		return State{}, fmt.Errorf("service startup: %w", startupCtx.Err())
	case r := <-response:
		if r.err != nil || r.value.Error != "" || r.value.Record == nil {
			abort()
			return State{}, fmt.Errorf("service startup failed: %s %v; logs: %s", r.value.Error, r.err, p.log)
		}
		client := Client{Record: *r.value.Record}
		status, err := client.Status(ctx)
		if err != nil {
			abort()
			return State{}, err
		}
		return State{Running: true, Record: r.value.Record, Status: &status}, nil
	}
}

func stop(ctx context.Context, path string) error {
	state, err := Probe(ctx, path)
	if err != nil {
		return err
	}
	if !state.Running {
		return nil
	}
	client := Client{Record: *state.Record}
	var receipt struct {
		Accepted bool `json:"accepted"`
	}
	if err := client.call(ctx, "POST", "/shutdown", nil, &receipt); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, stopTimeout)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		f, held, err := lifetime(state.Record.Config.DBPath, false)
		if f != nil {
			_ = f.Close()
		}
		if err != nil {
			return err
		}
		if !held {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("service shutdown timed out; ownership retained: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func Stop(ctx context.Context, path string) error {
	canonical, _, err := identify(path)
	if err != nil {
		return err
	}
	release, err := admission(ctx, canonical)
	if err != nil {
		return err
	}
	defer release()
	return stop(ctx, canonical)
}

func Restart(ctx context.Context, options Options) (State, error) {
	path, _, err := identify(options.DBPath)
	if err != nil {
		return State{}, err
	}
	release, err := admission(ctx, path)
	if err != nil {
		return State{}, err
	}
	defer release()
	config, err := configuration(options)
	if err != nil {
		return State{}, err
	}
	if err := stop(ctx, path); err != nil {
		return State{}, err
	}
	return spawn(ctx, config)
}

func Run(ctx context.Context, options Options, stdout, stderr io.Writer) error {
	path, _, err := identify(options.DBPath)
	if err != nil {
		return err
	}
	release, err := admission(ctx, path)
	if err != nil {
		return err
	}
	config, err := configuration(options)
	if err != nil {
		release()
		return err
	}
	defer release()
	return runtime(ctx, config, stderr, func(record Record) error { release(); _, err := fmt.Fprintln(stdout, record.URL); return err })
}
