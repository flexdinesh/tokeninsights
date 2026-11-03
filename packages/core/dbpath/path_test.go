package dbpath

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDanglingSymlinkCycleAndParentAlias(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	if err := os.Symlink(b, a); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := Canonical(a); err == nil {
		t.Fatal("cycle accepted")
	}
	parent := filepath.Join(root, "alias")
	if err := os.Symlink(root, parent); err != nil {
		t.Fatal(err)
	}
	got, err := Canonical(filepath.Join(parent, "missing", "usage.sqlite"))
	if err != nil || got != filepath.Join(root, "missing", "usage.sqlite") {
		t.Fatal("absent-file identity lost", got, err)
	}
}

func TestValidateDistinctRejectsMissingAndSidecarAliases(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(root, "server.sqlite")
	cases := []struct {
		name, first, second, code string
	}{
		{"missing", "", database, "missing_database_path"},
		{"same", database, database, "database_paths_alias"},
		{"wal sidecar", database, database + "-wal", "database_paths_alias"},
		{"journal sidecar", database + "-journal", database, "database_paths_alias"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDistinct(tc.first, tc.second)
			var configuration *ConfigurationError
			if !errors.As(err, &configuration) || configuration.Code != tc.code || err.Error() != "configuration: "+tc.code {
				t.Fatalf("got %v, want code %s", err, tc.code)
			}
		})
	}
	if err := ValidateDistinct(database, filepath.Join(root, "app.sqlite")); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatal("validation mutated storage", entries, err)
	}
}
