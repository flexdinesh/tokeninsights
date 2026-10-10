package analytics

import (
	"context"
	"time"
)

// Repository reads one authorized dataset. Implementations must return rows,
// totals and publication metadata from a consistent snapshot. No query can
// select a different dataset through user-supplied filter values.
type Repository interface {
	// Dashboard clamps page after counting; rows, summary and revisions agree.
	Dashboard(context.Context, Query, time.Time) (Dashboard, error)
	// AllDashboard returns complete sorted rows or an error when the limit is
	// exceeded, never a successful truncation or separately snapshotted pages.
	AllDashboard(context.Context, Query, time.Time, int) (Dashboard, error)
	// Each call owns its snapshot; separate calls may observe newer revisions.
	Facets(context.Context, Query, string, time.Time) (Facets, error)
	// Status distinguishes accepted inputs, published revision and rebuild target.
	Status(context.Context) (ProcessingStatus, error)
}
