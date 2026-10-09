package localruntime

import (
	"errors"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

func TestHostnameFailureNeverInventsIdentity(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "  workstation  ", want: "workstation"},
		{name: "", want: "unknown"},
		{name: "  ", want: "unknown"},
		{name: "private-host", err: errors.New("lookup failed"), want: "unknown"},
	} {
		got := resolveHostname(func() (string, error) { return test.name, test.err })
		if got != test.want {
			t.Fatalf("hostname %q, want %q", got, test.want)
		}
	}
}

func TestDisabledCollectorProgressPublishesNothing(t *testing.T) {
	policy, err := serverfeatures.NewLocalViewer(false)
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Policy: policy, Progress: collectorprogress.New("instance")}
	observer := runtime.Observe(t.Context())
	observer.Delivery(collector.DeliveryProgress{Accepted: 3, Batches: 1, PendingKnown: true})
	observer.Finish(collector.Result{Accepted: 3, Batches: 1, PendingKnown: true}, nil)
	if progress := runtime.Progress.Snapshot(); len(progress.Attempts) != 0 {
		t.Fatal("disabled progress published", progress)
	}
}
