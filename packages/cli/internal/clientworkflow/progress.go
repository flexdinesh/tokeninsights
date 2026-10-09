package clientworkflow

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

func (s Session) Observe(ctx context.Context) *collectorprogress.Observer {
	if s.Local == nil || !queryclient.DescriptorCapabilities(s.Descriptor).Has(serverfeatures.CollectorProgress) {
		return collectorprogress.NewObserver(ctx, nil)
	}
	return collectorprogress.NewObserver(ctx, s.Local.PublishCollectorProgress)
}
