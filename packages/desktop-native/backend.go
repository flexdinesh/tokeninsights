package main

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/session"
)

// sessionBackend is a window's model.Backend over its own session lease.
type sessionBackend struct{ session *session.Session }

func (b sessionBackend) Instance(ctx context.Context) (application.Instance, error) {
	return b.session.Query.Instance(ctx)
}

// Capture requests default capture, as the web's Refresh does.
func (b sessionBackend) Capture(ctx context.Context) error {
	return b.session.Capture(ctx, false, "")
}

func (b sessionBackend) Status(ctx context.Context) (application.Status, error) {
	return b.session.Query.Status(ctx)
}

// Progress reads the collection attempts the web's progress shows,
// without the per-harness capture detail.
func (b sessionBackend) Progress(ctx context.Context) (collectorprogress.Snapshot, error) {
	detail, err := b.session.Progress(ctx)
	return detail.Collection, err
}
