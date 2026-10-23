// Package localstorage resets local SQLite stores written by older releases.
// Only local command compositions use it; hosted servers never reset storage.
// Resetting moves files aside instead of deleting them, so history the new
// store cannot rebuild remains recoverable by hand.
package localstorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectordb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dbpath"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecompat"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/writerlock"
)

type Role int

const (
	Collector Role = iota
	Server
	Application
	Jobs
)

// ErrBusy reports another TokenInsights process holding a store during reset.
var ErrBusy = errors.New("local_storage_busy")

// Store is one inspected file. Reset is true for older stores and for
// compatible stores paired with an older one.
type Store struct {
	Role   Role
	Path   string
	Result storagecompat.Result
	Reset  bool
}

type Report struct {
	Stores []Store
	// guard is the server/application pairing sidecar, moved with either half.
	guard string
}

// Moved records one file moved aside.
type Moved struct{ From, To string }

// InspectLocal classifies the stores an in-process command opens.
func InspectLocal(ctx context.Context, collectorPath, serverPath, appPath string) (Report, error) {
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
	app, err := dbpath.Canonical(appPath)
	if err != nil {
		return Report{}, err
	}
	appResult, err := appstore.InspectCompatibility(ctx, app, datastore.KindPersonal)
	if err != nil {
		return Report{}, fmt.Errorf("inspect %s: %w", app, err)
	}
	guard, err := appstore.GuardPath(server)
	if err != nil {
		return Report{}, err
	}
	serverOlder := serverResult.Status == storagecompat.Older
	// A new token store has a new database ID, so its application pair resets too.
	appReset := appResult.Status == storagecompat.Older || serverOlder && appResult.Status == storagecompat.Compatible
	return Report{guard: guard, Stores: []Store{
		collector,
		{Role: Server, Path: server, Result: serverResult, Reset: serverOlder},
		{Role: Application, Path: app, Result: appResult, Reset: appReset},
	}}, nil
}

// InspectSync classifies the stores a distributed sync client opens.
func InspectSync(ctx context.Context, collectorPath string) (Report, error) {
	collector, err := inspectCollector(ctx, collectorPath)
	if err != nil {
		return Report{}, err
	}
	path, result, err := syncjob.InspectCompatibility(ctx, collectorPath)
	if err != nil {
		return Report{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	return Report{Stores: []Store{collector, {Role: Jobs, Path: path, Result: result, Reset: result.Status == storagecompat.Older}}}, nil
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
	var server, app Store
	for _, store := range report.Stores {
		var files []Moved
		var err error
		switch {
		case store.Role == Server:
			server = store
		case store.Role == Application:
			app = store
		case !store.Reset:
		case store.Role == Collector:
			files, err = locked(ctx, store.Path, func() ([]Moved, error) { return moveAside(store, now) })
		case store.Role == Jobs:
			files, err = locked(ctx, syncjob.InitLock(store.Path), func() ([]Moved, error) { return moveAside(store, now) })
		}
		moved = append(moved, files...)
		if err != nil {
			return moved, err
		}
	}
	if server.Reset || app.Reset {
		files, err := resetPair(ctx, server, app, report.guard, now)
		moved = append(moved, files...)
		if err != nil {
			return moved, err
		}
	}
	return moved, nil
}

// resetPair moves the token store, application store and pairing guard under
// server ownership, the same exclusion local runtimes hold while open.
func resetPair(ctx context.Context, server, app Store, guard string, now time.Time) ([]Moved, error) {
	return locked(ctx, server.Path+".service.op", func() ([]Moved, error) {
		owner, held, err := serverownership.Lifetime(server.Path, true)
		if err != nil {
			return nil, err
		}
		if held {
			return nil, ErrBusy
		}
		defer func() { _ = owner.Close() }()
		var moved []Moved
		for _, store := range []Store{server, app} {
			if !store.Reset {
				continue
			}
			files, err := moveAside(store, now)
			moved = append(moved, files...)
			if err != nil {
				return moved, err
			}
		}
		// The guard takes the version of the store that triggered the reset.
		trigger := app
		if server.Reset {
			trigger = server
		}
		files, err := moveFile(guard, fmt.Sprintf("%s.v%d.bak", guard, trigger.Result.Found), now)
		return append(moved, files...), err
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

func moveFile(path, backup string, now time.Time) ([]Moved, error) {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	target, err := available(backup, now)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(path, target); err != nil {
		return nil, err
	}
	return []Moved{{From: path, To: target}}, nil
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
