package collectorprogress

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const attemptOne = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const attemptTwo = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestLeaseHeartbeatCrashAndConcurrentAttempts(t *testing.T) {
	now := time.Unix(1000, 0)
	r := New("instance")
	r.now = func() time.Time { return now }
	for _, id := range []string{attemptOne, attemptTwo} {
		if err := r.Apply(Message{Operation: "begin", AttemptID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Apply(Message{Operation: "update", AttemptID: attemptOne, Stage: "submitting", Harnesses: map[string]string{"codex": "complete"}, AcknowledgedBatches: 1, AcknowledgedEntries: 5, PendingKnown: true, Pending: 2}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * HeartbeatInterval)
	if err := r.Apply(Message{Operation: "heartbeat", AttemptID: attemptOne}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(HeartbeatInterval)
	snapshot := r.Snapshot()
	first, second := snapshot.Attempts[0], snapshot.Attempts[1]
	if first.Stage != "submitting" || first.AcknowledgedEntries != 5 || first.Pending != 2 || first.Harnesses["codex"] != "complete" {
		t.Fatal(first)
	}
	if second.Stage != "interrupted" || second.ErrorCode != "publisher_lost" || second.FinishedAtMS == 0 {
		t.Fatal(second)
	}
	if err := r.Apply(Message{Operation: "heartbeat", AttemptID: attemptTwo}); !errors.Is(err, ErrConflict) {
		t.Fatal("expired attempt revived", err)
	}
	if err := r.Apply(Message{Operation: "finish", AttemptID: attemptOne, Stage: "accepted", AcknowledgedBatches: 1, AcknowledgedEntries: 5}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(LeaseDuration)
	if r.Snapshot().Attempts[0].Stage != "accepted" {
		t.Fatal("finished attempt expired")
	}
	now = now.Add(TerminalRetention)
	if len(r.Snapshot().Attempts) != 0 {
		t.Fatal("terminal attempts retained forever")
	}
}

func TestCancellationCountsAndSnapshotOwnership(t *testing.T) {
	r := New("instance")
	if err := r.Apply(Message{Operation: "begin", AttemptID: attemptOne}); err != nil {
		t.Fatal(err)
	}
	harnesses := map[string]string{"pi": "running"}
	update := Message{Operation: "update", AttemptID: attemptOne, Stage: "capturing", Harnesses: harnesses, AcknowledgedBatches: 1, AcknowledgedEntries: 2}
	if err := r.Apply(update); err != nil {
		t.Fatal(err)
	}
	harnesses["pi"] = "failed"
	snapshot := r.Snapshot()
	if snapshot.Attempts[0].Harnesses["pi"] != "running" {
		t.Fatal("publisher mutated registry")
	}
	snapshot.Attempts[0].Harnesses["pi"] = "failed"
	if r.Snapshot().Attempts[0].Harnesses["pi"] != "running" {
		t.Fatal("reader mutated registry")
	}
	update.AcknowledgedEntries = 1
	if err := r.Apply(update); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted count decreased", err)
	}
	if err := r.Apply(Message{Operation: "finish", AttemptID: attemptOne, Stage: "interrupted", ErrorCode: "cancelled", AcknowledgedBatches: 1, AcknowledgedEntries: 2}); err != nil {
		t.Fatal(err)
	}
	if r.Snapshot().Attempts[0].Stage != "interrupted" {
		t.Fatal("cancelled attempt presented success")
	}
	if err := r.Apply(Message{Operation: "finish", AttemptID: attemptOne, Stage: "accepted", AcknowledgedBatches: 1, AcknowledgedEntries: 2}); !errors.Is(err, ErrConflict) {
		t.Fatal("terminal state overwritten", err)
	}
}

func TestBoundedRegistryDoesNotEvictActivePublisher(t *testing.T) {
	r := New("instance")
	for i := range MaxAttempts {
		if err := r.Apply(Message{Operation: "begin", AttemptID: fmt.Sprintf("%032x", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Apply(Message{Operation: "begin", AttemptID: attemptOne}); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	if err := r.Apply(Message{Operation: "finish", AttemptID: fmt.Sprintf("%032x", 0), Stage: "accepted"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(Message{Operation: "begin", AttemptID: attemptOne}); err != nil {
		t.Fatal(err)
	}
	if len(r.Snapshot().Attempts) != MaxAttempts {
		t.Fatal("registry bound lost")
	}
}

func TestHTTPRejectsSourceTextAndPublicWrites(t *testing.T) {
	r := New("instance")
	for _, body := range []string{
		`{"operation":"begin","attemptId":"` + attemptOne + `","sourcePath":"/secret/source"}`,
		`{"operation":"begin","attemptId":"/secret/source"}`,
		`{"operation":"update","attemptId":"` + attemptOne + `","stage":"/secret/source"}`,
		`{"operation":"finish","attemptId":"` + attemptOne + `","stage":"failed","errorCode":"/secret/source"}`,
		`{"operation":"begin","attemptId":"` + attemptOne + `","harnesses":{"/secret/source":"waiting"}}`,
		`{"operation":"begin","attemptId":"` + attemptOne + `"} {}`,
		strings.Repeat("x", MaxRequestBytes+1),
	} {
		response := httptest.NewRecorder()
		r.ControlHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "/secret/source") {
			t.Fatal("unsanitized response", response)
		}
	}
	if len(r.Snapshot().Attempts) != 0 {
		t.Fatal("rejected content stored")
	}
	if err := r.Apply(Message{Operation: "begin", AttemptID: attemptOne}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	r.ReadHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	var snapshot Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.InstanceID != "instance" || len(snapshot.Attempts) != 1 {
		t.Fatal(snapshot)
	}
	response = httptest.NewRecorder()
	r.ReadHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatal("public write allowed", response.Code)
	}
}

func TestConcurrentPublishersRemainIndependent(t *testing.T) {
	r := New("instance")
	var workers sync.WaitGroup
	for i := range MaxAttempts {
		workers.Go(func() {
			id := fmt.Sprintf("%032x", i)
			if err := r.Apply(Message{Operation: "begin", AttemptID: id}); err != nil {
				t.Error(err)
				return
			}
			if err := r.Apply(Message{Operation: "update", AttemptID: id, Stage: "submitting", AcknowledgedEntries: int64(i)}); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	for _, attempt := range r.Snapshot().Attempts {
		expected, err := strconv.ParseInt(attempt.AttemptID, 16, 64)
		if err != nil || attempt.AcknowledgedEntries != expected {
			t.Fatal("publisher state crossed attempts", attempt)
		}
	}
	r.InterruptAll()
	for _, attempt := range r.Snapshot().Attempts {
		if attempt.Stage != "interrupted" {
			t.Fatal(attempt)
		}
	}
}
