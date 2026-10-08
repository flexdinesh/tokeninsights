package appstore

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRelativeApplicationPathRetainsPairAndSavedData(t *testing.T) {
	t.Chdir(t.TempDir())
	path := filepath.Join("relative #? directory", "app.sqlite")
	data := filepath.Join("relative #? directory", "data.duckdb")
	store, err := OpenPaired(t.Context(), path, data, "database", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQL().ExecContext(t.Context(), "UPDATE application_metadata SET legacy_accounts_imported=1 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenPaired(t.Context(), path, data, "database", "personal")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	var imported int
	if err := reopened.SQL().QueryRowContext(t.Context(), "SELECT legacy_accounts_imported FROM application_metadata WHERE id=1").Scan(&imported); err != nil || imported != 1 {
		t.Fatal("relative path did not reopen saved application", imported, err)
	}
}

func TestPairAndRoleMismatchDoNotMutateDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.sqlite")
	store, err := Open(t.Context(), path, "database-a", "hosted")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"database-b", "hosted"}, {"database-a", "personal"}} {
		if other, err := Open(t.Context(), path, pair[0], pair[1]); err == nil {
			_ = other.Close()
			t.Fatal("mismatch accepted")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(original, after) {
			t.Fatal("mismatch mutated database", err)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe application permissions", err)
	}
	wrong := filepath.Join(t.TempDir(), "collector.sqlite")
	if err := os.WriteFile(wrong, []byte("unrelated data"), 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := Open(t.Context(), wrong, "database-a", "hosted"); err == nil {
		_ = other.Close()
		t.Fatal("wrong role accepted")
	}
	after, _ := os.ReadFile(wrong)
	if string(after) != "unrelated data" {
		t.Fatal("wrong role changed")
	}
}

func TestAttachmentRejectsMissingOrReplacedApplicationWithoutCreatingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.sqlite")
	data := filepath.Join(root, "data.duckdb")
	store, err := OpenPaired(t.Context(), path, data, "database", "hosted")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "backup.sqlite")
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if other, err := OpenPaired(t.Context(), path, data, "database", "hosted"); err == nil {
		_ = other.Close()
		t.Fatal("lost application recreated")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("missing app mutated", err)
	}
	replacement, err := Open(t.Context(), path, "database", "hosted")
	if err != nil {
		t.Fatal(err)
	}
	_ = replacement.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := OpenPaired(t.Context(), path, data, "database", "hosted"); err == nil {
		_ = other.Close()
		t.Fatal("replacement app accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("replacement mutated", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, path); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenPaired(t.Context(), path, data, "database", "hosted")
	if err != nil {
		t.Fatal("matching restore rejected", err)
	}
	_ = restored.Close()
}
