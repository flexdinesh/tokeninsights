package preferences

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

func TestFirstRunStartsWithSystemAndPersistsAChoice(t *testing.T) {
	dir := t.TempDir()
	store, p, err := Open(dir)
	if err != nil || p != Default() || p.Appearance != theme.AppearanceSystem {
		t.Fatalf("first run: %+v, %v", p, err)
	}
	store.Stage(Preferences{Appearance: theme.AppearanceDark})
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, FileName))
	if err != nil || info.Mode().Perm() != fileMode {
		t.Fatalf("file %v, %v", info, err)
	}
	_, reopened, err := Open(dir)
	if err != nil || reopened != (Preferences{Version: Version, Appearance: theme.AppearanceDark}) {
		t.Fatalf("reopened %+v, %v", reopened, err)
	}
	assertOnlyFile(t, dir)
}

func TestUnrecognizedFilesFallBackToDefaultsAndStayUntouched(t *testing.T) {
	for name, content := range map[string]string{
		"invalid json":     `{"version":1,`,
		"future version":   `{"version":2,"appearance":"dark"}`,
		"unknown value":    `{"version":1,"appearance":"sepia"}`,
		"unknown field":    `{"version":1,"appearance":"dark","accent":"red"}`,
		"trailing content": `{"version":1,"appearance":"dark"} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, FileName)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			store, p, err := Open(dir)
			if err == nil || p != Default() {
				t.Fatalf("%+v, %v", p, err)
			}
			store.Stage(Preferences{Appearance: theme.AppearanceLight})
			if err := store.Flush(); !errors.Is(err, ErrPreserved) {
				t.Fatalf("flush over an unrecognized file: %v", err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != content {
				t.Fatalf("file changed to %q, %v", data, err)
			}
			assertOnlyFile(t, dir)
		})
	}
}

func TestUnreadableFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	// A directory where the file belongs cannot be read as one.
	if err := os.Mkdir(filepath.Join(dir, FileName), 0o700); err != nil {
		t.Fatal(err)
	}
	store, p, err := Open(dir)
	if err == nil || p != Default() {
		t.Fatalf("%+v, %v", p, err)
	}
	if err := store.Flush(); !errors.Is(err, ErrPreserved) {
		t.Fatal(err)
	}
}

func TestConcurrentFlushesLeaveTheLastStagedChoice(t *testing.T) {
	dir := t.TempDir()
	store, _, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var flushes sync.WaitGroup
	choices := []theme.Appearance{theme.AppearanceLight, theme.AppearanceDark, theme.AppearanceSystem, theme.AppearanceDark}
	for _, choice := range choices {
		store.Stage(Preferences{Appearance: choice})
		flushes.Go(func() {
			if err := store.Flush(); err != nil {
				t.Error(err)
			}
		})
	}
	flushes.Wait()
	_, p, err := Open(dir)
	if err != nil || p.Appearance != choices[len(choices)-1] {
		t.Fatalf("%+v, %v", p, err)
	}
	assertOnlyFile(t, dir)
}

func TestFlushWithoutChangesWritesNothing(t *testing.T) {
	dir := t.TempDir()
	store, _, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("flush without a staged value wrote a file: %v", err)
	}
}

func TestFailedWriteLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	store, _, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Renaming over a non-empty directory fails after the temporary file is written.
	if err := os.MkdirAll(filepath.Join(dir, FileName, "keep"), 0o700); err != nil {
		t.Fatal(err)
	}
	store.Stage(Preferences{Appearance: theme.AppearanceDark})
	if err := store.Flush(); err == nil {
		t.Fatal("flush over a directory succeeded")
	}
	assertOnlyFile(t, dir)
}

// assertOnlyFile fails when dir holds anything besides the preferences file,
// such as a temporary file left behind.
func assertOnlyFile(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != FileName {
			t.Errorf("unexpected %s in %s", entry.Name(), dir)
		}
	}
}
