package syncjob

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const startupTimeout = 5 * time.Second
const maxStartupBytes = 16 * 1024

type startup struct {
	CollectorPath string
	JobID         string
	Token         string
}

// Spawn acknowledges a live finite worker, not successful remote acceptance.
func Spawn(ctx context.Context, job Job, token string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return spawn(ctx, executable, job, token)
}
func spawn(ctx context.Context, executable string, job Job, token string) error {
	inputRead, inputWrite, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = inputRead.Close(); _ = inputWrite.Close() }()
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = readyRead.Close(); _ = readyWrite.Close() }()
	child := exec.Command(executable, "__sync-run")
	child.ExtraFiles = []*os.File{inputRead, readyWrite}
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Credentials travel through a private pipe, including when configured by env.
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "TOKENINSIGHTS_ACCESS_TOKEN=") && !strings.HasPrefix(entry, "TOKENINSIGHTS_SERVER_TOKEN=") {
			child.Env = append(child.Env, entry)
		}
	}
	if err := child.Start(); err != nil {
		return errors.New("sync_worker_start_failed")
	}
	exited := make(chan struct{})
	go func() { _ = child.Wait(); close(exited) }()
	abort := func() {
		_ = child.Process.Kill()
		select {
		case <-exited:
		case <-time.After(startupTimeout):
		}
	}
	_ = inputRead.Close()
	_ = readyWrite.Close()
	deadline, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	response := make(chan error, 1)
	go func() {
		if err := json.NewEncoder(inputWrite).Encode(startup{CollectorPath: job.Spec.CollectorPath, JobID: job.ID, Token: token}); err != nil {
			response <- err
			return
		}
		_ = inputWrite.Close()
		var ready bool
		err := json.NewDecoder(io.LimitReader(readyRead, maxStartupBytes)).Decode(&ready)
		if err == nil && !ready {
			err = errors.New("sync_worker_not_ready")
		}
		response <- err
	}()
	select {
	case <-deadline.Done():
		abort()
		return errors.New("sync_worker_start_timeout")
	case err := <-response:
		if err != nil {
			abort()
			return errors.New("sync_worker_start_failed")
		}
		return nil
	}
}
func Child(ctx context.Context) error {
	input, output := os.NewFile(3, "sync-configuration"), os.NewFile(4, "sync-readiness")
	if input == nil || output == nil {
		return errors.New("missing_sync_descriptors")
	}
	defer func() { _ = input.Close(); _ = output.Close() }()
	var request startup
	decoder := json.NewDecoder(io.LimitReader(input, maxStartupBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return errors.New("invalid_sync_startup")
	}
	store, err := Open(ctx, request.CollectorPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	job, err := store.Get(ctx, request.JobID)
	if err != nil {
		return err
	}
	if job.Spec.Credential != Fingerprint(request.Token) {
		return errors.New("sync_configuration_changed")
	}
	if err := json.NewEncoder(output).Encode(true); err != nil {
		return err
	}
	_ = output.Close()
	_ = input.Close()
	run, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	_, err = RunRemote(run, store, job, request.Token, false, Progress{})
	return err
}
