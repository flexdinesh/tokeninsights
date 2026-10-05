package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func TestCollectorNamespaceHelpHasNoStorageSideEffects(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "collector.sqlite")
	t.Setenv("TOKENINSIGHTS_COLLECTOR_DB_PATH", path)
	for _, args := range [][]string{{"collector"}, {"collector", "--help"}, {"collector", "help"}, {"collector", "normalize", "--help"}, {"collector", "reset-all", "--help"}, {"collector", "reset-canonical", "--help"}} {
		var output bytes.Buffer
		if err := Run(context.Background(), args, &output, &output, time.Now()); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(output.String(), "tokeninsights collector") {
			t.Fatalf("namespace missing: %q", output.String())
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("help touched storage: %v %v", entries, err)
	}
}

func TestCollectorNamespaceRejectsUnrelatedCommands(t *testing.T) {
	for _, name := range []string{"sync", "service", "tui", "unknown"} {
		err := Run(context.Background(), []string{"collector", name}, io.Discard, io.Discard, time.Now())
		if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "unknown collector command") {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestGroupedResetCommandsOperateOnCollector(t *testing.T) {
	for _, name := range []string{"reset-canonical", "reset-all"} {
		root := t.TempDir()
		path := filepath.Join(root, "collector.sqlite")
		database, _, err := db.CreateIfMissing(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec("INSERT INTO canonical_sessions(semantic_key,harness,session_id,first_seen_at_ms,last_seen_at_ms) VALUES ('fixture-session','pi','fixture-native-session',10,10)"); err != nil {
			_ = database.Close()
			t.Fatal(err)
		}
		_ = database.Close()
		args := []string{"collector", name, "--confirm", "--collector-db-path", path}
		if err := Run(context.Background(), args, io.Discard, io.Discard, time.Now()); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		database, err = db.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		assertCLIQueryCount(t, database, "SELECT COUNT(*) FROM canonical_sessions", 0)
		_ = database.Close()
	}
}

func TestPrimaryHelpNamesEverydayCLIAndAdvancedNamespace(t *testing.T) {
	help := usageText()
	for _, name := range []string{"service start|stop|restart|status", "sync", "tui", "collector normalize|reset-canonical|reset-all"} {
		if !strings.Contains(help, name) {
			t.Fatalf("primary help missing %q", name)
		}
	}
	for _, name := range []string{"tui"} {
		command, ok := commandByName(name)
		if !ok || command.name != "tui" {
			t.Fatalf("%s did not resolve to tui: %+v", name, command)
		}
	}
}

func TestPrimaryHelpDoesNotAdvertiseRemovedCompatibility(t *testing.T) {
	if strings.Contains(usageText(), "Deprecated aliases") {
		t.Fatal("help advertises compatibility aliases")
	}
}
