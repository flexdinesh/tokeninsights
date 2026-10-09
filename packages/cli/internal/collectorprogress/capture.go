package collectorprogress

// Capture measurements are Go-only local progress. They are deliberately separate
// from Message, Attempt and Snapshot, which define the existing HTTP contract.
type CapturePhase string

const (
	CaptureDiscovering CapturePhase = "discovering"
	CaptureWaiting     CapturePhase = "waiting"
	CaptureReading     CapturePhase = "reading"
	CaptureSaving      CapturePhase = "saving"
	CaptureComplete    CapturePhase = "complete"
	CaptureFailed      CapturePhase = "failed"
	CaptureInterrupted CapturePhase = "interrupted"
)

// Checked counts finalized source outcomes, not successful sources alone.
// Failed excludes quarantined sources; Total is meaningful only when TotalKnown.
type Capture struct {
	Phase                               CapturePhase
	TotalKnown                          bool
	Total, Checked, Captured, Unchanged int64
	Failed, Quarantined, Active         int64
}

type DetailSnapshot struct {
	Collection Snapshot
	Captures   map[string]map[string]Capture // Attempt ID, then harness.
}

func captureTerminal(phase CapturePhase) bool {
	return phase == CaptureComplete || phase == CaptureFailed || phase == CaptureInterrupted
}

func validCapture(harness string, c Capture) bool {
	if !validHarness(harness) {
		return false
	}
	for _, count := range []int64{c.Total, c.Checked, c.Captured, c.Unchanged, c.Failed, c.Quarantined, c.Active} {
		if count < 0 || count > maxSafeCounter {
			return false
		}
	}
	if c.Checked != c.Captured+c.Unchanged+c.Failed+c.Quarantined ||
		c.TotalKnown && c.Checked+c.Active > c.Total ||
		!c.TotalKnown && (c.Total != 0 || c.Checked != 0 || c.Active != 0) {
		return false
	}
	switch c.Phase {
	case CaptureDiscovering:
		return !c.TotalKnown
	case CaptureWaiting:
		return c.TotalKnown && c.Active == 0
	case CaptureReading:
		return c.TotalKnown && c.Active > 0
	case CaptureSaving:
		return c.TotalKnown
	case CaptureComplete:
		return c.TotalKnown && c.Checked == c.Total && c.Failed == 0 && c.Quarantined == 0 && c.Active == 0
	case CaptureFailed, CaptureInterrupted:
		return c.Active == 0
	default:
		return false
	}
}

// UpdateCapture accepts only bounded sanitized measurements for a live attempt.
// Capture does no I/O here; observers may ignore rejection without failing work.
func (r *Registry) UpdateCapture(attemptID, harness string, capture Capture) error {
	if !attemptPattern.MatchString(attemptID) || !validCapture(harness, capture) {
		return ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expire(r.now())
	attempt, exists := r.attempts[attemptID]
	if !exists {
		return ErrMissing
	}
	if terminal(attempt.Stage) {
		return ErrConflict
	}
	measurements := r.captures[attemptID]
	if previous, exists := measurements[harness]; exists {
		cancelled := previous.Phase == CaptureFailed && capture.Phase == CaptureInterrupted &&
			capture.Checked == previous.Checked && capture.Captured == previous.Captured &&
			capture.Unchanged == previous.Unchanged && capture.Failed == previous.Failed && capture.Quarantined == previous.Quarantined
		if captureTerminal(previous.Phase) && !cancelled ||
			previous.TotalKnown && (!capture.TotalKnown || previous.Total != capture.Total) ||
			capture.Captured < previous.Captured || capture.Unchanged < previous.Unchanged ||
			capture.Failed < previous.Failed || capture.Quarantined < previous.Quarantined {
			return ErrConflict
		}
	}
	if measurements == nil {
		measurements = make(map[string]Capture)
		r.captures[attemptID] = measurements
	}
	measurements[harness] = capture
	return nil
}

// Details returns collection and measurements from one immutable local snapshot.
func (r *Registry) Details() DetailSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expire(r.now())
	result := DetailSnapshot{Collection: r.snapshotLocked(), Captures: make(map[string]map[string]Capture, len(r.captures))}
	for id, captures := range r.captures {
		copy := make(map[string]Capture, len(captures))
		for harness, capture := range captures {
			copy[harness] = capture
		}
		result.Captures[id] = copy
	}
	return result
}

func (r *Registry) finishCapture(attemptID, stage string) {
	for harness, capture := range r.captures[attemptID] {
		if captureTerminal(capture.Phase) {
			continue
		}
		capture.Phase, capture.Active = CaptureInterrupted, 0
		if stage == "failed" {
			capture.Phase = CaptureFailed
		}
		r.captures[attemptID][harness] = capture
	}
}
