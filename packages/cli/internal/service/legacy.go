package service

import (
	"errors"
	"fmt"
	"os"
)

// LegacyBind exposes only bind preferences for explicit client-config migration.
// Credentials remain private and are never copied to the client config.
func LegacyBind(path string) (string, int, bool, error) {
	canonical, key, err := identify(path)
	if err != nil {
		return "", 0, false, err
	}
	files, err := servicePaths(key, false)
	if err != nil {
		return "", 0, false, err
	}
	var saved Config
	if err := readFile(files.config, &saved); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", 0, false, nil
		}
		return "", 0, false, err
	}
	if saved.DBPath != canonical || saved.DatabaseKey != key {
		return "", 0, false, fmt.Errorf("saved service database identity mismatch")
	}
	return saved.Host, saved.Port, saved.Version == 2, nil
}
