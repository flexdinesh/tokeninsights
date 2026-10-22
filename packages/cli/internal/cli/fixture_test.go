package cli

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func isolateViewSources(t *testing.T, root string) {
	t.Helper()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "xdg"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude"))
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "")
	t.Setenv("TOKENINSIGHTS_SERVER_TOKEN", "")
}

func assertViewMissingPath(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only remote view touched %s: %v", path, err)
	}
}

// seedLocalUsage commits one processed usage fact through a real local runtime
// and closes it, returning the collector and server database paths.
func seedLocalUsage(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	collectorPath, serverPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	runtime, err := localruntime.Open(t.Context(), collectorPath, serverPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	metadata, err := runtime.Store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 2,
		Data:    json.RawMessage(`{"type":"message","id":"request","message":{"role":"assistant","timestamp":1767225600000,"usage":{"input":80,"output":20,"totalTokens":100}}}`),
		Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
	body, err := json.Marshal(evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: 1, Entries: []evidence.Entry{{Sequence: 1, Record: record}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Store.Accept(t.Context(), evidence.ProtocolVersion, body); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := runtime.WaitVisible(ctx); err != nil {
		t.Fatal(err)
	}
	return collectorPath, serverPath
}
