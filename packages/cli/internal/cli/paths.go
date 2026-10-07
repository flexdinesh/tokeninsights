package cli

import (
	"os"
	"path/filepath"
	"strings"
)

func defaultCollectorDBPath() string {
	if value := strings.TrimSpace(os.Getenv("TOKENINSIGHTS_COLLECTOR_DB_PATH")); value != "" {
		return value
	}
	return filepath.Join(defaultDataPath(), "collector.sqlite")
}

func defaultServerDBPath() string {
	if value := strings.TrimSpace(os.Getenv("TOKENINSIGHTS_SERVER_DB_PATH")); value != "" {
		return value
	}
	return filepath.Join(defaultDataPath(), "server.duckdb")
}

func defaultServerURL() string { return strings.TrimSpace(os.Getenv("TOKENINSIGHTS_SERVER_URL")) }

func defaultDataPath() string {
	xdgDataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if xdgDataHome != "" {
		return filepath.Join(xdgDataHome, "tokeninsights")
	}

	home := strings.TrimSpace(os.Getenv("HOME"))
	if home != "" {
		return filepath.Join(home, ".local", "share", "tokeninsights")
	}

	cwd, err := os.Getwd()
	if err == nil && strings.TrimSpace(cwd) != "" {
		return filepath.Join(cwd, ".tokeninsights-data")
	}

	return filepath.Join(".", ".tokeninsights-data")
}
