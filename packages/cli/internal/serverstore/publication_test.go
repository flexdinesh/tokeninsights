package serverstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPublicationCrashHelper(t *testing.T) {
	path := os.Getenv("TOKENINSIGHTS_TEST_PUBLICATION_PATH")
	if path == "" {
		return
	}
	temporary := path + ".initialization"
	if err := os.WriteFile(temporary, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := initialize(temporary)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := publishDatabase(temporary, path); err != nil {
		t.Fatal(err)
	}
	// Simulate death at the first instruction after publication. No defer or
	// temporary-file cleanup may run; the published inode must have one name.
	_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
	t.Fatal("publication helper survived SIGKILL")
}

func TestPostPublicationCrashLeavesRestartableSingleLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sqlite")
	child := exec.Command(os.Args[0], "-test.run=^TestPublicationCrashHelper$")
	child.Env = append(os.Environ(), "TOKENINSIGHTS_TEST_PUBLICATION_PATH="+path)
	if output, err := child.CombinedOutput(); err == nil {
		t.Fatalf("expected killed child: %s", output)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		t.Fatalf("published database retained initialization alias: %+v", info.Sys())
	}
	if _, err := os.Stat(path + ".initialization"); !os.IsNotExist(err) {
		t.Fatalf("initialization alias survived: %v", err)
	}
	store, err := CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if _, err := store.Metadata(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestExclusivePublicationPreservesExistingTarget(t *testing.T) {
	root := t.TempDir()
	temporary, target := filepath.Join(root, "initialization"), filepath.Join(root, "existing")
	if err := os.WriteFile(temporary, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishDatabase(temporary, target); !os.IsExist(err) {
		t.Fatalf("publication did not reject existing history: %v", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "original" {
		t.Fatal("publication replaced existing target", err)
	}
}
