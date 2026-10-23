package cli

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectordb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
)

func nonInteractiveStorage(t *testing.T) {
	t.Helper()
	previous := storagePromptInteractive
	storagePromptInteractive = func(io.Reader) bool { return false }
	t.Cleanup(func() { storagePromptInteractive = previous })
}

func interactiveStorage(t *testing.T) {
	t.Helper()
	previous := storagePromptInteractive
	storagePromptInteractive = func(io.Reader) bool { return true }
	t.Cleanup(func() { storagePromptInteractive = previous })
}

type storageFixture struct {
	root     string
	settings config.LocalSettings
}

// newStorageFixture creates every current local store through its real owner.
func newStorageFixture(t *testing.T) storageFixture {
	t.Helper()
	root := t.TempDir()
	settings := config.LocalSettings{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite"), AppDBPath: filepath.Join(root, "app.sqlite")}
	runtime, err := localruntime.OpenWithApp(t.Context(), settings.CollectorDBPath, settings.ServerDBPath, settings.AppDBPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	collector, _, err := collectordb.CreateIfMissing(settings.CollectorDBPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := collector.Close(); err != nil {
		t.Fatal(err)
	}
	jobs, err := syncjob.Open(t.Context(), settings.CollectorDBPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Close(); err != nil {
		t.Fatal(err)
	}
	return storageFixture{root: root, settings: settings}
}

func (f storageFixture) version(t *testing.T, path string, version int) {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.Exec("PRAGMA user_version=" + strconv.Itoa(version)); err != nil {
		t.Fatal(err)
	}
}

// files records durable file contents; lock sidecars and WAL indexes are excluded.
func (f storageFixture) files(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	entries, err := os.ReadDir(f.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) == ".lock" || strings.HasSuffix(name, "-shm") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(f.root, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(body) == 0 && strings.HasSuffix(name, "-wal") {
			continue
		}
		files[name] = string(body)
	}
	return files
}

func (f storageFixture) invocation(t *testing.T, answer string, stderr io.Writer) commandInvocation {
	return commandInvocation{context: t.Context(), stdin: strings.NewReader(answer), stdout: io.Discard, stderr: stderr, now: time.Now()}
}

func TestConfirmedResetMovesOlderCollectorAndCommandRecovers(t *testing.T) {
	interactiveStorage(t)
	f := newStorageFixture(t)
	f.version(t, f.settings.CollectorDBPath, collectordb.SupportedSchemaVersion-1)
	var stderr bytes.Buffer
	if err := f.invocation(t, "y\n", &stderr).prepareLocalStorage(f.settings); err != nil {
		t.Fatal(err)
	}
	prompt := stderr.String()
	for _, want := range []string{
		"TokenInsights needs to reset local storage written by an older version.",
		"  collector.sqlite   schema 19 -> 20   reset\n",
		"  server.sqlite      schema 1          keep\n",
		"  app.sqlite         schema 2          keep\n",
		"Old files move aside:\n  " + f.settings.CollectorDBPath + ".v19.bak\n",
		"Usage is rebuilt from saved usage and harness logs.",
		"Reset now? [y/N] ",
		"Moved " + f.settings.CollectorDBPath + " to " + f.settings.CollectorDBPath + ".v19.bak\n",
		"Local storage reset.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, unwanted := range []string{"credentials", "cannot be rebuilt"} {
		if strings.Contains(prompt, unwanted) {
			t.Errorf("collector-only prompt mentions %q", unwanted)
		}
	}
	if _, err := os.Stat(f.settings.CollectorDBPath + ".v19.bak"); err != nil {
		t.Fatal(err)
	}
	args := []string{"--config-file", filepath.Join(f.root, "config.json"), "data", "wait", "--collector-db-path", f.settings.CollectorDBPath, "--server-db-path", f.settings.ServerDBPath, "--app-db-path", f.settings.AppDBPath}
	if err := Run(t.Context(), args, io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatal("command did not recover", err)
	}
}

func TestResetPromptWarnsWhenCollectorAndServerResetTogether(t *testing.T) {
	interactiveStorage(t)
	f := newStorageFixture(t)
	f.version(t, f.settings.CollectorDBPath, collectordb.SupportedSchemaVersion-1)
	f.version(t, f.settings.ServerDBPath, 0)
	var stderr bytes.Buffer
	if err := f.invocation(t, "n\n", &stderr).prepareLocalStorage(f.settings); err == nil {
		t.Fatal("declined reset continued")
	}
	for _, want := range []string{"  app.sqlite         schema 2          reset\n", "Local accounts and credentials are recreated.", "Usage whose harness logs were already deleted cannot be rebuilt."} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("prompt missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestDeclinedOrUnansweredResetChangesNothing(t *testing.T) {
	interactiveStorage(t)
	for _, answer := range []string{"n\n", "\n", "", "yep\n"} {
		f := newStorageFixture(t)
		f.version(t, f.settings.CollectorDBPath, collectordb.SupportedSchemaVersion-1)
		before := f.files(t)
		err := f.invocation(t, answer, io.Discard).prepareLocalStorage(f.settings)
		if err == nil || err.Error() != "Reset declined. "+storageUnchanged {
			t.Fatalf("answer %q: %v", answer, err)
		}
		if after := f.files(t); !reflect.DeepEqual(before, after) {
			t.Fatalf("answer %q changed files", answer)
		}
	}
}

func TestBlockedStorageIsReportedWithoutPrompt(t *testing.T) {
	interactiveStorage(t)
	cases := []struct {
		name   string
		mutate func(storageFixture)
		want   []string
	}{
		{"newer", func(f storageFixture) { f.version(t, f.settings.CollectorDBPath, collectordb.SupportedSchemaVersion+1) },
			[]string{"collector.sqlite: schema 21; this build supports 20", "Upgrade TokenInsights, or move the file aside to start fresh."}},
		{"foreign", func(f storageFixture) {
			if err := os.Rename(f.settings.AppDBPath, f.settings.ServerDBPath+".tmp"); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(f.settings.ServerDBPath, f.settings.AppDBPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(f.settings.ServerDBPath+".tmp", f.settings.ServerDBPath); err != nil {
				t.Fatal(err)
			}
		}, []string{"server.sqlite: not a TokenInsights token database", "app.sqlite: not a TokenInsights application database", "Check the configured path."}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := newStorageFixture(t)
			f.version(t, f.settings.ServerDBPath, 0)
			test.mutate(f)
			before := f.files(t)
			var stderr bytes.Buffer
			err := f.invocation(t, "y\n", &stderr).prepareLocalStorage(f.settings)
			if err == nil || stderr.Len() != 0 {
				t.Fatalf("blocked storage prompted: %v %q", err, stderr.String())
			}
			for _, want := range append(test.want, "TokenInsights cannot open local storage. "+storageUnchanged) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("missing %q: %v", want, err)
				}
			}
			if after := f.files(t); !reflect.DeepEqual(before, after) {
				t.Fatal("blocked storage changed")
			}
		})
	}
}

func TestResetWhileAnotherProcessOwnsServerChangesNothing(t *testing.T) {
	interactiveStorage(t)
	f := newStorageFixture(t)
	f.version(t, f.settings.ServerDBPath, 0)
	owner, held, err := serverownership.Lifetime(f.settings.ServerDBPath, true)
	if err != nil || held {
		t.Fatal(held, err)
	}
	defer func() { _ = owner.Close() }()
	before := f.files(t)
	err = f.invocation(t, "y\n", io.Discard).prepareLocalStorage(f.settings)
	if err == nil || !strings.Contains(err.Error(), "another TokenInsights process is using local storage") {
		t.Fatal(err)
	}
	if after := f.files(t); !reflect.DeepEqual(before, after) {
		t.Fatal("busy reset changed files")
	}
}

func TestSyncOffersResetForCollectorAndJobsOnly(t *testing.T) {
	nonInteractiveStorage(t)
	f := newStorageFixture(t)
	jobs := f.settings.CollectorDBPath + ".jobs.sqlite"
	f.version(t, jobs, syncjob.SchemaVersion-1)
	f.version(t, f.settings.ServerDBPath, 0)
	before := f.files(t)
	args := []string{"--config-file", filepath.Join(f.root, "config.json"), "sync", "status", "--collector-db-path", f.settings.CollectorDBPath}
	err := Run(t.Context(), args, io.Discard, io.Discard, time.Now())
	if err == nil || !strings.Contains(err.Error(), "\n  "+jobs) || strings.Contains(err.Error(), "server.sqlite") {
		t.Fatalf("sync storage report: %v", err)
	}
	if after := f.files(t); !reflect.DeepEqual(before, after) {
		t.Fatal("sync without terminal changed files")
	}
}

func TestSyncDryRunNeverOffersReset(t *testing.T) {
	interactiveStorage(t)
	f := newStorageFixture(t)
	f.version(t, f.settings.CollectorDBPath, collectordb.SupportedSchemaVersion-1)
	before := f.files(t)
	args := []string{"--config-file", filepath.Join(f.root, "config.json"), "sync", "--dry-run", "--collector-db-path", f.settings.CollectorDBPath, "--source-dir", filepath.Join(f.root, "source")}
	err := Run(t.Context(), args, io.Discard, io.Discard, time.Now())
	if err != nil && strings.Contains(err.Error(), "older version") {
		t.Fatal("dry run offered reset", err)
	}
	if after := f.files(t); !reflect.DeepEqual(before, after) {
		t.Fatal("dry run changed files")
	}
	if errors.Is(err, ErrUsage) {
		t.Fatal(err)
	}
}
