package pipeline

import "sync"

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

// CaptureProgressEvent measures sources, not evidence records or sessions.
// Checked contains mutually exclusive finalized outcomes. Total is meaningful
// only when TotalKnown is true; cancellation may leave unchecked sources.
// Callbacks are serialized and must return promptly. Events contain no source
// identity, path, or native content.
type CaptureProgressEvent struct {
	Harness                                                          Harness
	Phase                                                            CapturePhase
	TotalKnown                                                       bool
	Total, Checked, Captured, Unchanged, Failed, Quarantined, Active int
}

type captureMeasurement struct {
	event  CaptureProgressEvent
	saving bool
}

// captureReporter is opt-in. Its lock serializes worker and writer snapshots;
// callers cannot observe partially updated counters.
type captureReporter struct {
	mu       sync.Mutex
	callback func(CaptureProgressEvent)
	states   []captureMeasurement
}

func newCaptureReporter(callback func(CaptureProgressEvent), harnesses []Harness) *captureReporter {
	if callback == nil {
		return nil
	}
	reporter := &captureReporter{callback: callback, states: make([]captureMeasurement, len(harnesses))}
	for i, harness := range harnesses {
		reporter.states[i].event.Harness = harness
	}
	return reporter
}

func (r *captureReporter) update(index int, apply func(*captureMeasurement)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state := &r.states[index]
	apply(state)
	r.callback(state.event)
}

func (r *captureReporter) discovering(index int) {
	r.update(index, func(state *captureMeasurement) { state.event.Phase = CaptureDiscovering })
}

func (r *captureReporter) discovered(index, total int) {
	r.update(index, func(state *captureMeasurement) {
		state.event.TotalKnown, state.event.Total = true, total
		state.event.Phase = CaptureWaiting
		if total == 0 {
			state.event.Phase = CaptureComplete
		}
	})
}

func captureWorkPhase(state *captureMeasurement) {
	switch {
	case state.saving:
		state.event.Phase = CaptureSaving
	case state.event.Active > 0:
		state.event.Phase = CaptureReading
	case state.event.Checked < state.event.Total:
		state.event.Phase = CaptureWaiting
	case state.event.Failed > 0 || state.event.Quarantined > 0:
		state.event.Phase = CaptureFailed
	default:
		state.event.Phase = CaptureComplete
	}
}

func (r *captureReporter) reading(index int, started bool) {
	r.update(index, func(state *captureMeasurement) {
		if started {
			state.event.Active++
		} else {
			state.event.Active--
		}
		captureWorkPhase(state)
	})
}

func (r *captureReporter) saving(index int) {
	r.update(index, func(state *captureMeasurement) {
		state.saving = true
		captureWorkPhase(state)
	})
}

func (r *captureReporter) finalized(index, count int, err error, quarantined bool) {
	r.update(index, func(state *captureMeasurement) {
		state.saving = false
		state.event.Checked++
		switch {
		case quarantined:
			state.event.Quarantined++
		case err != nil:
			state.event.Failed++
		case count > 0:
			state.event.Captured++
		default:
			state.event.Unchanged++
		}
		captureWorkPhase(state)
	})
}

func (r *captureReporter) stopped(index int, interrupted bool) {
	r.update(index, func(state *captureMeasurement) {
		state.event.Phase = CaptureFailed
		if interrupted {
			state.event.Phase = CaptureInterrupted
		}
	})
}

func (r *captureReporter) stoppedUnfinished(interrupted bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := range r.states {
		state := &r.states[index]
		if state.event.TotalKnown && (state.event.Checked < state.event.Total || interrupted && state.event.Failed > 0) {
			state.event.Phase = CaptureFailed
			if interrupted {
				state.event.Phase = CaptureInterrupted
			}
			r.callback(state.event)
		}
	}
}
