package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestion"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
)

// SQLite's libc timezone state is process-wide. Separate processes exercise the
// same TZ environment as a real server without racing or caching another zone.
func TestTimestampBucketsAcrossTimezones(t *testing.T) {
	for _, zone := range []string{"UTC", "Pacific/Kiritimati", "Etc/GMT+12", "Australia/Sydney", "America/New_York", "XXX-24"} {
		t.Run(zone, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=^TestTimestampBucketsSQLiteHelper$", "-test.count=1")
			for _, variable := range os.Environ() {
				if !strings.HasPrefix(variable, "TZ=") && !strings.HasPrefix(variable, "TOKENINSIGHTS_TEST_TIMESTAMP_BUCKETS=") {
					child.Env = append(child.Env, variable)
				}
			}
			child.Env = append(child.Env, "TZ="+zone, "TOKENINSIGHTS_TEST_TIMESTAMP_BUCKETS=1")
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("SQLite bucket verification: %v\n%s", err, output)
			}
		})
	}
}

func TestTimestampBucketsSQLiteHelper(t *testing.T) {
	if os.Getenv("TOKENINSIGHTS_TEST_TIMESTAMP_BUCKETS") == "" {
		return
	}
	zone := os.Getenv("TZ")
	location, err := time.LoadLocation(zone)
	if zone == "XXX-24" {
		location, err = time.FixedZone("UTC+24", 24*60*60), nil
	}
	if err != nil {
		t.Fatal(err)
	}
	store, path := timestampBucketStore(t)
	core := ingestion.NewCore(store)
	httpServer := httptest.NewServer(NewHandler(context.Background(), path, core, io.Discard, "127.0.0.1"))
	defer httpServer.Close()
	values := []int64{0, 1, publication.MaxTimestampMs - 1, publication.MaxTimestampMs}
	// Midnight, Monday/year rollover, leap day, and both DST transitions.
	for _, text := range []string{
		"2023-12-31T23:59:59Z", "2024-01-01T00:00:00Z", "2024-02-29T12:00:00Z",
		"2026-03-08T06:59:59Z", "2026-03-08T07:00:00Z",
		"2026-11-01T05:59:59Z", "2026-11-01T06:00:00Z",
		"2026-04-04T15:59:59Z", "2026-04-04T16:00:00Z",
		"2026-10-03T15:59:59Z", "2026-10-03T16:00:00Z",
	} {
		parsed, err := time.Parse(time.RFC3339, text)
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, parsed.UnixMilli())
	}
	for _, value := range values {
		id := strconv.FormatInt(value, 10)
		fact := publication.Fact{Harness: "pi", Session: publication.Session{Harness: "pi"}, Message: &publication.Message{NativeID: "request-" + id}, Provider: "unknown", ProviderSource: "unknown", Model: "unknown", UsageScope: "message", Quality: "exact", Countable: true, InputTokens: 80, OutputTokens: 20, TotalTokens: 100}
		fact.Session.NativeID = "session-" + id
		fact.Session.FirstOccurredAtMs, fact.Session.LastOccurredAtMs = value, value
		fact.OccurredAtMs, fact.Message.OccurredAtMs = value, value
		publication.SetIDs(&fact)
		metadata, err := store.Metadata(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		batch := publication.Batch{ProtocolVersion: publication.ProtocolVersion, IdentityVersion: publication.IdentityVersion, SemanticsVersion: publication.SemanticsVersion, DatabaseID: metadata.DatabaseID, StreamID: "stream", BatchID: "batch-" + id, FromSequence: 1, ToSequence: 1, Entries: []publication.Entry{{Sequence: 1, Fact: fact}}}
		body, err := publication.EncodeBatch(batch)
		if err != nil {
			t.Fatal(err)
		}
		response, err := httpServer.Client().Post(httpServer.URL+"/api/v1/ingestion/batches", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		receipt, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || readErr != nil {
			t.Fatalf("ingestion HTTP=%d error=%v body=%s", response.StatusCode, readErr, receipt)
		}
		if decoded, err := publication.DecodeReceipt(receipt); err != nil || publication.ValidateReceipt(decoded, batch, body) != nil {
			t.Fatalf("invalid receipt: %v", err)
		}
		local := time.UnixMilli(value).In(location)
		day := local.Format("2006-01-02")
		monday := local.AddDate(0, 0, -(int(local.Weekday())+6)%7).Format("2006-01-02")
		filter := db.Filter{SessionIDs: []string{fact.Session.NativeID}}
		for bucket, expected := range map[db.TimeBucket]string{
			db.BucketDay: day, db.BucketWeek: monday, db.BucketMonth: local.Format("2006-01"), db.BucketYear: local.Format("2006"),
		} {
			rows, err := db.ViewerTokenBuckets(context.Background(), store.SQL(), filter, bucket)
			if err != nil || len(rows) != 1 || rows[0].Bucket != expected || rows[0].TotalTokens != 100 {
				t.Fatalf("timestamp=%d zone=%s bucket=%s rows=%+v expected=%s error=%v", value, zone, bucket, rows, expected, err)
			}
			usage := timestampHTTPUsage(t, httpServer, string(bucket), fact.Session.NativeID)
			if len(usage.Rows) != 1 || usage.Rows[0].Key != expected || usage.Summary.Total != 100 || usage.Rows[0].Date != value {
				t.Fatalf("timestamp=%d zone=%s REST bucket=%s response=%+v", value, zone, bucket, usage)
			}
		}
		filter.DayFrom, filter.DayTo = day, day
		rows, err := db.AggregateTokens(context.Background(), store.SQL(), filter, db.GroupByDayHour)
		if err != nil || len(rows) != 1 || rows[0].Day != day || rows[0].Hour != local.Format("15:00") || rows[0].TotalTokens != 100 {
			t.Fatalf("timestamp=%d hour rows=%+v expected=%s error=%v", value, rows, local.Format("15:00"), err)
		}
	}
	for _, bucket := range []db.TimeBucket{db.BucketDay, db.BucketWeek, db.BucketMonth, db.BucketYear} {
		rows, err := db.ViewerTokenBuckets(context.Background(), store.SQL(), db.Filter{}, bucket)
		if err != nil {
			t.Fatalf("all-time bucket=%s error=%v", bucket, err)
		}
		var total int64
		for _, row := range rows {
			if row.Bucket == "" {
				t.Fatal("empty bucket")
			}
			total += row.TotalTokens
		}
		if want := int64(len(values)) * 100; total != want {
			t.Fatalf("all-time total=%d want=%d", total, want)
		}
	}
}

func timestampBucketStore(t *testing.T) (*serverstore.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "server.sqlite")
	store, err := serverstore.CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}

func timestampHTTPUsage(t *testing.T, server *httptest.Server, bucket, session string) serverapi.UsageResponse {
	t.Helper()
	values := url.Values{"period": {"all"}, "bucket": {bucket}, "session": {session}}
	response, err := server.Client().Get(server.URL + "/api/v1/usage?" + values.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("usage HTTP=%d error=%v body=%s", response.StatusCode, err, body)
	}
	var usage serverapi.UsageResponse
	if err := json.Unmarshal(body, &usage); err != nil {
		t.Fatal(err)
	}
	return usage
}
