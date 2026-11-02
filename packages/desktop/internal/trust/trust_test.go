package trust

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrustPersistsPrivately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "trusted-origins.json")
	store := New(path)
	if ok, err := store.Trusted("https://usage.example.com"); err != nil || ok {
		t.Fatalf("empty store Trusted = %v, %v", ok, err)
	}
	for _, origin := range []string{"https://usage.example.com", "http://10.0.0.2:8765", "https://usage.example.com"} {
		if err := store.Trust(origin); err != nil {
			t.Fatal(err)
		}
	}
	reopened := New(path)
	for _, origin := range []string{"https://usage.example.com", "http://10.0.0.2:8765"} {
		if ok, err := reopened.Trusted(origin); err != nil || !ok {
			t.Fatalf("Trusted(%q) = %v, %v", origin, ok, err)
		}
	}
	if ok, _ := reopened.Trusted("https://other.example.com"); ok {
		t.Fatal("unapproved origin trusted")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"origins\": [\n    \"http://10.0.0.2:8765\",\n    \"https://usage.example.com\"\n  ]\n}\n"; string(data) != want {
		t.Fatalf("file = %q, want %q", data, want)
	}
	assertMode(t, path, fileMode)
	assertMode(t, filepath.Dir(path), dirMode)
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("leftover temp files: %v, %v", entries, err)
	}
}

func TestCorruptFileIsReportedNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trusted-origins.json")
	for _, content := range []string{"{", `{"origins":[],"extra":1}`, `{"origins":[]} {}`} {
		if err := os.WriteFile(path, []byte(content), fileMode); err != nil {
			t.Fatal(err)
		}
		store := New(path)
		if _, err := store.Trusted("https://a.example"); err == nil {
			t.Fatalf("Trusted accepted %q", content)
		}
		if err := store.Trust("https://a.example"); err == nil {
			t.Fatalf("Trust overwrote %q", content)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != content {
			t.Fatalf("file changed to %q, %v", data, err)
		}
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %v, want %v", path, got, want)
	}
}
