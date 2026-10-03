package dbpath

import (
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
