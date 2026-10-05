package collector_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestion"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
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

func acceptanceSetup(t *testing.T) (collector.Options, *serverstore.Store, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "server.sqlite")
	s, err := serverstore.CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	options := collector.Options{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: path, SyncOptions: pipeline.SyncOptions{SourceDir: acceptanceSources(t), Harnesses: pipeline.SupportedHarnesses, Normalize: true, Now: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)}}
	return options, s, path
}

func acceptanceRun(t *testing.T, o collector.Options) collector.Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r, err := collector.Run(ctx, o)
	if err != nil {
		t.Fatalf("real collector sync: %+v %v", r, err)
	}
	return r
}

type acceptanceFact struct {
	Harness        string `json:"harness"`
	Session        string `json:"session_id"`
	Message        string `json:"message_id"`
	Provider       string `json:"provider"`
	ProviderSource string `json:"provider_source"`
	Model          string `json:"model"`
	Scope          string `json:"usage_scope"`
	Quality        string `json:"quality"`
	Countable      int64  `json:"is_countable"`
	Input          int64  `json:"input_tokens"`
	Output         int64  `json:"output_tokens"`
	Reasoning      int64  `json:"reasoning_tokens"`
	CacheRead      int64  `json:"cache_read_tokens"`
	CacheWrite     int64  `json:"cache_write_tokens"`
	Total          int64  `json:"total_tokens"`
}

