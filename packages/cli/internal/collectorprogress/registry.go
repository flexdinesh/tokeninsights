// Package collectorprogress holds sanitized, ephemeral client-published progress.
// It never starts collection or retains source evidence.
package collectorprogress

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

const (
	HeartbeatInterval = 5 * time.Second
	LeaseDuration     = 3 * HeartbeatInterval
	TerminalRetention = 5 * time.Minute
	MaxAttempts       = 32
	MaxRequestBytes   = 4096
	maxSafeCounter    = 1<<53 - 1
)

var (
	ErrInvalid     = errors.New("invalid collector progress")
	ErrMissing     = errors.New("collector progress attempt not found")
	ErrConflict    = errors.New("collector progress attempt conflict")
	ErrBusy        = errors.New("collector progress registry full")
	attemptPattern = regexp.MustCompile(`^(?:[a-f0-9]{32}|[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})$`)
)

// Message is a complete sanitized progress update. Heartbeats identify only the
// attempt; they cannot alter counts or terminal state.
type Message struct {
	Operation           string            `json:"operation"`
	AttemptID           string            `json:"attemptId"`
	Stage               string            `json:"stage,omitempty"`
	Harnesses           map[string]string `json:"harnesses,omitempty"`
	AcknowledgedBatches int64             `json:"acknowledgedBatches,omitempty"`
	AcknowledgedEntries int64             `json:"acknowledgedEntries,omitempty"`
	PendingKnown        bool              `json:"pendingKnown,omitempty"`
	Pending             int64             `json:"pending,omitempty"`
	ErrorCode           string            `json:"errorCode,omitempty"`
}

type Attempt struct {
	AttemptID           string            `json:"attemptId"`
	Stage               string            `json:"stage"`
	Harnesses           map[string]string `json:"harnesses"`
	AcknowledgedBatches int64             `json:"acknowledgedBatches"`
	AcknowledgedEntries int64             `json:"acknowledgedEntries"`
	PendingKnown        bool              `json:"pendingKnown"`
	Pending             int64             `json:"pending"`
	ErrorCode           string            `json:"errorCode,omitempty"`
	StartedAtMS         int64             `json:"startedAtMs"`
	UpdatedAtMS         int64             `json:"updatedAtMs"`
	ExpiresAtMS         int64             `json:"expiresAtMs"`
	FinishedAtMS        int64             `json:"finishedAtMs,omitempty"`
}

type Snapshot struct {
	InstanceID string    `json:"instanceId"`
	Attempts   []Attempt `json:"attempts"`
}

type Registry struct {
	mu         sync.Mutex
	instanceID string
	now        func() time.Time
	attempts   map[string]Attempt
	captures   map[string]map[string]Capture
}

func New(instanceID string) *Registry {
	return &Registry{instanceID: instanceID, now: time.Now, attempts: make(map[string]Attempt), captures: make(map[string]map[string]Capture)}
}

func validHarness(harness string) bool {
	switch harness {
	case "codex", "opencode", "claude-code", "pi":
		return true
	default:
		return false
	}
}

func terminal(stage string) bool {
	return stage == "accepted" || stage == "failed" || stage == "interrupted"
}

func validMessage(m Message) bool {
	if !attemptPattern.MatchString(m.AttemptID) || m.AcknowledgedBatches < 0 || m.AcknowledgedBatches > maxSafeCounter || m.AcknowledgedEntries < 0 || m.AcknowledgedEntries > maxSafeCounter || m.Pending < 0 || m.Pending > maxSafeCounter || !m.PendingKnown && m.Pending != 0 {
		return false
	}
	for harness, state := range m.Harnesses {
		if !validHarness(harness) {
			return false
		}
		switch state {
		case "waiting", "running", "complete", "failed", "skipped":
		default:
			return false
		}
	}
	switch m.ErrorCode {
	case "", "collection_failed", "submission_failed", "cancelled", "publisher_lost":
	default:
		return false
	}
	switch m.Operation {
	case "begin":
		return (m.Stage == "" || m.Stage == "waiting") && m.AcknowledgedBatches == 0 && m.AcknowledgedEntries == 0 && m.ErrorCode == ""
	case "heartbeat":
		return m.Stage == "" && len(m.Harnesses) == 0 && m.AcknowledgedBatches == 0 && m.AcknowledgedEntries == 0 && !m.PendingKnown && m.Pending == 0 && m.ErrorCode == ""
	case "update":
		return (m.Stage == "waiting" || m.Stage == "capturing" || m.Stage == "submitting") && m.ErrorCode == ""
	case "finish":
		return terminal(m.Stage) && (m.Stage != "accepted" || m.ErrorCode == "") && (m.Stage == "accepted" || m.ErrorCode != "")
	default:
		return false
	}
}

