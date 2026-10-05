package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
)

const contractTime int64 = 1767225600000

func contractFact(request string) publication.Fact {
	f := publication.Fact{Harness: "pi", Session: publication.Session{Harness: "pi", NativeID: "fixture-session-A", FirstOccurredAtMs: contractTime, LastOccurredAtMs: contractTime},
		Message: &publication.Message{NativeID: request, OccurredAtMs: contractTime}, OccurredAtMs: contractTime,
		Provider: "unknown", ProviderSource: "unknown", Model: "unknown", UsageScope: "message", Quality: "exact", Countable: true,
		InputTokens: 80, OutputTokens: 20, TotalTokens: 100}
	publication.SetIDs(&f)
	return f
}

func contractStore(t *testing.T) (*serverstore.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "server.sqlite")
	s, err := serverstore.CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func contractBatch(t *testing.T, s *serverstore.Store, stream, batch string, facts ...publication.Fact) []byte {
	t.Helper()
	m, err := s.Metadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b := publication.Batch{ProtocolVersion: 1, IdentityVersion: 1, SemanticsVersion: 1, DatabaseID: m.DatabaseID, StreamID: stream, BatchID: batch, FromSequence: 1, ToSequence: int64(len(facts))}
	for i, f := range facts {
		b.Entries = append(b.Entries, publication.Entry{Sequence: int64(i + 1), Fact: f})
	}
	body, err := publication.EncodeBatch(b)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func contractResponse(t *testing.T, server *httptest.Server, body []byte) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/ingestion/batches", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}

func contractPost(t *testing.T, server *httptest.Server, body []byte, wantStatus int) []byte {
	t.Helper()
	status, data := contractResponse(t, server, body)
	if status != wantStatus {
		if wantStatus == 0 && (status == http.StatusOK || status == http.StatusServiceUnavailable) {
			return data
		}
		t.Fatalf("HTTP status=%d want=%d body=%s", status, wantStatus, data)
	}
	return data
}

type contractRow struct {
	ID, Harness, Session, Message, Provider, ProviderSource, Model, Scope, Quality string
	Occurred, Countable, Input, Output, Reasoning, CacheRead, CacheWrite, Total    int64
}

type contractBlockedBody struct {
	reader  io.Reader
	ready   chan<- struct{}
	release <-chan struct{}
	once    sync.Once
}

func (b *contractBlockedBody) Read(p []byte) (int, error) {
	b.once.Do(func() { b.ready <- struct{}{} })
	<-b.release
	return b.reader.Read(p)
}
func (*contractBlockedBody) Close() error { return nil }

func contractAssert(t *testing.T, s *serverstore.Store, receipts int, facts ...publication.Fact) {
	t.Helper()
	rows, err := s.SQL().Query(`SELECT u.semantic_key,u.harness,s.session_id,COALESCE(m.harness_message_id,''),u.provider,u.provider_source,u.model,u.usage_scope,u.quality,u.recorded_at_ms,u.is_countable,u.input_tokens,u.output_tokens,u.reasoning_tokens,u.cache_read_tokens,u.cache_write_tokens,u.total_tokens FROM canonical_token_usage u JOIN canonical_sessions s ON s.id=u.session_id LEFT JOIN canonical_messages m ON m.id=u.message_id ORDER BY u.semantic_key`)
	if err != nil {
		t.Fatal(err)
	}
	got := []contractRow{}
	for rows.Next() {
		var r contractRow
		if err := rows.Scan(&r.ID, &r.Harness, &r.Session, &r.Message, &r.Provider, &r.ProviderSource, &r.Model, &r.Scope, &r.Quality, &r.Occurred, &r.Countable, &r.Input, &r.Output, &r.Reasoning, &r.CacheRead, &r.CacheWrite, &r.Total); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	want := []contractRow{}
	for _, f := range facts {
		message := ""
		if f.Message != nil {
			message = f.Message.NativeID
		}
		var countable int64
		if f.Countable {
			countable = 1
		}
		want = append(want, contractRow{f.ID, f.Harness, f.Session.NativeID, message, f.Provider, f.ProviderSource, f.Model, f.UsageScope, f.Quality, f.OccurredAtMs, countable, f.InputTokens, f.OutputTokens, f.ReasoningTokens, f.CacheReadTokens, f.CacheWriteTokens, f.TotalTokens})
	}
	sort.Slice(want, func(i, j int) bool { return want[i].ID < want[j].ID })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exact canonical facts differ: got=%+v want=%+v", got, want)
	}
	var count int
	if err := s.SQL().QueryRow("SELECT COUNT(*) FROM ingestion_receipts").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != receipts {
		t.Fatalf("durable receipts=%d want=%d", count, receipts)
	}
	if len(facts) == 0 {
		for _, table := range []string{"canonical_sessions", "canonical_messages", "usage_locations"} {
			if err := s.SQL().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("partial %s persisted: %d", table, count)
			}
		}
	}
}

