package dbpath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolve existing symlinks, including parent-directory aliases for a database
// that has not been created yet.
func Canonical(path string) (string, error) {
	return canonical(path, make(map[string]bool))
}

func canonical(path string, seen map[string]bool) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if seen[abs] {
		return "", fmt.Errorf("database symlink cycle")
	}
	seen[abs] = true
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	// A dangling file symlink still names the same future database.
	if target, linkErr := os.Readlink(abs); linkErr == nil {
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(abs), target)
		}
		return canonical(target, seen)
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return "", err
	}
	resolvedParent, err := canonical(parent, seen)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(abs)), nil
}

// ConfigurationError names a fixed database path rejection without echoing paths.
type ConfigurationError struct{ Code string }

func (e *ConfigurationError) Error() string { return "configuration: " + e.Code }

// ValidateDistinct rejects missing paths and aliases between two database
// roles, including SQLite sidecars. It must precede creation of either role.
func ValidateDistinct(firstPath, secondPath string) error {
	if strings.TrimSpace(firstPath) == "" || strings.TrimSpace(secondPath) == "" {
		return &ConfigurationError{Code: "missing_database_path"}
	}
	first, err := Canonical(firstPath)
	if err != nil {
		return err
	}
	second, err := Canonical(secondPath)
	if err != nil {
		return err
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := distinct(first+suffix, second); err != nil {
			return err
		}
		if suffix != "" {
			if err := distinct(first, second+suffix); err != nil {
				return err
			}
		}
	}
	return nil
}

func distinct(first, second string) error {
	var err error
	first, err = Canonical(first)
	if err != nil {
		return err
	}
	second, err = Canonical(second)
	if err != nil {
		return err
	}
	if first == second {
		return &ConfigurationError{Code: "database_paths_alias"}
	}
	a, ae := os.Stat(first)
	b, be := os.Stat(second)
	if ae == nil && be == nil && os.SameFile(a, b) {
		return &ConfigurationError{Code: "database_paths_alias"}
	}
	if ae != nil && !errors.Is(ae, os.ErrNotExist) {
		return ae
	}
	if be != nil && !errors.Is(be, os.ErrNotExist) {
		return be
	}
	return nil
}