func cloneHarnesses(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func (r *Registry) expire(now time.Time) {
	for id, attempt := range r.attempts {
		if !terminal(attempt.Stage) && now.UnixMilli() >= attempt.ExpiresAtMS {
			attempt.Stage = "interrupted"
			attempt.ErrorCode = "publisher_lost"
			attempt.FinishedAtMS = attempt.ExpiresAtMS
			attempt.UpdatedAtMS = attempt.ExpiresAtMS
			r.attempts[id] = attempt
			r.finishCapture(id, attempt.Stage)
		}
		if terminal(attempt.Stage) && now.UnixMilli()-attempt.FinishedAtMS >= TerminalRetention.Milliseconds() {
			delete(r.attempts, id)
			delete(r.captures, id)
		}
	}
}

func (r *Registry) Apply(m Message) error {
	if !validMessage(m) {
		return ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.expire(now)
	attempt, exists := r.attempts[m.AttemptID]
	if m.Operation == "begin" {
		if exists {
			return ErrConflict
		}
		if len(r.attempts) >= MaxAttempts {
			var oldest string
			for id, candidate := range r.attempts {
				if terminal(candidate.Stage) && (oldest == "" || candidate.FinishedAtMS < r.attempts[oldest].FinishedAtMS) {
					oldest = id
				}
			}
			if oldest == "" {
				return ErrBusy
			}
			delete(r.attempts, oldest)
			delete(r.captures, oldest)
		}
		attempt = Attempt{AttemptID: m.AttemptID, Stage: "waiting", StartedAtMS: now.UnixMilli()}
	} else {
		if !exists {
			return ErrMissing
		}
		if terminal(attempt.Stage) {
			return ErrConflict
		}
	}
	if m.Operation != "heartbeat" {
		if m.AcknowledgedBatches < attempt.AcknowledgedBatches || m.AcknowledgedEntries < attempt.AcknowledgedEntries {
			return ErrConflict
		}
		if m.Stage != "" {
			attempt.Stage = m.Stage
		}
		attempt.Harnesses = cloneHarnesses(m.Harnesses)
		attempt.AcknowledgedBatches, attempt.AcknowledgedEntries = m.AcknowledgedBatches, m.AcknowledgedEntries
		attempt.PendingKnown, attempt.Pending, attempt.ErrorCode = m.PendingKnown, m.Pending, m.ErrorCode
	}
	attempt.UpdatedAtMS = now.UnixMilli()
	attempt.ExpiresAtMS = now.Add(LeaseDuration).UnixMilli()
	if m.Operation == "finish" {
		attempt.FinishedAtMS = now.UnixMilli()
		r.finishCapture(m.AttemptID, attempt.Stage)
	}
	r.attempts[m.AttemptID] = attempt
	return nil
}

func (r *Registry) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expire(r.now())
	return r.snapshotLocked()
}

func (r *Registry) snapshotLocked() Snapshot {
	snapshot := Snapshot{InstanceID: r.instanceID, Attempts: make([]Attempt, 0, len(r.attempts))}
	for _, attempt := range r.attempts {
		attempt.Harnesses = cloneHarnesses(attempt.Harnesses)
		snapshot.Attempts = append(snapshot.Attempts, attempt)
	}
	sort.Slice(snapshot.Attempts, func(i, j int) bool {
		left, right := snapshot.Attempts[i], snapshot.Attempts[j]
		if left.StartedAtMS != right.StartedAtMS {
			return left.StartedAtMS < right.StartedAtMS
		}
		return left.AttemptID < right.AttemptID
	})
	return snapshot
}

// InterruptAll ends outstanding attempts when their owning service stops.
func (r *Registry) InterruptAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now().UnixMilli()
	for id, attempt := range r.attempts {
		if terminal(attempt.Stage) {
			continue
		}
		attempt.Stage, attempt.ErrorCode = "interrupted", "publisher_lost"
		attempt.UpdatedAtMS, attempt.FinishedAtMS = now, now
		r.attempts[id] = attempt
		r.finishCapture(id, attempt.Stage)
	}
}
