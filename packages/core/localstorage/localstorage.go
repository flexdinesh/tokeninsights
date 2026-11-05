// Package localstorage resets local SQLite stores written by older releases.
// Resetting moves files aside instead of deleting them, so history the new
// store cannot rebuild remains recoverable by hand.
package localstorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/collectordb"
	"github.com/flexdinesh/tokeninsights/packages/core/datastore"
	"github.com/flexdinesh/tokeninsights/packages/core/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/core/storagecompat"
	"github.com/flexdinesh/tokeninsights/packages/core/writerlock"
)

type Role int

const (
	Collector Role = iota
	Server
)

// ErrBusy reports another TokenInsights process holding a store during reset.
var ErrBusy = errors.New("local_storage_busy")

// Store is one inspected file. Reset marks storage written by an older release.
type Store struct {
	Role   Role
	Path   string
	Result storagecompat.Result
	Reset  bool
}

type Report struct {
	Stores []Store
}

// Moved records one file moved aside.
type Moved struct{ From, To string }

// RequireCurrent performs the non-interactive executable startup preflight.
// Viewer/session composition receives this policy as an injected callback.
func RequireCurrent(ctx context.Context, collectorPath, serverPath string) error {
	report, err := InspectLocal(ctx, collectorPath, serverPath)
	if err != nil {
		return err
	}
	if len(report.Blocked()) > 0 || report.NeedsReset() {
		return errors.New("local storage is incompatible; run tokeninsights data wait in a terminal for repair guidance; files unchanged")
	}
	return nil
}

// InspectLocal classifies the stores an in-process command opens.
func InspectLocal(ctx context.Context, collectorPath, serverPath string) (Report, error) {
	collector, err := inspectCollector(ctx, collectorPath)
	if err != nil {
		return Report{}, err
	}
	server, _, err := serverownership.Identify(serverPath)
	if err != nil {
		return Report{}, err
	}
	serverResult, err := datastore.InspectCompatibility(ctx, server)
	if err != nil {
		return Report{}, fmt.Errorf("inspect %s: %w", server, err)
	}
	return Report{Stores: []Store{
		collector,
		{Role: Server, Path: server, Result: serverResult, Reset: serverResult.Status == storagecompat.Older},
	}}, nil
}

func inspectCollector(ctx context.Context, collectorPath string) (Store, error) {
	path, _, err := serverownership.Identify(collectorPath)
	if err != nil {
		return Store{}, err
	}
	result, err := collectordb.InspectCompatibility(ctx, path)
	if err != nil {
		return Store{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	return Store{Role: Collector, Path: path, Result: result, Reset: result.Status == storagecompat.Older}, nil
}

// Blocked lists stores that only the user can resolve: newer, foreign or
// mismatched files. Any blocked store prevents every reset.
func (r Report) Blocked() []Store {
	var blocked []Store
	for _, store := range r.Stores {
		switch store.Result.Status {
		case storagecompat.Newer, storagecompat.Foreign, storagecompat.Mismatch:
			blocked = append(blocked, store)
		}
	}
	return blocked
}

// NeedsReset reports whether any store was written by an older release.
func (r Report) NeedsReset() bool {
	for _, store := range r.Stores {
		if store.Reset {
			return true
		}
	}
	return false
}

// LosesHistory reports whether collector evidence and token history reset
// together, leaving only retained harness logs to rebuild from.
func (r Report) LosesHistory() bool {
	collector, server := false, false
	for _, store := range r.Stores {
		collector = collector || store.Role == Collector && store.Reset
		server = server || store.Role == Server && store.Reset
	}
	return collector && server
}

// Backup names the file a store moves to, before any timestamp suffix.
func (s Store) Backup() string {
	return fmt.Sprintf("%s.v%d.bak", s.Path, s.Result.Found)
}

// Reset moves every planned store aside under the locks its owner uses.
// Callers must check Blocked first; blocked reports are rejected unchanged.
func Reset(ctx context.Context, report Report, now time.Time) ([]Moved, error) {
	if len(report.Blocked()) > 0 {
		return nil, errors.New("local storage reset blocked")
	}
	var moved []Moved
	for _, store := range report.Stores {
		if !store.Reset {
			continue
		}
		var files []Moved
		var err error
		if store.Role == Server {
			files, err = resetServer(ctx, store, now)
		} else {
			files, err = locked(ctx, store.Path, func() ([]Moved, error) { return moveAside(store, now) })
		}
		moved = append(moved, files...)
		if err != nil {
			return moved, err
		}
	}
	return moved, nil
}

// resetServer holds the same ownership exclusion as a running personal backend.
func resetServer(ctx context.Context, server Store, now time.Time) ([]Moved, error) {
	return locked(ctx, server.Path+".service.op", func() ([]Moved, error) {
		owner, held, err := serverownership.Lifetime(server.Path, true)
		if err != nil {
			return nil, err
		}
		if held {
			return nil, ErrBusy
		}
		defer func() { _ = owner.Close() }()
		return moveAside(server, now)
	})
}

func locked(ctx context.Context, lock string, move func() ([]Moved, error)) ([]Moved, error) {
	release, err := writerlock.Acquire(ctx, lock)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBusy, err)
	}
	defer release()
	return move()
}

// SQLite companions move first so a recreated database never adopts a stale WAL.
var companions = []string{"-wal", "-shm", ""}

func moveAside(store Store, now time.Time) ([]Moved, error) {
	if store.Result.Status == storagecompat.Missing {
		return nil, nil
	}
	target, err := available(store.Backup(), now)
	if err != nil {
		return nil, err
	}
	var moved []Moved
	for _, suffix := range companions {
		from, to := store.Path+suffix, target+suffix
		if _, err := os.Lstat(from); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return moved, err
		}
		if err := os.Rename(from, to); err != nil {
			return moved, err
		}
		moved = append(moved, Moved{From: from, To: to})
	}
	return moved, nil
}

// backupTimeLayout is a sortable, filename-safe UTC timestamp.
const backupTimeLayout = "20060102T150405Z"

// available keeps earlier backups: a taken name gains a UTC timestamp.
func available(backup string, now time.Time) (string, error) {
	for _, candidate := range []string{backup, backup + "." + now.UTC().Format(backupTimeLayout)} {
		free := true
		for _, suffix := range companions {
			if _, err := os.Lstat(candidate + suffix); err == nil {
				free = false
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
		if free {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("backup %s already exists", backup)
}