// F01/G01/G02/G05: fresh delivery streams cannot change native fact identity.
func TestFailureContractFreshStreamsPreserveDistinctRequests(t *testing.T) {
	s, _ := contractStore(t)
	server := httptest.NewServer(NewHandler(NewCore(s)))
	defer server.Close()
	a, b := contractFact("fixture-request-A"), contractFact("fixture-request-B")
	if a.ID != "dbbdd82e66a44eda576e1f5cbfe531465198b525eeb6e70f874f4b27896a2db9" {
		t.Fatal("independent A identity vector differs")
	}
	for i, stream := range []string{"old-stream", "rebuilt-stream", "another-rebuild"} {
		contractPost(t, server, contractBatch(t, s, stream, "batch", a, b), http.StatusOK)
		contractAssert(t, s, i+1, a, b)
	}
	var total int64
	if err := s.SQL().QueryRow("SELECT SUM(total_tokens) FROM canonical_token_usage WHERE is_countable=1").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 200 {
		t.Fatalf("independent total=%d want=200", total)
	}
}

// F02/G05/G06: the stored receipt survives response loss and database reopen.
func TestFailureContractLostResponseReopenReturnsExactReceipt(t *testing.T) {
	s, path := contractStore(t)
	core := NewCore(s)
	a, b := contractFact("fixture-request-A"), contractFact("fixture-request-B")
	body := contractBatch(t, s, "stream", "lost-ack", a, b)
	done := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err == nil {
			_, err = core.Ingest(r.Context(), data)
		}
		done <- err
		connection, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = connection.Close()
		}
	}))
	server.Client().Timeout = 10 * time.Second
	r, err := server.Client().Post(server.URL, "application/json", bytes.NewReader(body))
	if r != nil {
		_ = r.Body.Close()
	}
	if err == nil {
		t.Fatal("lost response unexpectedly acknowledged")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not reach the lost-response commit")
	}
	server.Close()
	contractAssert(t, s, 1, a, b)
	var original string
	if err := s.SQL().QueryRow("SELECT receipt_json FROM ingestion_receipts").Scan(&original); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := serverstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	replay := httptest.NewServer(NewHandler(NewCore(reopened)))
	defer replay.Close()
	receipt := contractPost(t, replay, body, http.StatusOK)
	if strings.TrimSpace(string(receipt)) != original {
		t.Fatalf("durable receipt replaced: got=%s want=%s", receipt, original)
	}
	contractAssert(t, reopened, 1, a, b)
}

// F03/F04/G05/G06: conflict rejects the whole batch, including new facts.
func TestFailureContractBatchAndFactConflictsAreAtomic(t *testing.T) {
	s, _ := contractStore(t)
	server := httptest.NewServer(NewHandler(NewCore(s)))
	defer server.Close()
	a, b := contractFact("fixture-request-A"), contractFact("fixture-request-B")
	first := contractPost(t, server, contractBatch(t, s, "old", "same", a), http.StatusOK)
	changed := a
	changed.OutputTokens = 40
	changed.TotalTokens = 120
	contractPost(t, server, contractBatch(t, s, "old", "same", changed), http.StatusConflict)
	contractPost(t, server, contractBatch(t, s, "rebuilt", "new", b, changed), http.StatusConflict)
	contractAssert(t, s, 1, a)
	if again := contractPost(t, server, contractBatch(t, s, "old", "same", a), http.StatusOK); !bytes.Equal(first, again) {
		t.Fatal("original receipt changed after conflicts")
	}
}

