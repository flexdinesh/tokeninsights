package collector_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqliteaccounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/storagecontract"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

// Timings on the backend are accumulated across concurrent workers, not additive
// phases of elapsed time. Counts include stale work and repeated scope loading.
type measuredBackend struct {
	*datastore.Store
	loadNS, publishNS, records, published, stale atomic.Int64
}

func (m *measuredBackend) LoadWorkExcluding(ctx context.Context, claims []dataengine.Work, limit int64) (dataengine.Work, bool, error) {
	start := time.Now()
	work, found, err := m.Store.LoadWorkExcluding(ctx, claims, limit)
	m.loadNS.Add(time.Since(start).Nanoseconds())
	if found {
		m.records.Add(int64(len(work.Records)))
	}
	return work, found, err
}

func (m *measuredBackend) PublishProjection(ctx context.Context, work dataengine.Work, projection evidence.Projection) (bool, error) {
	start := time.Now()
	published, err := m.Store.PublishProjection(ctx, work, projection)
	m.publishNS.Add(time.Since(start).Nanoseconds())
	if err == nil {
		if published {
			m.published.Add(1)
		} else {
			m.stale.Add(1)
		}
	}
	return published, err
}

type measuredDelivery struct {
	collector.Delivery
	worker *dataengine.Worker
	submit time.Duration
}

func (m *measuredDelivery) Submit(ctx context.Context, protocol int, body []byte) ([]byte, error) {
	start := time.Now()
	response, err := m.Delivery.Submit(ctx, protocol, body)
	m.submit += time.Since(start)
	// The store's own worker is inactive: this benchmark wraps its backend to
	// measure real concurrent processing without adding production observers.
	if err == nil {
		m.worker.Wake()
	}
	return response, err
}

func ingestionDestination(b *testing.B, store *datastore.Store, root, transport string) (*datastore.Store, collector.Delivery, func()) {
	b.Helper()
	if transport == "Direct" {
		return store, collector.DirectDelivery{Receiver: store}, func() {}
	}
	identity, err := store.DatabaseIdentity(b.Context())
	if err != nil {
		b.Fatal(err)
	}
	app, err := appstore.Open(b.Context(), filepath.Join(root, "app.sqlite"), identity, datastore.KindHosted)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = app.Close() })
	service := accounts.New(sqliteaccounts.NewSQLite(app, store))
	user, err := service.CreateUser(b.Context(), "benchmark")
	if err != nil {
		b.Fatal(err)
	}
	token, err := service.CreateToken(b.Context(), user.UserID, []string{accounts.Read, accounts.Ingest}, nil)
	if err != nil {
		b.Fatal(err)
	}
	policy, err := serverfeatures.New(serverfeatures.Hosted, false)
	if err != nil {
		b.Fatal(err)
	}
	remote := httptest.NewServer(server.NewDataHandlerWithOptions(b.Context(), sqlanalytics.Source{Store: store}, nil, server.DataHandlerOptions{
		Host: "127.0.0.1", InstanceID: "benchmark", AllowIngestion: true,
		Policy: policy, Accounts: service, PublicURL: "https://benchmark.example",
	}))
	b.Cleanup(remote.Close)
	return store.ForDataset(user.DatasetID), collector.HTTPDelivery{URL: remote.URL, Token: token.Secret, Client: remote.Client()}, func() {
		remote.Close()
		_ = app.Close()
	}
}

func ingestionSources(b *testing.B, sessions, messages int) string {
	b.Helper()
	root := b.TempDir()
	for session := range sessions {
		var body strings.Builder
		fmt.Fprintf(&body, "{\"type\":\"session\",\"id\":\"session-%d\"}\n", session)
		for message := range messages {
			body.WriteString(startupBenchmarkMessage(session, message))
		}
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("session-%d.jsonl", session)), []byte(body.String()), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	return root
}

