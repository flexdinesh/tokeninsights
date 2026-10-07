// Package service manages the local daemon and its private control socket.
package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"crypto/rand"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
	"golang.org/x/sys/unix"
)

const protocolVersion = 3
const bodyLimit = 256 * 1024
const callTimeout = 3 * time.Second
const startupTimeout = 45 * time.Second
const stopTimeout = 15 * time.Second

type Config struct {
	Version     int    `json:"version"`
	DBPath      string `json:"dbPath"`
	DatabaseKey string `json:"databaseKey"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Remote      bool   `json:"remote,omitempty"`
	Token       string `json:"token,omitempty"`
}

type Record struct {
	SchemaVersion int       `json:"schemaVersion"`
	Config        Config    `json:"config"`
	InstanceID    string    `json:"instanceId"`
	PID           int       `json:"pid"`
	Protocol      int       `json:"protocol"`
	Version       string    `json:"version"`
	URL           string    `json:"url"`
	Address       string    `json:"address"`
	Socket        string    `json:"socket"`
	StartedAt     time.Time `json:"startedAt"`
}

type paths struct{ config, record, socket, log, fallbackRecord string }

func identify(path string) (string, string, error) { return serverownership.Identify(path) }

// ownedDir verifies ownership and secures private application directories.
func ownedDir(path string, private bool) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("service directory is not a real directory: %s", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("service directory not owned by current user: %s", path)
	}
	if private && info.Mode().Perm() != 0o700 {
		if err := f.Chmod(0o700); err != nil {
			return fmt.Errorf("make service directory private: %s: %w", path, err)
		}
	}
	return nil
}

func servicePaths(key string, create bool) (paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return paths{}, err
	}
	base := func(env string, suffix string) (string, error) {
		value := os.Getenv(env)
		if value == "" {
			value = filepath.Join(home, suffix)
		}
		if !filepath.IsAbs(value) {
			return "", fmt.Errorf("%s must be absolute", env)
		}
		return value, nil
	}
	config, err := base("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return paths{}, err
	}
	state, err := base("XDG_STATE_HOME", ".local/state")
	if err != nil {
		return paths{}, err
	}
	config = filepath.Join(config, "tokeninsights", "services")
	state = filepath.Join(state, "tokeninsights")
	fallback := filepath.Join(state, "runtime")
	runtime := fallback
	if value := os.Getenv("XDG_RUNTIME_DIR"); value != "" {
		if !filepath.IsAbs(value) {
			return paths{}, fmt.Errorf("XDG_RUNTIME_DIR must be absolute")
		}
		if err := ownedDir(value, false); err == nil {
			runtime = filepath.Join(value, "tokeninsights")
		} else if !errors.Is(err, os.ErrNotExist) {
			return paths{}, err
		}
	}
	if create {
		for _, dir := range []string{config, state, runtime, fallback} {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return paths{}, err
			}
			if err := ownedDir(dir, true); err != nil {
				return paths{}, err
			}
		}
	}
	name := key[:32]
	p := paths{config: filepath.Join(config, name+".json"), record: filepath.Join(runtime, name+".json"), socket: filepath.Join(runtime, name+".sock"), log: filepath.Join(state, name+".log"), fallbackRecord: filepath.Join(fallback, name+".json")}
	if len(p.socket) >= 100 {
		return paths{}, fmt.Errorf("service socket path too long; use a shorter XDG_RUNTIME_DIR")
	}
	return p, nil
}

func decode(reader io.Reader, value interface{}) error {
	d := json.NewDecoder(io.LimitReader(reader, bodyLimit+1))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if err := d.Decode(new(interface{})); err != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	return nil
}

func readFile(path string, value interface{}) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > bodyLimit {
		return fmt.Errorf("invalid private service file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return decode(f, value)
}

func atomicFile(path string, value interface{}) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".service-*")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(value); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Lifetime probing is read-only and never removes lock inodes.
func lifetime(path string, create bool) (*os.File, bool, error) {
	return serverownership.Lifetime(path, create)
}

func admission(ctx context.Context, path string) (func(), error) {
	return db.AcquireWriterLock(ctx, path+".service.op")
}

func validateConfig(c Config) error {
	path, key, err := identify(c.DBPath)
	if err != nil {
		return err
	}
	if path != c.DBPath || key != c.DatabaseKey || c.Version != 3 || c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("invalid service configuration")
	}
	if c.Token != "" || c.Remote {
		return fmt.Errorf("legacy server authentication/deployment settings require explicit service restart")
	}
	return nil
}

func instanceID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