func acceptanceAssert(t *testing.T, s *serverstore.Store) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "conformance", "collector-rebuild", "expected", "canonical_facts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want []acceptanceFact
	if err := json.Unmarshal(body, &want); err != nil {
		t.Fatal(err)
	}
	rows, err := s.SQL().Query(`SELECT u.harness,s.session_id,COALESCE(m.harness_message_id,''),u.provider,u.provider_source,u.model,u.usage_scope,u.quality,u.is_countable,u.input_tokens,u.output_tokens,u.reasoning_tokens,u.cache_read_tokens,u.cache_write_tokens,u.total_tokens,u.semantic_key FROM canonical_token_usage u JOIN canonical_sessions s ON s.id=u.session_id LEFT JOIN canonical_messages m ON m.id=u.message_id ORDER BY u.harness,s.session_id,m.harness_message_id,u.semantic_key`)
	if err != nil {
		t.Fatal(err)
	}
	got := []acceptanceFact{}
	identities := []string{}
	for rows.Next() {
		var f acceptanceFact
		var id string
		if err := rows.Scan(&f.Harness, &f.Session, &f.Message, &f.Provider, &f.ProviderSource, &f.Model, &f.Scope, &f.Quality, &f.Countable, &f.Input, &f.Output, &f.Reasoning, &f.CacheRead, &f.CacheWrite, &f.Total, &id); err != nil {
			t.Fatal(err)
		}
		got = append(got, f)
		identities = append(identities, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 12 {
		t.Fatalf("native fixture facts=%d want=12: %+v", len(got), got)
	}
	for i, f := range got {
		if f.Harness == "codex" {
			if !strings.HasPrefix(f.Message, want[i].Message) || len(strings.TrimPrefix(f.Message, want[i].Message)) != 64 {
				t.Fatalf("Codex native identity differs: %s", f.Message)
			}
			got[i].Message = want[i].Message
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("independent native golden facts differ: got=%+v want=%+v", got, want)
	}
	var counts [7]int64
	if err := s.SQL().QueryRow(`SELECT COUNT(*),SUM(input_tokens),SUM(output_tokens),SUM(reasoning_tokens),SUM(cache_read_tokens),SUM(cache_write_tokens),SUM(total_tokens) FROM canonical_token_usage WHERE is_countable=1`).Scan(&counts[0], &counts[1], &counts[2], &counts[3], &counts[4], &counts[5], &counts[6]); err != nil {
		t.Fatal(err)
	}
	if counts != [7]int64{12, 800, 148, 52, 96, 6, 1102} {
		t.Fatalf("independent components/total=%v want=[12 800 148 52 96 6 1102]", counts)
	}
	return identities
}

func acceptanceOpenCollector(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := db.OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// F01/G01/G02/G05: every harness is captured, normalized and delivered through HTTP.
func TestCollectorContractRebuildAndRepeatPreserveNativeFacts(t *testing.T) {
	o, s, path := acceptanceSetup(t)
	server := httptest.NewServer(ingestion.NewHandler(ingestion.NewCore(s)))
	defer server.Close()
	o.ServerURL = server.URL
	first := acceptanceRun(t, o)
	if first.Inserted != 12 || first.Pending != 0 {
		t.Fatalf("initial publication=%+v", first)
	}
	want := acceptanceAssert(t, s)
	database := acceptanceOpenCollector(t, o.CollectorDBPath)
	var oldStream string
	if err := database.QueryRow("SELECT stream_id FROM publication_state WHERE id=1").Scan(&oldStream); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(o.CollectorDBPath); err != nil {
		t.Fatal(err)
	}
	o.SyncOptions.Now = o.SyncOptions.Now.Add(24 * time.Hour)
	rebuilt := acceptanceRun(t, o)
	if rebuilt.Inserted != 0 || rebuilt.Noop != 12 || rebuilt.Pending != 0 {
		t.Fatalf("rebuilt publication=%+v", rebuilt)
	}
	database = acceptanceOpenCollector(t, o.CollectorDBPath)
	var newStream string
	if err := database.QueryRow("SELECT stream_id FROM publication_state WHERE id=1").Scan(&newStream); err != nil {
		t.Fatal(err)
	}
	if oldStream == newStream {
		t.Fatal("collector reinstall reused delivery stream")
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	o.SyncOptions.FullRefresh = true
	for i := 0; i < 100; i++ {
		o.SyncOptions.Now = o.SyncOptions.Now.Add(time.Hour)
		r := acceptanceRun(t, o)
		if r.Inserted != 0 || r.Updated != 0 || r.Pending != 0 {
			t.Fatalf("repeat %d publication=%+v", i, r)
		}
	}
	if got := acceptanceAssert(t, s); !reflect.DeepEqual(got, want) {
		t.Fatal("native identities changed across collector rebuild")
	}
	server.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := serverstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if got := acceptanceAssert(t, reopened); !reflect.DeepEqual(got, want) {
		t.Fatal("native identities changed across server reopen")
	}
}

// F02/G06/G07/G10: commit succeeds, response is lost, exact batch survives reopening.
func TestCollectorContractLostAcknowledgementResumesSavedBatch(t *testing.T) {
	o, s, path := acceptanceSetup(t)
	var mu sync.Mutex
	core := ingestion.NewCore(s)
	lose := true
	var requests [][]byte
	var firstReceipt publication.Receipt
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodPost {
			ingestion.NewHandler(core).ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		requests = append(requests, bytes.Clone(body))
		receipt, err := core.Ingest(r.Context(), body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if lose {
			lose = false
			firstReceipt = receipt
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(receipt); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	o.ServerURL = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := collector.Run(ctx, o)
	if err == nil || result.CollectionError != nil || result.DeliveryError == nil || result.Pending != 12 {
		t.Fatalf("lost response result=%+v err=%v", result, err)
	}
	acceptanceAssert(t, s)
	database := acceptanceOpenCollector(t, o.CollectorDBPath)
	var cursor int64
	if err := database.QueryRow("SELECT acknowledged_sequence FROM publication_destinations").Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if cursor != 0 {
		t.Fatal("lost response advanced collector cursor")
	}
	var saved []byte
	if err := database.QueryRow("SELECT request_bytes FROM publication_batches WHERE receipt_bytes IS NULL").Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := serverstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	core = ingestion.NewCore(reopened)
	mu.Unlock()
	defer func() { _ = reopened.Close() }()
	o.PublishOnly = true
	result = acceptanceRun(t, o)
	if result.Noop != 0 || result.Inserted != 12 || result.Pending != 0 {
		t.Fatalf("durable receipt replay=%+v", result)
	}
	mu.Lock()
	if len(requests) != 2 || !bytes.Equal(requests[0], requests[1]) || !bytes.Equal(saved, requests[1]) {
		t.Fatal("pending retry body changed")
	}
	mu.Unlock()
	var receiptJSON string
	if err := reopened.SQL().QueryRow("SELECT receipt_json FROM ingestion_receipts").Scan(&receiptJSON); err != nil {
		t.Fatal(err)
	}
	var durable publication.Receipt
	if err := json.Unmarshal([]byte(receiptJSON), &durable); err != nil {
		t.Fatal(err)
	}
	if durable != firstReceipt {
		t.Fatal("receipt identity changed across reopen")
	}
	acceptanceAssert(t, reopened)
}

// G04/G07/G08: destination progress is independent and wrong receipts stay pending.
func TestCollectorContractDestinationIsolationAndReceiptBinding(t *testing.T) {
	o, s, _ := acceptanceSetup(t)
	left := httptest.NewServer(ingestion.NewHandler(ingestion.NewCore(s)))
	defer left.Close()
	o.ServerURL = left.URL
	acceptanceRun(t, o)
	other, err := serverstore.CreateIfMissing(filepath.Join(t.TempDir(), "other.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	right := httptest.NewServer(ingestion.NewHandler(ingestion.NewCore(other)))
	defer right.Close()
	o.ServerURL = right.URL
	o.PublishOnly = true
	r := acceptanceRun(t, o)
	if r.Inserted != 12 {
		t.Fatalf("second destination inherited first cursor: %+v", r)
	}
	acceptanceAssert(t, s)
	acceptanceAssert(t, other)
	database := acceptanceOpenCollector(t, o.CollectorDBPath)
	store := collectorstore.Store{DB: database}
	m, err := other.Metadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BindDestination(context.Background(), "fault-destination", right.URL, m.DatabaseID); err != nil {
		t.Fatal(err)
	}
	saved, err := store.PrepareBatch(context.Background(), "fault-destination", "synthetic-host", 1767225600000)
	if err != nil || saved == nil {
		t.Fatalf("prepare: %v", err)
	}
	receipt, err := ingestion.NewCore(other).Ingest(context.Background(), saved.Request)
	if err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*publication.Receipt){func(r *publication.Receipt) { r.DatabaseID = "wrong" }, func(r *publication.Receipt) { r.BatchID = "wrong" }, func(r *publication.Receipt) { r.StreamID = "wrong" }, func(r *publication.Receipt) { r.RequestHash = "wrong" }, func(r *publication.Receipt) { r.ToSequence++ }} {
		wrong := receipt
		alter(&wrong)
		body, err := json.Marshal(wrong)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Ack(context.Background(), "fault-destination", body, 1767225600001); err == nil {
			t.Fatal("foreign receipt advanced progress")
		}
		var cursor int64
		if err := database.QueryRow("SELECT acknowledged_sequence FROM publication_destinations WHERE destination_id='fault-destination'").Scan(&cursor); err != nil {
			t.Fatal(err)
		}
		if cursor != 0 {
			t.Fatal("wrong receipt advanced cursor")
		}
	}
	body, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Ack(context.Background(), "fault-destination", body, 1767225600001); err != nil {
		t.Fatal(err)
	}
	if pending, err := store.Pending(context.Background(), "fault-destination"); err != nil || pending != 0 {
		t.Fatalf("valid receipt pending=%d err=%v", pending, err)
	}
	for _, marker := range []string{o.SyncOptions.SourceDir, "promptText", "toolOutput", "source_path", "request_headers"} {
		if bytes.Contains(saved.Request, []byte(marker)) {
			t.Fatalf("publication contains local/private field %q", marker)
		}
	}
}

// F13/G10: source disappearance never deletes already ingested canonical history.
func TestCollectorContractMissingSourcesRetainServerFacts(t *testing.T) {
	o, s, _ := acceptanceSetup(t)
	server := httptest.NewServer(ingestion.NewHandler(ingestion.NewCore(s)))
	defer server.Close()
	o.ServerURL = server.URL
	acceptanceRun(t, o)
	want := acceptanceAssert(t, s)
	if err := os.RemoveAll(o.SyncOptions.SourceDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(o.SyncOptions.SourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(o.CollectorDBPath); err != nil {
		t.Fatal(err)
	}
	r := acceptanceRun(t, o)
	if r.Inserted != 0 || r.Updated != 0 || r.Batches != 0 {
		t.Fatalf("empty sources published changes: %+v", r)
	}
	if got := acceptanceAssert(t, s); !reflect.DeepEqual(got, want) {
		t.Fatal("missing sources removed server history")
	}
}

// F12/G08: a real adapter sees harmless transcript/tool/header sentinels, but
// neither metadata storage nor the actual HTTP publication retains them.
func TestCollectorContractRawContentNeverEntersPublication(t *testing.T) {
	o, s, serverPath := acceptanceSetup(t)
	artifact := filepath.Join(o.SyncOptions.SourceDir, "pi", "project", "main.jsonl")
	source, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	source = bytes.ReplaceAll(source, []byte(`"role":"assistant",`), []byte(`"role":"assistant","content":[{"type":"text","text":"SYNTHETIC_PRIVATE_MARKER"}],"toolOutput":"SYNTHETIC_TOOL_MARKER","request_headers":{"authorization":"SYNTHETIC_SECRET_MARKER"},`))
	if err := os.WriteFile(artifact, source, 0o600); err != nil {
		t.Fatal(err)
	}
	markers := [][]byte{[]byte("SYNTHETIC_PRIVATE_MARKER"), []byte("SYNTHETIC_TOOL_MARKER"), []byte("SYNTHETIC_SECRET_MARKER")}
	handler := ingestion.NewHandler(ingestion.NewCore(s))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			for _, marker := range markers {
				if bytes.Contains(body, marker) {
					t.Error("source content leaked into actual publication")
				}
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	o.ServerURL = server.URL
	acceptanceRun(t, o)
	acceptanceAssert(t, s)
	for _, path := range []string{o.CollectorDBPath, o.CollectorDBPath + "-wal", serverPath, serverPath + "-wal"} {
		body, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if bytes.Contains(body, marker) {
				t.Fatalf("source content retained in metadata database %s", filepath.Base(path))
			}
		}
	}
}
