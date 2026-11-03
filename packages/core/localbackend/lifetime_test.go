package localbackend

import (
	"testing"
	"time"
)

func TestViewerLifetimeStartupReconnectAndFinalDisconnect(t *testing.T) {
	now := time.Unix(100, 0)
	s := viewerState{deadline: now.Add(StartupGrace)}
	if s.expired(now.Add(StartupGrace - time.Nanosecond)) {
		t.Fatal("startup grace expired early")
	}
	if !s.attach() {
		t.Fatal("initial attachment failed")
	}
	if !s.attach() {
		t.Fatal("second attachment failed")
	}
	if s.expired(now.Add(2 * StartupGrace)) {
		t.Fatal("live viewers expired")
	}
	s.detach(now)
	if s.expired(now.Add(3 * StartupGrace)) {
		t.Fatal("closing one viewer stopped another")
	}
	s.detach(now)
	if s.expired(now.Add(ReconnectGrace - time.Nanosecond)) {
		t.Fatal("reconnect grace expired early")
	}
	if !s.attach() {
		t.Fatal("reconnect rejected during grace")
	}
	if s.expired(now.Add(4 * StartupGrace)) {
		t.Fatal("reconnected viewer expired")
	}
	s.detach(now.Add(4 * StartupGrace))
	if !s.expired(now.Add(4*StartupGrace + ReconnectGrace)) {
		t.Fatal("final viewer did not stop backend")
	}
	if s.attach() {
		t.Fatal("attachment admitted after shutdown committed")
	}
}

func TestViewerLifetimeWithoutInitialAttachment(t *testing.T) {
	now := time.Unix(100, 0)
	s := viewerState{deadline: now.Add(StartupGrace)}
	if !s.expired(now.Add(StartupGrace)) {
		t.Fatal("unclaimed backend did not stop")
	}
}
