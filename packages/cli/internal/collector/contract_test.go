package collector_test

import (
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func acceptanceSources(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "source")
	fixture := filepath.Join("..", "..", "testdata", "conformance", "collector-rebuild", "source")
	if err := filepath.WalkDir(fixture, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(fixture, path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o600)
	}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "opencode", "source.sql"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := sql.Open("sqlite", filepath.Join(root, "opencode", "opencode.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	return root
}
