package clientworkflow

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

func TestObserverPublishesAcknowledgedWorkAndAncillaryFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	registry := collectorprogress.New("instance")
	handler := registry.ControlHandler()
	var mu sync.Mutex
	failed := false
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TokenInsights-Instance") != "instance" || r.URL.Path != "/control/v1/collector-progress" {
			t.Error("unverified private publisher")
		}
		mu.Lock()
		unavailable := failed
		mu.Unlock()
		if unavailable {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	session := Session{Descriptor: api.InstanceResponseV2{Capabilities: []string{"collector-progress"}}, Local: &service.Client{Record: service.Record{InstanceID: "instance", Socket: path}}}
	observer := session.Observe(t.Context())
	observer.Collection(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSyncing})
	observer.Delivery(collector.DeliveryProgress{Batches: 2, Accepted: 5, PendingKnown: true, Pending: 3})
	snapshot := registry.Snapshot()
	if len(snapshot.Attempts) != 1 || snapshot.Attempts[0].Stage != "submitting" || snapshot.Attempts[0].AcknowledgedEntries != 5 || snapshot.Attempts[0].AcknowledgedBatches != 2 || snapshot.Attempts[0].Harnesses["pi"] != "running" {
		t.Fatal(snapshot)
	}
	observer.Finish(collector.Result{Batches: 2, Accepted: 5, PendingKnown: true}, nil)
	snapshot = registry.Snapshot()
	if snapshot.Attempts[0].Stage != "accepted" || snapshot.Attempts[0].Pending != 0 {
		t.Fatal(snapshot)
	}
	mu.Lock()
	failed = true
	mu.Unlock()
	// A lost publisher cannot stop collection or mask its original failure.
	observer = session.Observe(t.Context())
	observer.Collection(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressFailed})
	observer.Delivery(collector.DeliveryProgress{})
	observer.Finish(collector.Result{}, errors.New("collection failed"))
}

func TestObserverReportsCancellationAndHostedHasNoPublisher(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Missing advertised progress never contacts even an invalid private socket.
	session := Session{Descriptor: api.InstanceResponseV2{ServerKind: "hosted", Capabilities: []string{"usage"}}, Local: &service.Client{Record: service.Record{Socket: "/missing"}}}
	observer := session.Observe(ctx)
	if observer.publish != nil {
		t.Fatal("hosted progress publisher created")
	}
	observer.Finish(collector.Result{}, context.Canceled)
}