// F05/G05/G06: independent SQLite connections, not a shared in-memory dedupe map.
func TestFailureContractConcurrentDuplicateTransactions(t *testing.T) {
	for _, sameBatch := range []bool{true, false} {
		t.Run(map[bool]string{true: "same-batch", false: "fresh-batches"}[sameBatch], func(t *testing.T) {
			s, path := contractStore(t)
			other, err := serverstore.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = other.Close() }()
			servers := []*httptest.Server{httptest.NewServer(NewHandler(NewCore(s))), httptest.NewServer(NewHandler(NewCore(other)))}
			defer servers[0].Close()
			defer servers[1].Close()
			a, b := contractFact("fixture-request-A"), contractFact("fixture-request-B")
			bodies := [][]byte{contractBatch(t, s, "stream", "batch", a, b), contractBatch(t, s, "stream", "batch", a, b)}
			if !sameBatch {
				bodies[1] = contractBatch(t, s, "new-stream", "new-batch", a, b)
			}
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range servers {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					// SQLite can reject a contending read-to-write upgrade.
					// Recovery replays the same immutable request.
					contractPost(t, servers[i], bodies[i], 0)
				}(i)
			}
			close(start)
			wg.Wait()
			for i := range servers {
				contractPost(t, servers[i], bodies[i], http.StatusOK)
			}
			receipts := 2
			if sameBatch {
				receipts = 1
			}
			contractAssert(t, s, receipts, a, b)
		})
	}
}

// F05/G05/G06: envelope repetition cannot create extra contributions, and
// contradictions inside an otherwise valid envelope never partially commit.
func TestFailureContractDuplicateEntitiesWithinBatch(t *testing.T) {
	for _, conflicting := range []bool{false, true} {
		t.Run(map[bool]string{false: "identical", true: "conflicting"}[conflicting], func(t *testing.T) {
			s, _ := contractStore(t)
			server := httptest.NewServer(NewHandler(NewCore(s)))
			defer server.Close()
			a := contractFact("fixture-request-A")
			duplicate := a
			if conflicting {
				duplicate.OutputTokens = 40
				duplicate.TotalTokens = 120
			}
			status := http.StatusOK
			if conflicting {
				status = http.StatusConflict
			}
			contractPost(t, server, contractBatch(t, s, "stream", "internal-repeat", a, duplicate), status)
			if conflicting {
				contractAssert(t, s, 0)
			} else {
				contractAssert(t, s, 1, a)
			}
		})
	}
}

