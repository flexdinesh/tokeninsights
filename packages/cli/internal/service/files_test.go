package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServicePathsPrivateDirectories(t *testing.T) {
	for _, runtime := range []string{"xdg", "fallback"} {
		for _, existing := range []bool{false, true} {
			name := runtime + "/fresh"
			if existing {
				name = runtime + "/upgrade"
			}
			t.Run(name, func(t *testing.T) {
				options := environment(t)
				if runtime == "fallback" {
					t.Setenv("XDG_RUNTIME_DIR", "")
				}
				parents := []string{os.Getenv("XDG_CONFIG_HOME"), os.Getenv("XDG_STATE_HOME")}
				if runtime == "xdg" {
					parents = append(parents, os.Getenv("XDG_RUNTIME_DIR"))
				}
				for _, dir := range parents {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(dir, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				_, key, err := identify(options.DBPath)
				if err != nil {
					t.Fatal(err)
				}
				files, err := servicePaths(key, false)
				if err != nil {
					t.Fatal(err)
				}
				dirs := []string{filepath.Dir(files.config), filepath.Dir(files.log), filepath.Dir(files.record), filepath.Dir(files.fallbackRecord)}
				legacyLog := filepath.Join(filepath.Dir(files.log), "logs", "legacy.log")
				if existing {
					for _, dir := range dirs {
						if err := os.MkdirAll(dir, 0o755); err != nil {
							t.Fatal(err)
						}
						if err := os.Chmod(dir, 0o755); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.MkdirAll(filepath.Dir(legacyLog), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(legacyLog, []byte("legacy log"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := servicePaths(key, false); err != nil {
					t.Fatal(err)
				}
				for _, dir := range dirs {
					if !existing {
						if _, err := os.Stat(dir); !os.IsNotExist(err) {
							t.Fatalf("read-only lookup created %s: %v", dir, err)
						}
						continue
					}
					assertDirectoryMode(t, dir, 0o755)
				}
				if _, err := servicePaths(key, true); err != nil {
					t.Fatal(err)
				}
				for _, dir := range dirs {
					assertDirectoryMode(t, dir, 0o700)
				}
				if existing {
					content, err := os.ReadFile(legacyLog)
					if err != nil || string(content) != "legacy log" {
						t.Fatal("upgrade changed legacy log", err)
					}
				}
				for _, dir := range parents {
					assertDirectoryMode(t, dir, 0o755)
				}
			})
		}
	}
}

func TestOwnedDirRejectsSymlinksAndFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ownedDir(link, true); err == nil {
		t.Fatal("symlink accepted")
	}
	assertDirectoryMode(t, target, 0o755)
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ownedDir(file, true); err == nil {
		t.Fatal("regular file accepted")
	}
}

func assertDirectoryMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != want {
		t.Fatalf("%s: mode %v, want directory with mode %v", path, info.Mode(), want)
	}
}
