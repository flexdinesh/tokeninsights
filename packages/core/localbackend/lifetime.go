package localbackend

import (
	"sync"
	"time"
)

const (
	StartupGrace    = 15 * time.Second
	ReconnectGrace  = 2 * time.Second
	StreamHeartbeat = 5 * time.Second
)

// viewerState is a pure lifetime controller. A timer only asks Expired; a
// reconnect racing that timer either acquires a lease or observes shutdown.
type viewerState struct {
	viewers  int
	deadline time.Time
	stopping bool
}

func (s *viewerState) attach() bool {
	if s.stopping {
		return false
	}
	s.viewers++
	return true
}

func (s *viewerState) detach(now time.Time) {
	if s.viewers > 0 {
		s.viewers--
		if s.viewers == 0 {
			s.deadline = now.Add(ReconnectGrace)
		}
	}
}

func (s *viewerState) expired(now time.Time) bool {
	if s.viewers == 0 && !now.Before(s.deadline) {
		s.stopping = true
	}
	return s.stopping
}

type viewers struct {
	mu    sync.Mutex
	state viewerState
	wake  chan struct{}
}

func newViewers() *viewers {
	return &viewers{state: viewerState{deadline: time.Now().Add(StartupGrace)}, wake: make(chan struct{}, 1)}
}

func (v *viewers) attach() (func(), bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.state.attach() {
		return nil, false
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			v.mu.Lock()
			v.state.detach(time.Now())
			v.mu.Unlock()
			select {
			case v.wake <- struct{}{}:
			default:
			}
		})
	}, true
}
