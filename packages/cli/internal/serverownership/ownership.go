// Package serverownership protects a canonical database across server compositions.
package serverownership

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dbpath"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func Identify(path string) (string, string, error) {
	if path == "" {
		return "", "", fmt.Errorf("empty database path")
	}
	canonical, err := dbpath.Canonical(path)
	if err != nil {
		return "", "", err
	}
	if info, err := os.Stat(canonical); err == nil {
		if !info.Mode().IsRegular() {
			return "", "", fmt.Errorf("database must be a regular file")
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink > 1 {
			return "", "", fmt.Errorf("hard-linked databases are unsupported")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return canonical, hex.EncodeToString(sum[:]), nil
}

func Lifetime(path string, create bool) (*os.File, bool, error) {
	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	if create {
		flags = unix.O_CREAT | unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC
	}
	fd, err := unix.Open(path+".service.lock", flags, 0o600)
	if errors.Is(err, os.ErrNotExist) && !create {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	f := os.NewFile(uintptr(fd), path+".service.lock")
	err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		_ = f.Close()
		return nil, true, nil
	}
	if err != nil {
		_ = f.Close()
		return nil, false, err
	}
	return f, false, nil
}
