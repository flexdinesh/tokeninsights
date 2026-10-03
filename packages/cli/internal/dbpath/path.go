package dbpath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
