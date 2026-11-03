package localbackend

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

type readyStore struct{}

func (readyStore) Ready(context.Context) error { return nil }

func newTestDashboard(t *testing.T, grace time.Duration) *dashboard {
	t.Helper()
	d := newDashboard(t.Context(), readyStore{}, func(*dashboardBinding) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	})
	d.grace = grace
	t.Cleanup(d.close)
	return d
}

func serving(url string) bool {
	response, err := http.Get(url)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode == http.StatusNoContent
}

func waitUnbound(t *testing.T, d *dashboard) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if url, _ := d.descriptor(); url == "" {
			d.mu.Lock()
			draining := d.draining
			d.mu.Unlock()
			if draining == nil {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("dashboard listener did not unbind")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDashboardSharesEqualRequestsAndRejectsDifferentWhileBound(t *testing.T) {
	d := newTestDashboard(t, time.Hour)
	listen := localprotocol.Listen{Host: "127.0.0.1", Port: 0}
	url, releaseFirst, err := d.acquire(t.Context(), listen)
	if err != nil || !serving(url) {
		t.Fatal("first dashboard lease did not bind", err)
	}
	second, releaseSecond, err := d.acquire(t.Context(), listen)
	if err != nil || second != url {
		t.Fatal("equal request did not share listener", second, url, err)
	}
	if _, _, err := d.acquire(t.Context(), localprotocol.Listen{Host: "0.0.0.0", Port: 0}); !errors.Is(err, localprotocol.ErrSettingsMismatch) {
		t.Fatal("different listen accepted while bound", err)
	}
	if _, _, err := d.acquire(t.Context(), localprotocol.Listen{Host: "", Port: 0}); err == nil {
		t.Fatal("invalid listen accepted")
	}
	releaseFirst()
	releaseFirst() // Releases are idempotent per lease.
	if got, listen := d.descriptor(); got != url || listen == nil {
		t.Fatal("listener closed while another lease remained")
	}
	releaseSecond()
}

func TestDashboardGraceKeepsReloadsAndUnbindsAfterLastLease(t *testing.T) {
	d := newTestDashboard(t, 100*time.Millisecond)
	listen := localprotocol.Listen{Host: "127.0.0.1", Port: 0}
	url, release, err := d.acquire(t.Context(), listen)
	if err != nil {
		t.Fatal(err)
	}
	binding := d.current
	release()
	// A reload inside the grace window keeps the same binding.
	reloaded, ok := d.acquireBound(binding)
	if !ok {
		t.Fatal("reload rejected during grace")
	}
	time.Sleep(3 * d.grace)
	if got, _ := d.descriptor(); got != url || !serving(url) {
		t.Fatal("listener closed while a reloaded tab held it")
	}
	reloaded()
	waitUnbound(t, d)
	if serving(url) {
		t.Fatal("listener still serving after last lease")
	}
	if _, ok := d.acquireBound(binding); ok {
		t.Fatal("stale binding leased after unbind")
	}
	other, releaseOther, err := d.acquire(t.Context(), localprotocol.Listen{Host: "0.0.0.0", Port: 0})
	if err != nil || other == "" || !serving(other) {
		t.Fatal("different listen rejected after unbind", err)
	}
	releaseOther()
}

func TestDashboardAcquireDuringDrainWaitsThenRebinds(t *testing.T) {
	d := newTestDashboard(t, time.Hour)
	_, release, err := d.acquire(t.Context(), localprotocol.Listen{Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	binding := d.current
	release()
	d.mu.Lock()
	wait := make(chan struct{})
	d.current, d.draining = nil, wait
	d.mu.Unlock()
	acquired := make(chan error, 1)
	go func() {
		_, releaseNext, err := d.acquire(t.Context(), localprotocol.Listen{Host: "127.0.0.1", Port: 0})
		if err == nil {
			releaseNext()
		}
		acquired <- err
	}()
	select {
	case err := <-acquired:
		t.Fatal("acquire did not wait for drain", err)
	case <-time.After(50 * time.Millisecond):
	}
	d.shutdown(binding)
	d.mu.Lock()
	d.draining = nil
	close(wait)
	d.mu.Unlock()
	if err := <-acquired; err != nil {
		t.Fatal("acquire after drain failed", err)
	}
}

func TestDashboardCloseStopsAdmissionAndJoinsListener(t *testing.T) {
	d := newTestDashboard(t, time.Hour)
	url, _, err := d.acquire(t.Context(), localprotocol.Listen{Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	d.close()
	if serving(url) {
		t.Fatal("closed dashboard still serving")
	}
	if _, _, err := d.acquire(t.Context(), localprotocol.Listen{Host: "127.0.0.1", Port: 0}); !errors.Is(err, localprotocol.ErrShuttingDown) {
		t.Fatal("closed dashboard admitted lease", err)
	}
}