func ingestionVisible(ctx context.Context, store *datastore.Store) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := sqlanalytics.Status(ctx, store)
		if err != nil {
			return err
		}
		if status.Failed > 0 {
			return fmt.Errorf("processing failed: %d scopes", status.Failed)
		}
		if status.Pending == 0 && status.Metadata.Generation == status.Metadata.TargetGeneration {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Real storage, capture, adapters and the production two-worker dispatcher.
// HTTP includes hosted authentication/admission on loopback, without TLS or
// network latency. Storage opening and provisioning are outside the timed region;
// BenchmarkLocalStartup measures command-owned opening separately.
func BenchmarkIngestion(b *testing.B) {
	for _, shape := range []struct {
		name               string
		sessions, messages int
	}{
		{"Sessions50", 50, 200},
		{"Session1", 1, 10000},
		{"Sessions500", 500, 20},
		{"Native", 0, 0},
		{"Unchanged", 50, 200},
		{"Append", 50, 200},
	} {
		name := shape.name
		if storagecontract.BenchmarkSmoke() && shape.sessions > 0 {
			shape.sessions = min(shape.sessions, 5)
			shape.messages = min(shape.messages, 20)
			name += "-Smoke"
		}
		for _, transport := range []string{"Direct", "HTTP"} {
			b.Run(name+"/"+transport, func(b *testing.B) {
				b.ReportAllocs()
				b.StopTimer()
				metrics := make(map[string]int64)
				for range b.N {
					func() {
						root := b.TempDir()
						sources := ""
						harnesses := []pipeline.Harness{pipeline.HarnessPi}
						if shape.name == "Native" {
							sources = acceptanceSources(b)
							harnesses = pipeline.SupportedHarnesses
						} else {
							sources = ingestionSources(b, shape.sessions, shape.messages)
						}
						kind := datastore.KindPersonal
						if transport == "HTTP" {
							kind = datastore.KindHosted
						}
						dataPath := filepath.Join(root, "server.sqlite")
						store, err := datastore.OpenKind(b.Context(), dataPath, kind)
						if err != nil {
							b.Fatal(err)
						}
						defer func() { _ = store.Close() }()
						scoped, delivery, closeDestination := ingestionDestination(b, store, root, transport)
						defer closeDestination()
						caps, err := delivery.Capabilities(b.Context())
						if err != nil {
							b.Fatal(err)
						}
						backend := &measuredBackend{Store: store}
						worker := dataengine.NewWorker()
						observed := &measuredDelivery{Delivery: delivery, worker: worker}
						ctx, cancel := context.WithTimeout(b.Context(), 5*time.Minute)
						done := make(chan struct{})
						defer func() { cancel(); <-done }()
						go func() {
							defer close(done)
							worker.Run(ctx, backend, func(err error) { b.Error(err) })
						}()
						var captured time.Time
						options := collector.Options{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: dataPath,
							Destination: &collector.Destination{Identity: "http://benchmark", DatabaseID: caps.DatabaseID, DatasetID: caps.DatasetID, Local: transport == "Direct", Transport: observed},
							SyncOptions: pipeline.SyncOptions{SourceDir: sources, Harnesses: harnesses},
							DeliveryProgress: func(collector.DeliveryProgress) {
								if captured.IsZero() {
									captured = time.Now()
								}
							}}
						if shape.name == "Unchanged" || shape.name == "Append" {
							if _, err := collector.Run(ctx, options); err != nil {
								b.Fatal(err)
							}
							if err := ingestionVisible(ctx, scoped); err != nil {
								b.Fatal(err)
							}
							if shape.name == "Append" {
								file, err := os.OpenFile(filepath.Join(sources, "session-0.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
								if err != nil {
									b.Fatal(err)
								}
								_, writeErr := file.WriteString(startupBenchmarkMessage(0, shape.messages))
								closeErr := file.Close()
								if writeErr != nil || closeErr != nil {
									b.Fatal(writeErr, closeErr)
								}
							}
							captured = time.Time{}
							observed.submit = 0
							backend.loadNS.Store(0)
							backend.publishNS.Store(0)
							backend.records.Store(0)
							backend.published.Store(0)
							backend.stale.Store(0)
						}
						stopProfile := ingestionProfile(b)
						b.StartTimer()
						start := time.Now()
						result, err := collector.Run(ctx, options)
						accepted := time.Now()
						if err != nil || result.Pending != 0 {
							b.Fatalf("acceptance: %+v %v", result, err)
						}
						if err := ingestionVisible(ctx, scoped); err != nil {
							b.Fatal(err)
						}
						visible := time.Now()
						query := analytics.Query{Selection: viewer.Selection{Period: "all", Bucket: "day"}, Tab: "sessions", Quality: "confirmed", Sort: "total", Direction: "desc", Page: 1, PageSize: 200}
						dashboard, err := sqlanalytics.LoadDashboard(ctx, scoped, query, time.Now())
						b.StopTimer()
						queried := time.Now()
						stopProfile()
						if err != nil || dashboard.Pending != 0 {
							b.Fatalf("query: %+v %v", dashboard, err)
						}
						if shape.name == "Native" {
							rawGolden(b, scoped)
						} else {
							messages := int64(shape.sessions * shape.messages)
							acceptedRecords := messages + int64(shape.sessions)
							switch shape.name {
							case "Append":
								messages++
								acceptedRecords = 1
							case "Unchanged":
								acceptedRecords = 0
							}
							if dashboard.Summary.TotalTokens != messages*startupBenchmarkTokens || dashboard.Summary.SessionCount != int64(shape.sessions) || result.Accepted != acceptedRecords {
								b.Fatalf("totals: %+v", dashboard.Summary)
							}
							var components [7]int64
							err := scoped.SQL().QueryRowContext(ctx, `SELECT COUNT(*),CAST(SUM(input_tokens) AS BIGINT),CAST(SUM(output_tokens) AS BIGINT),CAST(SUM(reasoning_tokens) AS BIGINT),CAST(SUM(cache_read_tokens) AS BIGINT),CAST(SUM(cache_write_tokens) AS BIGINT),CAST(SUM(total_tokens) AS BIGINT) FROM analytics_confirmed WHERE countable AND dataset_id=?`, caps.DatasetID).Scan(&components[0], &components[1], &components[2], &components[3], &components[4], &components[5], &components[6])
							if err != nil || components != [7]int64{messages, messages * 100, messages * 20, 0, 0, 0, messages * startupBenchmarkTokens} {
								b.Fatalf("component totals: %v %v", components, err)
							}
						}
						metrics["capture-ns/op"] += captured.Sub(start).Nanoseconds()
						metrics["delivery-ns/op"] += accepted.Sub(captured).Nanoseconds()
						metrics["submit-ns/op"] += observed.submit.Nanoseconds()
						metrics["visibility-lag-ns/op"] += visible.Sub(accepted).Nanoseconds()
						metrics["query-ns/op"] += queried.Sub(visible).Nanoseconds()
						metrics["load-work-ns/op"] += backend.loadNS.Load()
						metrics["publish-ns/op"] += backend.publishNS.Load()
						metrics["processed-records/op"] += backend.records.Load()
						metrics["published/op"] += backend.published.Load()
						metrics["stale/op"] += backend.stale.Load()
						metrics["accepted/op"] += result.Accepted
					}()
				}
				for unit, value := range metrics {
					b.ReportMetric(float64(value)/float64(b.N), unit)
				}
			})
		}
	}
}
