//go:build linux || darwin

package pipeline

import (
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"os"
	"syscall"
)

func rawSourceSignature(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if native, ok := info.Sys().(*syscall.Stat_t); ok {
		return evidence.Tuple("quarantine-file", native.Dev, native.Ino, info.Size(), info.ModTime().UnixNano()), nil
	}
	return "", nil
}
