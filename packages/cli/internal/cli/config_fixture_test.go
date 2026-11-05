package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/config"
)

// configurationEnvironment lists variables that outrank the config file.
var configurationEnvironment = []string{
	"TOKENINSIGHTS_COLLECTOR_DB_PATH", "TOKENINSIGHTS_SERVER_DB_PATH", "TOKENINSIGHTS_APP_DB_PATH",
	"TOKENINSIGHTS_HOST", "TOKENINSIGHTS_PORT", "TOKENINSIGHTS_SERVER_URL", "TOKENINSIGHTS_ACCESS_TOKEN",
}

// writeConfigForTest saves values through the production config file path and
// clears configuration environment so the file decides resolution.
func writeConfigForTest(t *testing.T, values map[string]string) string {
	t.Helper()
	for _, key := range configurationEnvironment {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Update(t.Context(), path, func(v *config.Values) error {
		for key, value := range values {
			if err := v.Set(key, value, false); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return path
}

func localConfigForTest(t *testing.T, settings config.LocalSettings) string {
	t.Helper()
	values := map[string]string{
		"collector.db-path":         settings.CollectorDBPath,
		"in-process.server-db-path": settings.ServerDBPath,
		"in-process.host":           settings.Host,
		"in-process.port":           strconv.Itoa(settings.Port),
	}
	return writeConfigForTest(t, values)
}
