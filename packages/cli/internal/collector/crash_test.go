package collector_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestion"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"

	"modernc.org/sqlite"
)

type crashConfiguration struct {
	Seam, CollectorPath, ServerPath, SourceDir, ServerURL, SavedBody string
}

// F14/G03-G07/G10: named barriers prove which real transaction is interrupted.
// The scalar function exists only inside this test subprocess. No production
// environment fault hooks, graceful shutdown, or WAL removal participate.
func TestCollectorContractCrashBoundaries(t *testing.T) {
	for _, seam := range []string{"before_capture_commit", "during_capture_transaction", "after_capture_commit", "during_canonical_journal_transaction", "after_canonical_journal_commit", "during_batch_preparation_transaction", "after_batch_saved_before_send", "during_server_transaction", "after_server_commit_before_response", "after_response_before_collector_ack_commit", "during_collector_ack_transaction", "after_collector_ack_commit"} {
		t.Run(seam, func(t *testing.T) {
			o, s, serverPath := acceptanceSetup(t)
			server := httptest.NewServer(ingestion.NewHandler(ingestion.NewCore(s)))
			defer server.Close()
			o.ServerURL = server.URL
			config := crashConfiguration{Seam: seam, CollectorPath: o.CollectorDBPath, ServerPath: serverPath, SourceDir: o.SyncOptions.SourceDir, ServerURL: server.URL}
			if seam == "during_server_transaction" || seam == "after_server_commit_before_response" {
				if _, err := pipeline.Sync(context.Background(), pipeline.SyncOptions{DBPath: o.CollectorDBPath, SourceDir: o.SyncOptions.SourceDir, Harnesses: pipeline.SupportedHarnesses, Normalize: true, Now: o.SyncOptions.Now}); err != nil {
					t.Fatal(err)
				}
				database := acceptanceOpenCollector(t, o.CollectorDBPath)
				store := collectorstore.Store{DB: database}
				metadata, err := s.Metadata(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256([]byte(server.URL))
				destination := hex.EncodeToString(sum[:])
				if err := store.BindDestination(context.Background(), destination, server.URL, metadata.DatabaseID); err != nil {
					t.Fatal(err)
				}
				saved, err := store.PrepareBatch(context.Background(), destination, "synthetic-host", 1767225600000)
				if err != nil || saved == nil {
					t.Fatalf("saved batch: %v", err)
				}
				config.SavedBody = filepath.Join(t.TempDir(), "saved.json")
				if err := os.WriteFile(config.SavedBody, saved.Request, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := database.Close(); err != nil {
					t.Fatal(err)
				}
			}
			killAtBarrier(t, config)
			database := acceptanceOpenCollector(t, o.CollectorDBPath)
			var raw, canonical, journal, batches, cursor int64
			for _, query := range []struct {
				sql    string
				target *int64
			}{{"SELECT COUNT(*) FROM raw_token_usage", &raw}, {"SELECT COUNT(*) FROM canonical_token_usage", &canonical}, {"SELECT COUNT(*) FROM publication_journal", &journal}, {"SELECT COUNT(*) FROM publication_batches", &batches}, {"SELECT COALESCE(MAX(acknowledged_sequence),0) FROM publication_destinations", &cursor}} {
				if err := database.QueryRow(query.sql).Scan(query.target); err != nil {
					t.Fatal(err)
				}
			}
			switch seam {
			case "before_capture_commit", "during_capture_transaction":
				if raw != 0 || canonical != 0 || journal != 0 || cursor != 0 {
					t.Fatalf("capture escaped crash rollback raw=%d canonical=%d journal=%d cursor=%d", raw, canonical, journal, cursor)
				}
				var markers int
				if err := database.QueryRow("SELECT COUNT(*) FROM source_cursor_state").Scan(&markers); err != nil {
					t.Fatal(err)
				}
				if markers != 0 {
					t.Fatal("capture cursor escaped rollback")
				}
			case "after_capture_commit", "during_canonical_journal_transaction":
				if raw == 0 || canonical != 0 || journal != 0 || cursor != 0 {
					t.Fatalf("capture/normalization atomicity raw=%d canonical=%d journal=%d cursor=%d", raw, canonical, journal, cursor)
				}
				var pending int
				if err := database.QueryRow("SELECT COUNT(*) FROM normalization_work_queue").Scan(&pending); err != nil {
					t.Fatal(err)
				}
				if pending == 0 {
					t.Fatal("normalization work lost on crash")
				}
				var markers int
				if err := database.QueryRow("SELECT COUNT(*) FROM source_refresh_state").Scan(&markers); err != nil {
					t.Fatal(err)
				}
				if markers == 0 {
					t.Fatal("committed capture lost source continuity metadata")
				}
			case "after_canonical_journal_commit", "during_batch_preparation_transaction":
				if canonical != 12 || journal != 12 || batches != 0 || cursor != 0 {
					t.Fatalf("journal/batch atomicity canonical=%d journal=%d batches=%d cursor=%d", canonical, journal, batches, cursor)
				}
			case "after_collector_ack_commit":
				if cursor != 12 {
					t.Fatalf("durable ack cursor=%d want12", cursor)
				}
			default:
				if canonical != 12 || journal != 12 || batches != 1 || cursor != 0 {
					t.Fatalf("pending delivery atomicity canonical=%d journal=%d batches=%d cursor=%d", canonical, journal, batches, cursor)
				}
			}
			if _, err := database.Exec("DROP TRIGGER IF EXISTS test_crash_boundary"); err != nil {
				t.Fatal(err)
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := s.SQL().Exec("DROP TRIGGER IF EXISTS test_crash_boundary"); err != nil {
				t.Fatal(err)
			}
			var serverFacts int64
			if err := s.SQL().QueryRow("SELECT COUNT(*) FROM canonical_token_usage").Scan(&serverFacts); err != nil {
				t.Fatal(err)
			}
			committed := seam == "after_server_commit_before_response" || seam == "after_response_before_collector_ack_commit" || seam == "during_collector_ack_transaction" || seam == "after_collector_ack_commit"
			var receipts int
			if err := s.SQL().QueryRow("SELECT COUNT(*) FROM ingestion_receipts").Scan(&receipts); err != nil {
				t.Fatal(err)
			}
			if committed {
				if serverFacts != 12 || receipts != 1 {
					t.Fatalf("ack precedes commit: server facts=%d receipts=%d", serverFacts, receipts)
				}
				acceptanceAssert(t, s)
			} else if serverFacts != 0 || receipts != 0 {
				t.Fatalf("server partial data escaped crash: facts=%d receipts=%d", serverFacts, receipts)
			}
			// Closing and reopening replaces connections, retaining the crash WAL.
			server.Close()
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := serverstore.Open(serverPath)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = reopened.Close() }()
			resumedServer := httptest.NewUnstartedServer(ingestion.NewHandler(ingestion.NewCore(reopened)))
			// Keep the original listener address: destination binding is unchanged.
			listener, err := net.Listen("tcp", server.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			_ = resumedServer.Listener.Close()
			resumedServer.Listener = listener
			resumedServer.Start()
			defer resumedServer.Close()
			o.ServerURL = resumedServer.URL
			result := acceptanceRun(t, o)
			if result.Pending != 0 {
				t.Fatalf("manual resume left pending=%d", result.Pending)
			}
			acceptanceAssert(t, reopened)
		})
	}
}

func killAtBarrier(t *testing.T, config crashConfiguration) {
	t.Helper()
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = read.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCollectorCrashHelper$", "-test.timeout=18s")
	command.Env = append(os.Environ(), "TOKENINSIGHTS_TEST_CRASH_CONFIG="+string(body))
	command.ExtraFiles = []*os.File{write}
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	_ = write.Close()
	ready := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(read).ReadString('\n')
		if err == nil && line != "ready\n" {
			err = fmt.Errorf("unexpected crash barrier %q", line)
		}
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatalf("barrier not reached: %v %s", err, output.String())
		}
	case <-ctx.Done():
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("barrier deadline: %s", output.String())
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("child exited gracefully instead of being killed")
	}
}

func TestCollectorCrashHelper(t *testing.T) {
	body := os.Getenv("TOKENINSIGHTS_TEST_CRASH_CONFIG")
	if body == "" {
		return
	}
	var config crashConfiguration
	if err := json.Unmarshal([]byte(body), &config); err != nil {
		t.Fatal(err)
	}
	pipe := os.NewFile(3, "crash-barrier")
	if pipe == nil {
		t.Fatal("missing barrier pipe")
	}
	barrier := func() {
		if _, err := pipe.WriteString("ready\n"); err != nil {
			t.Fatal(err)
		}
		<-make(chan struct{})
	}
	if err := sqlite.RegisterScalarFunction("test_crash_barrier", 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) { barrier(); return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if config.Seam == "during_server_transaction" || config.Seam == "after_server_commit_before_response" {
		store, err := serverstore.Open(config.ServerPath)
		if err != nil {
			t.Fatal(err)
		}
		if config.Seam == "during_server_transaction" {
			if _, err := store.SQL().Exec(`CREATE TRIGGER test_crash_boundary AFTER INSERT ON ingestion_receipts BEGIN SELECT test_crash_barrier(); END`); err != nil {
				t.Fatal(err)
			}
		}
		request, err := os.ReadFile(config.SavedBody)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ingestion.NewCore(store).Ingest(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		barrier()
		return
	}
	database, _, err := db.CreateIfMissing(config.CollectorPath)
	if err != nil {
		t.Fatal(err)
	}
	var trigger string
	switch config.Seam {
	case "during_capture_transaction":
		trigger = `AFTER INSERT ON raw_token_usage`
	case "during_canonical_journal_transaction":
		trigger = `BEFORE INSERT ON publication_journal`
	case "during_batch_preparation_transaction":
		trigger = `AFTER INSERT ON publication_batches`
	case "after_response_before_collector_ack_commit":
		trigger = `BEFORE UPDATE ON publication_batches WHEN NEW.receipt_bytes IS NOT NULL`
	case "during_collector_ack_transaction":
		trigger = `AFTER UPDATE ON publication_batches WHEN NEW.receipt_bytes IS NOT NULL`
	}
	if trigger != "" {
		if _, err := database.Exec("CREATE TRIGGER test_crash_boundary " + trigger + " BEGIN SELECT test_crash_barrier(); END"); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	options := collector.Options{CollectorDBPath: config.CollectorPath, ServerDBPath: config.ServerPath, ServerURL: config.ServerURL, SyncOptions: pipeline.SyncOptions{SourceDir: config.SourceDir, Harnesses: pipeline.SupportedHarnesses, Normalize: true, Now: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)}}
	options.SyncOptions.Progress = func(event pipeline.SyncProgressEvent) {
		if config.Seam == "before_capture_commit" && event.Status == pipeline.SyncProgressSyncing || config.Seam == "after_capture_commit" && event.Status == pipeline.SyncProgressNormalizing {
			barrier()
		}
	}
	if config.Seam == "after_canonical_journal_commit" {
		options.ServerURL = ""
		options.EnsureLocal = func(context.Context) (string, error) { barrier(); return config.ServerURL, nil }
	}
	if config.Seam == "after_batch_saved_before_send" {
		if _, err := pipeline.Sync(context.Background(), pipeline.SyncOptions{DBPath: config.CollectorPath, SourceDir: config.SourceDir, Harnesses: pipeline.SupportedHarnesses, Normalize: true, Now: options.SyncOptions.Now}); err != nil {
			t.Fatal(err)
		}
		store, err := serverstore.Open(config.ServerPath)
		if err != nil {
			t.Fatal(err)
		}
		metadata, err := store.Metadata(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_ = store.Close()
		database, err := db.OpenWritable(config.CollectorPath)
		if err != nil {
			t.Fatal(err)
		}
		delivery := collectorstore.Store{DB: database}
		sum := sha256.Sum256([]byte(config.ServerURL))
		destination := hex.EncodeToString(sum[:])
		if err := delivery.BindDestination(context.Background(), destination, config.ServerURL, metadata.DatabaseID); err != nil {
			t.Fatal(err)
		}
		if saved, err := delivery.PrepareBatch(context.Background(), destination, "synthetic-host", 1767225600000); err != nil || saved == nil {
			t.Fatalf("batch save: %v", err)
		}
		barrier()
		return
	}
	if _, err := collector.Run(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	if config.Seam == "after_collector_ack_commit" {
		barrier()
	}
	t.Fatal("requested fault seam was not reached")
}
