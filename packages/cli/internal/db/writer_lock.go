package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dbpath"
	"golang.org/x/sys/unix"
)

const WriterLockTimeout = 30 * time.Second
const writerLockPollInterval = 25 * time.Millisecond

// observeWriter holds an available lock through the caller's status snapshot.
// It never creates a sidecar or changes the database.
func observeWriter(path string) (bool, func(), error) {
	canonical, err := canonicalDBPath(path)
	if err != nil {
		return false, nil, err
	}
	file, err := os.Open(canonical + ".lock")
	if errors.Is(err, os.ErrNotExist) {
		return false, func() {}, nil
	}
	if err != nil {
		return false, nil, err
	}
	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		_ = file.Close()
		return true, func() {}, nil
	}
	if err != nil {
		_ = file.Close()
		return false, nil, err
	}
	return false, func() { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN); _ = file.Close() }, nil
}

// AcquireWriterLock serializes writers across processes. Callers must not nest
// acquisitions. The persistent sidecar must never be removed: flock locks its
// inode, and the kernel releases it when the owning process exits.
func AcquireWriterLock(ctx context.Context, path string) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, WriterLockTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	canonical, err := canonicalDBPath(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(canonical+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open database writer lock: %w", err)
	}
	ticker := time.NewTicker(writerLockPollInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("waiting for database writer lock: %w", err)
		}
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			var once sync.Once
			return func() {
				once.Do(func() {
					_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
					_ = file.Close()
				})
			}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EINTR) {
			_ = file.Close()
			return nil, fmt.Errorf("acquire database writer lock: %w", err)
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

func canonicalDBPath(path string) (string, error) { return dbpath.Canonical(path) }