// F05/G05: conflicting bodies race through independent SQLite connections.
// Either transaction may win, but only its one contribution and receipt persist.
func TestFailureContractConcurrentBatchBodyConflict(t *testing.T) {
	s, path := contractStore(t)
	other, err := serverstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	servers := []*httptest.Server{httptest.NewServer(NewHandler(NewCore(s))), httptest.NewServer(NewHandler(NewCore(other)))}
	defer servers[0].Close()
	defer servers[1].Close()
	a := contractFact("fixture-request-A")
	changed := a
	changed.OutputTokens = 40
	changed.TotalTokens = 120
	facts := []publication.Fact{a, changed}
	bodies := [][]byte{contractBatch(t, s, "stream", "conflicting-batch", a), contractBatch(t, s, "stream", "conflicting-batch", changed)}
	type outcome struct {
		index, status int
		receipt       []byte
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	for i := range servers {
		go func(i int) {
			<-start
			status, receipt := contractResponse(t, servers[i], bodies[i])
			outcomes <- outcome{i, status, receipt}
		}(i)
	}
	close(start)
	winner := -1
	var winningReceipt []byte
	deadline := time.After(12 * time.Second)
	for i := 0; i < 2; i++ {
		var result outcome
		select {
		case result = <-outcomes:
		case <-deadline:
			t.Fatal("concurrent batch did not complete")
		}
		switch result.status {
		case http.StatusOK:
			if winner != -1 {
				t.Fatal("two conflicting bodies received successful receipts")
			}
			winner = result.index
			winningReceipt = result.receipt
		case http.StatusConflict, http.StatusServiceUnavailable:
		default:
			t.Fatalf("unexpected contender status=%d", result.status)
		}
	}
	if winner < 0 {
		t.Fatal("neither conflicting batch committed")
	}
	loser := 1 - winner
	contractPost(t, servers[loser], bodies[loser], http.StatusConflict)
	if again := contractPost(t, servers[winner], bodies[winner], http.StatusOK); !bytes.Equal(again, winningReceipt) {
		t.Fatal("winning durable receipt changed")
	}
	contractAssert(t, s, 1, facts[winner])
}

// F06/G05: native source occurrence orders Claude revisions; stream sequence does not.
func TestFailureContractClaudeSourceRevisionOrder(t *testing.T) {
	for _, newFirst := range []bool{false, true} {
		t.Run(map[bool]string{true: "new-before-old", false: "old-before-new"}[newFirst], func(t *testing.T) {
			s, _ := contractStore(t)
			server := httptest.NewServer(NewHandler(NewCore(s)))
			defer server.Close()
			old := contractFact("fixture-request-A")
			old.Harness = "claude-code"
			old.Session.Harness = "claude-code"
			old.NativeRequestID = "fixture-native-request"
			old.Revision = &publication.SourceRevision{Rule: publication.ClaudeRevisionRule, Value: contractTime}
			publication.SetIDs(&old)
			newer := old
			newer.OccurredAtMs++
			newer.Session.LastOccurredAtMs++
			newer.OutputTokens = 40
			newer.TotalTokens = 120
			newer.Revision = &publication.SourceRevision{Rule: publication.ClaudeRevisionRule, Value: newer.OccurredAtMs}
			ordered := []publication.Fact{old, newer}
			if newFirst {
				ordered = []publication.Fact{newer, old}
			}
			contractPost(t, server, contractBatch(t, s, "stream-high-seq", "first", ordered[0]), http.StatusOK)
			contractPost(t, server, contractBatch(t, s, "fresh-stream-low-seq", "second", ordered[1]), http.StatusOK)
			contractAssert(t, s, 2, newer)
			clash := newer
			clash.OutputTokens = 41
			clash.TotalTokens = 121
			contractPost(t, server, contractBatch(t, s, "another-stream", "clash", clash), http.StatusConflict)
			contractAssert(t, s, 2, newer)
		})
	}
}

// F07: each server DB is one trusted owner; a foreign database binding cannot ingest.
func TestFailureContractSeparateOwnerDatabases(t *testing.T) {
	a := contractFact("fixture-request-A")
	first, _ := contractStore(t)
	second, _ := contractStore(t)
	left := httptest.NewServer(NewHandler(NewCore(first)))
	defer left.Close()
	right := httptest.NewServer(NewHandler(NewCore(second)))
	defer right.Close()
	foreign := contractBatch(t, first, "same-stream", "same-batch", a)
	contractPost(t, left, foreign, http.StatusOK)
	contractPost(t, right, foreign, http.StatusConflict)
	contractAssert(t, second, 0)
	contractPost(t, right, contractBatch(t, second, "same-stream", "same-batch", a), http.StatusOK)
	contractAssert(t, first, 1, a)
	contractAssert(t, second, 1, a)
}

// F08/G06: SQLite failures in facts or receipt persistence roll back all references.
func TestFailureContractSQLiteFailureRollsBackWholeBatch(t *testing.T) {
	for _, trigger := range []string{
		`CREATE TRIGGER fault BEFORE INSERT ON canonical_token_usage WHEN NEW.output_tokens=21 BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`,
		`CREATE TRIGGER fault BEFORE INSERT ON ingestion_receipts BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`,
	} {
		t.Run(trigger, func(t *testing.T) {
			s, _ := contractStore(t)
			server := httptest.NewServer(NewHandler(NewCore(s)))
			defer server.Close()
			a, b := contractFact("fixture-request-A"), contractFact("fixture-request-B")
			b.OutputTokens = 21
			b.TotalTokens = 101
			body := contractBatch(t, s, "stream", "retry", a, b)
			if _, err := s.SQL().Exec(trigger); err != nil {
				t.Fatal(err)
			}
			contractPost(t, server, body, http.StatusInternalServerError)
			contractAssert(t, s, 0)
			if _, err := s.SQL().Exec("DROP TRIGGER fault"); err != nil {
				t.Fatal(err)
			}
			contractPost(t, server, body, http.StatusOK)
			contractAssert(t, s, 1, a, b)
		})
	}
}

// F09/F10/F12/G06/G08/G09: valid earlier entries cannot leak through later invalid ones.
func TestFailureContractValidationPrivacyAndCompatibility(t *testing.T) {
	a, b := contractFact("fixture-request-A"), contractFact("fixture-request-B")
	// Distinct values ensure field mutations below affect the second entry.
	// The valid first entry must never survive a later invalid fact.
	b.OutputTokens, b.TotalTokens, b.Quality = 21, 101, "derived"
	for name, alter := range map[string]func([]byte) []byte{
		"negative": func(v []byte) []byte {
			return bytes.Replace(v, []byte(`"outputTokens":20`), []byte(`"outputTokens":-1`), 1)
		},
		"fraction": func(v []byte) []byte {
			return bytes.Replace(v, []byte(`"outputTokens":20`), []byte(`"outputTokens":0.5`), 1)
		},
		"overflow": func(v []byte) []byte {
			return bytes.Replace(v, []byte(`"outputTokens":20`), []byte(`"outputTokens":9223372036854775808`), 1)
		},
		"precision": func(v []byte) []byte {
			return bytes.Replace(v, []byte(`"outputTokens":20`), []byte(`"outputTokens":9007199254740993`), 1)
		},
		"total": func(v []byte) []byte {
			return bytes.Replace(v, []byte(`"totalTokens":100`), []byte(`"totalTokens":999`), 1)
		},
		"missing": func(v []byte) []byte { return bytes.Replace(v, []byte(`"outputTokens":20,`), nil, 1) },
		"session identity": func(v []byte) []byte {
			return bytes.Replace(v, []byte(`"nativeId":"fixture-session-A"`), []byte(`"nativeId":"missing-session"`), 1)
		},
		"private": func(v []byte) []byte {
			return bytes.Replace(v, []byte(`"quality":"exact"`), []byte(`"quality":"exact","promptText":"SYNTHETIC_PRIVATE_MARKER"`), 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, path := contractStore(t)
			server := httptest.NewServer(NewHandler(NewCore(s)))
			defer server.Close()
			body := contractBatch(t, s, "stream", "invalid", b, a)
			response := contractPost(t, server, alter(body), http.StatusBadRequest)
			contractAssert(t, s, 0)
			if bytes.Contains(response, []byte("SYNTHETIC_PRIVATE_MARKER")) {
				t.Fatal("private value echoed")
			}
			for _, file := range []string{path, path + "-wal"} {
				data, err := os.ReadFile(file)
				if err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if bytes.Contains(data, []byte("SYNTHETIC_PRIVATE_MARKER")) {
					t.Fatal("private value stored")
				}
			}
		})
	}
	s, _ := contractStore(t)
	server := httptest.NewServer(NewHandler(NewCore(s)))
	defer server.Close()
	contractPost(t, server, contractBatch(t, s, "stream", "original", a), http.StatusOK)
	bad := bytes.Replace(contractBatch(t, s, "stream", "newer", b), []byte(`"protocolVersion":1`), []byte(`"protocolVersion":2`), 1)
	contractPost(t, server, bad, http.StatusUnprocessableEntity)
	contractAssert(t, s, 1, a)
}

// F10: the accepted safe domain includes projected owner totals, not only each field.
func TestFailureContractAggregateOverflowIsAtomic(t *testing.T) {
	s, _ := contractStore(t)
	server := httptest.NewServer(NewHandler(NewCore(s)))
	defer server.Close()
	a := contractFact("fixture-request-A")
	a.InputTokens = publication.SafeInteger
	a.OutputTokens = 0
	a.TotalTokens = publication.SafeInteger
	contractPost(t, server, contractBatch(t, s, "stream", "max", a), http.StatusOK)
	b := contractFact("fixture-request-B")
	b.InputTokens = 1
	b.OutputTokens = 0
	b.TotalTokens = 1
	contractPost(t, server, contractBatch(t, s, "stream", "overflow", b), http.StatusBadRequest)
	contractAssert(t, s, 1, a)
}

// F11/G10: body/admission/SQLite contention failures preserve durable retry inputs.
func TestFailureContractAdmissionBodyAndBusyRetry(t *testing.T) {
	s, path := contractStore(t)
	core := NewCore(s)
	server := httptest.NewServer(NewHandler(core))
	defer server.Close()
	a, b := contractFact("fixture-request-A"), contractFact("fixture-request-B")
	body := contractBatch(t, s, "stream", "retry", a, b)
	contractPost(t, server, bytes.Repeat([]byte(" "), publication.MaxBodyBytes+1), http.StatusRequestEntityTooLarge)
	// Hold real handler requests while reading their bodies. A fifth request
	// must be rejected before its body enters unbounded parse/write work.
	ready := make(chan struct{}, MaxConcurrentRequests)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	finished := make(chan int, MaxConcurrentRequests)
	handler := NewHandler(core)
	for i := 0; i < MaxConcurrentRequests; i++ {
		go func() {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/ingestion/batches", nil)
			request.Body = &contractBlockedBody{reader: strings.NewReader("{}"), ready: ready, release: release}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			finished <- response.Code
		}()
	}
	deadline := time.After(10 * time.Second)
	for i := 0; i < MaxConcurrentRequests; i++ {
		select {
		case <-ready:
		case <-deadline:
			t.Fatal("admission did not reach bounded body-read barrier")
		}
	}
	contractPost(t, server, body, http.StatusServiceUnavailable)
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < MaxConcurrentRequests; i++ {
		select {
		case status := <-finished:
			if status != http.StatusBadRequest {
				t.Fatalf("held invalid request status=%d", status)
			}
		case <-deadline:
			t.Fatal("admitted requests did not finish")
		}
	}
	contractAssert(t, s, 0)
	other, err := serverstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if _, err := s.SQL().Exec("PRAGMA busy_timeout=1"); err != nil {
		t.Fatal(err)
	}
	tx, err := other.SQL().Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE server_metadata SET revision=revision WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	contractPost(t, server, body, http.StatusServiceUnavailable)
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	contractAssert(t, s, 0)
	contractPost(t, server, body, http.StatusOK)
	contractAssert(t, s, 1, a, b)
}

// F11/G06: inclusive limits are accepted; one extra byte/entity/string byte
// cannot partially ingest or invalidate the previously committed receipt.
func TestFailureContractLimitsAtBoundary(t *testing.T) {
	s, _ := contractStore(t)
	server := httptest.NewServer(NewHandler(NewCore(s)))
	defer server.Close()
	a := contractFact("fixture-request-A")
	facts := make([]publication.Fact, publication.MaxEntries)
	for i := range facts {
		facts[i] = a
	}
	body := contractBatch(t, s, "stream", "at-limit", facts...)
	padded := append(bytes.Clone(body), bytes.Repeat([]byte(" "), publication.MaxBodyBytes-len(body))...)
	contractPost(t, server, padded, http.StatusOK)
	contractAssert(t, s, 1, a)
	contractPost(t, server, append(bytes.Clone(padded), ' '), http.StatusRequestEntityTooLarge)
	contractAssert(t, s, 1, a)
	var batch publication.Batch
	if err := json.Unmarshal(body, &batch); err != nil {
		t.Fatal(err)
	}
	batch.BatchID = "too-many"
	batch.ToSequence++
	batch.Entries = append(batch.Entries, publication.Entry{Sequence: batch.ToSequence, Fact: a})
	tooMany, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	contractPost(t, server, tooMany, http.StatusRequestEntityTooLarge)
	contractAssert(t, s, 1, a)
	b := contractFact("fixture-request-B")
	b.Model = strings.Repeat("x", publication.MaxStringBytes)
	contractPost(t, server, contractBatch(t, s, "stream", "string-at-limit", b), http.StatusOK)
	contractAssert(t, s, 2, a, b)
	c := contractFact("fixture-request-C")
	c.Model = strings.Repeat("x", publication.MaxStringBytes+1)
	batch.BatchID = "string-over-limit"
	batch.FromSequence = 1
	batch.ToSequence = 1
	batch.Entries = []publication.Entry{{Sequence: 1, Fact: c}}
	tooLong, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	contractPost(t, server, tooLong, http.StatusBadRequest)
	contractAssert(t, s, 2, a, b)
}

// F13/G10: an incomplete source set is not a deletion instruction.
func TestFailureContractSubsetUploadRetainsHistory(t *testing.T) {
	s, _ := contractStore(t)
	server := httptest.NewServer(NewHandler(NewCore(s)))
	defer server.Close()
	a, b := contractFact("fixture-request-A"), contractFact("fixture-request-B")
	contractPost(t, server, contractBatch(t, s, "old", "all", a, b), http.StatusOK)
	response := contractPost(t, server, contractBatch(t, s, "rebuilt", "remaining", a), http.StatusOK)
	var receipt publication.Receipt
	if err := json.Unmarshal(response, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Inserted != 0 || receipt.Noop != 1 {
		t.Fatalf("subset receipt=%+v", receipt)
	}
	contractAssert(t, s, 2, a, b)
}
