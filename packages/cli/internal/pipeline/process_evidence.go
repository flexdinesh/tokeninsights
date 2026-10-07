package pipeline

import (
	"context"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
)

// ProcessEvidence preserves the legacy call site; interpretation belongs to processor.
func ProcessEvidence(ctx context.Context, records []evidence.Stored) (evidence.Projection, error) {
	return processor.Process(ctx, records)
}
