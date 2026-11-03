package collectorprogress

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCaptureDetailsAreAttemptScopedAndAbsentFromHTTP(t *testing.T) {
	r := New("instance")
	for _, id := range []string{attemptOne, attemptTwo} {
		if err := r.Apply(Message{Operation: "begin", AttemptID: id}); err != nil {
			t.Fatal(err)
		}
	}
	reading := Capture{Phase: CaptureReading, TotalKnown: true, Total: 5, Checked: 2, Captured: 1, Unchanged: 1, Active: 1}
	if err := r.UpdateCapture(attemptOne, "codex", reading); err != nil {
		t.Fatal(err)
	}
	complete := Capture{Phase: CaptureComplete, TotalKnown: true, Total: 2, Checked: 2, Captured: 2}
	if err := r.UpdateCapture(attemptTwo, "codex", complete); err != nil {
		t.Fatal(err)
	}
	snapshot := r.Details()
	if snapshot.Captures[attemptOne]["codex"] != reading || snapshot.Captures[attemptTwo]["codex"] != complete {
		t.Fatal("attempt measurements mixed", snapshot)
	}
	snapshot.Captures[attemptOne]["codex"] = complete
	snapshot.Collection.Attempts[0].Harnesses["pi"] = "failed"
	if r.Details().Captures[attemptOne]["codex"] != reading || r.Snapshot().Attempts[0].Harnesses["pi"] != "" {
		t.Fatal("reader mutated registry")
	}
	response := httptest.NewRecorder()
	r.ReadHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	var wire Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(r.Snapshot())
	if err != nil || strings.TrimSpace(response.Body.String()) != string(expected) || !reflect.DeepEqual(wire, r.Snapshot()) {
		t.Fatal("HTTP progress contract changed", response.Body.String(), err)
	}
	response = httptest.NewRecorder()
	r.ControlHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"operation":"update","attemptId":"`+attemptOne+`","stage":"capturing","capture":{"total":5}}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatal("HTTP accepted Go-only capture details", response.Code)
	}
}

func TestCaptureRejectsInconsistentOrRegressingMeasurements(t *testing.T) {
	r := New("instance")
	if err := r.Apply(Message{Operation: "begin", AttemptID: attemptOne}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Capture{
		{Phase: CaptureReading, TotalKnown: true, Total: 2},
		{Phase: CaptureWaiting, TotalKnown: true, Total: 2, Active: 1},
		{Phase: CaptureReading, Total: 10},
		{Phase: CaptureReading, TotalKnown: true, Total: 2, Checked: 1},
		{Phase: CaptureReading, TotalKnown: true, Total: 2, Checked: 3, Captured: 3},
		{Phase: CaptureReading, TotalKnown: true, Total: 2, Active: 3},
		{Phase: CaptureComplete, TotalKnown: true, Total: 2, Checked: 1, Captured: 1},
		{Phase: CaptureComplete, TotalKnown: true, Total: 1, Checked: 1, Failed: 1},
		{Phase: CaptureComplete, TotalKnown: true, Total: 1, Checked: 1, Quarantined: 1},
		{Phase: CaptureFailed, TotalKnown: true, Total: 2, Active: 1},
		{Phase: "private-source-path"},
		{Phase: CaptureWaiting, TotalKnown: true, Total: -1},
		{Phase: CaptureWaiting, TotalKnown: true, Total: maxSafeCounter + 1},
	} {
		if err := r.UpdateCapture(attemptOne, "pi", bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted inconsistent measurement %+v: %v", bad, err)
		}
	}
	valid := Capture{Phase: CaptureReading, TotalKnown: true, Total: 3, Checked: 1, Captured: 1, Active: 1}
	if err := r.UpdateCapture(attemptOne, "/private/source", valid); !errors.Is(err, ErrInvalid) {
		t.Fatal("accepted source path as harness", err)
	}
	if err := r.UpdateCapture(attemptOne, "pi", valid); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Capture{
		{Phase: CaptureDiscovering},
		{Phase: CaptureReading, TotalKnown: true, Total: 4, Checked: 1, Captured: 1, Active: 1},
		{Phase: CaptureReading, TotalKnown: true, Total: 3, Active: 1},
	} {
		if err := r.UpdateCapture(attemptOne, "pi", bad); !errors.Is(err, ErrConflict) {
			t.Fatal("accepted regressing measurement", bad, err)
		}
	}
	if r.Details().Captures[attemptOne]["pi"] != valid {
		t.Fatal("rejection mutated capture")
	}
}

func TestCaptureLeaseAndShutdownPreserveRemainingWork(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "lease", true: "shutdown"}[shutdown], func(t *testing.T) {
			now := time.Unix(1000, 0)
			r := New("instance")
			r.now = func() time.Time { return now }
			if err := r.Apply(Message{Operation: "begin", AttemptID: attemptOne}); err != nil {
				t.Fatal(err)
			}
			capture := Capture{Phase: CaptureReading, TotalKnown: true, Total: 5, Checked: 2, Captured: 2, Active: 1}
			if err := r.UpdateCapture(attemptOne, "pi", capture); err != nil {
				t.Fatal(err)
			}
			if shutdown {
				r.InterruptAll()
			} else {
				now = now.Add(LeaseDuration)
			}
			interrupted := r.Details().Captures[attemptOne]["pi"]
			if interrupted.Phase != CaptureInterrupted || interrupted.Active != 0 || interrupted.Checked != 2 || interrupted.Total != 5 {
				t.Fatal("interruption fabricated completed work", interrupted)
			}
			if err := r.UpdateCapture(attemptOne, "pi", capture); !errors.Is(err, ErrConflict) {
				t.Fatal("revived interrupted capture", err)
			}
			now = now.Add(TerminalRetention)
			if details := r.Details(); len(details.Collection.Attempts) != 0 || len(details.Captures) != 0 {
				t.Fatal("expired details retained", details)
			}
		})
	}
}

func TestFinishedCaptureDoesNotBecomeSuccessfulOnAcceptance(t *testing.T) {
	r := New("instance")
	if err := r.Apply(Message{Operation: "begin", AttemptID: attemptOne}); err != nil {
		t.Fatal(err)
	}
	capture := Capture{Phase: CaptureFailed, TotalKnown: true, Total: 2, Checked: 2, Captured: 1, Failed: 1}
	if err := r.UpdateCapture(attemptOne, "pi", capture); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(Message{Operation: "finish", AttemptID: attemptOne, Stage: "failed", ErrorCode: "collection_failed", AcknowledgedEntries: 5}); err != nil {
		t.Fatal(err)
	}
	if actual := r.Details().Captures[attemptOne]["pi"]; actual != capture {
		t.Fatal("acceptance rewrote capture outcome", actual)
	}
}
