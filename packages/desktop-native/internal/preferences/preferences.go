// Package preferences keeps the native app's app-wide preferences in one
// small JSON file. Reading never fails the app: a missing, unreadable or
// unrecognized file yields the defaults. A file the app did not recognize is
// left untouched, never replaced or deleted. Writes are atomic.
package preferences

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// FileName is the preferences file inside the app's user data directory.
const FileName = "preferences.json"

// Version is the file format this build reads and writes.
const Version = 1

// fileMode keeps preferences private to the user.
const fileMode fs.FileMode = 0o600

// Preferences are the app-wide choices that outlive a run.
type Preferences struct {
	Version    int              `json:"version"`
	Appearance theme.Appearance `json:"appearance"`
}

// Default is what a first run, or an unrecognized file, starts with.
func Default() Preferences {
	return Preferences{Version: Version, Appearance: theme.AppearanceSystem}
}

// ErrPreserved reports that the existing file was not recognized, so the
// store keeps it as it is and does not write over it.
var ErrPreserved = errors.New("preferences file was not recognized; leaving it unchanged")

// Store reads and writes the preferences file in one directory. Stage
// records a value on the caller's thread; Flush writes the latest staged
// value, so flushes running on worker goroutines in any order leave the
// last staged value on disk.
type Store struct {
	path     string
	preserve bool

	mu      sync.Mutex
	staged  Preferences
	stage   uint64
	writeMu sync.Mutex
	written uint64
}

// Open reads the preferences in dir. It always returns a usable store and
// preferences: on error they are the defaults, and the error says why.
func Open(dir string) (*Store, Preferences, error) {
	s := &Store{path: filepath.Join(dir, FileName)}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, Default(), nil
	}
	if err != nil {
		s.preserve = true
		return s, Default(), fmt.Errorf("read preferences: %w", err)
	}
	p, err := decode(data)
	if err != nil {
		s.preserve = true
		return s, Default(), fmt.Errorf("read preferences %s: %w", s.path, err)
	}
	return s, p, nil
}

// decode accepts only the current version's exact fields and values.
func decode(data []byte) (Preferences, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var p Preferences
	if err := decoder.Decode(&p); err != nil {
		return Preferences{}, err
	}
	if decoder.More() {
		return Preferences{}, errors.New("trailing data")
	}
	if p.Version != Version {
		return Preferences{}, fmt.Errorf("unsupported version %d", p.Version)
	}
	if !p.Appearance.Valid() {
		return Preferences{}, fmt.Errorf("unknown appearance %q", p.Appearance)
	}
	return p, nil
}

// Stage records p as the value the next Flush writes.
func (s *Store) Stage(p Preferences) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.Version = Version
	s.staged = p
	s.stage++
}

// Flush writes the latest staged value unless it is already on disk. It
// returns ErrPreserved, writing nothing, when Open did not recognize the
// existing file.
func (s *Store) Flush() error {
	if s.preserve {
		return ErrPreserved
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	p, stage := s.staged, s.stage
	s.mu.Unlock()
	if stage == s.written {
		return nil
	}
	if err := writeAtomic(s.path, p); err != nil {
		return err
	}
	s.written = stage
	return nil
}

// writeAtomic replaces path with p through a temporary file in the same
// directory, so readers see the old file or the new one, never a part.
func writeAtomic(path string, p Preferences) (err error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+FileName+"-*")
	if err != nil {
		return fmt.Errorf("write preferences: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(temp.Name())
		}
	}()
	_, err = temp.Write(append(data, '\n'))
	if err == nil {
		err = temp.Chmod(fileMode)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("write preferences: %w", err)
	}
	return nil
}
