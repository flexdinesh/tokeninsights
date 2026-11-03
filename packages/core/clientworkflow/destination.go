// Package clientworkflow composes destination discovery and feature negotiation.
package clientworkflow

import (
	"context"
	"fmt"

	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collector"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

type Session struct {
	URL         string
	Descriptor  application.Instance
	Query       *queryclient.Client
	Destination *collector.Destination
}

func Resolve(ctx context.Context, settings config.SyncSettings) (Session, error) {
	var result Session
	if err := settings.ValidateDestination(); err != nil {
		return result, err
	}
	result.URL = settings.ServerURL
	canonical, err := collector.CanonicalEndpoint(result.URL)
	if err != nil {
		return result, err
	}
	result.URL = canonical
	client, err := queryclient.New(result.URL, nil)
	if err != nil {
		return result, err
	}
	client = client.WithToken(settings.ServerToken)
	result.Descriptor, err = client.Descriptor(ctx)
	if err != nil {
		return result, err
	}
	if serverfeatures.Kind(result.Descriptor.Kind) != serverfeatures.Hosted {
		return result, fmt.Errorf("server kind mismatch; distributed mode requires an authenticated remote server")
	}
	result.Query = client.WithDataset(result.Descriptor.DatasetID)
	result.Destination = &collector.Destination{Transport: collector.HTTPDelivery{URL: result.URL, Token: settings.ServerToken}, Identity: result.URL, DatabaseID: result.Descriptor.DatabaseID, DatasetID: result.Descriptor.DatasetID}
	return result, nil
}

// VerifyIngestion negotiates the raw contract before capture can mutate local
// state. Read-only workflows never need access to the ingestion transport.
func (s Session) VerifyIngestion(ctx context.Context) error {
	if err := s.Require(serverfeatures.Ingest, serverfeatures.RawIngestion); err != nil {
		return err
	}
	if s.Destination == nil {
		return fmt.Errorf("ingestion destination unavailable")
	}
	capabilities, err := collector.NegotiateCapabilities(ctx, s.Destination)
	if err != nil {
		return err
	}
	if capabilities.DatabaseID != s.Descriptor.DatabaseID || capabilities.DatasetID != s.Descriptor.DatasetID {
		return fmt.Errorf("ingestion identity differs from server descriptor; reconnect")
	}
	return nil
}

func (s Session) Require(permission serverfeatures.Permission, capabilities ...serverfeatures.Capability) error {
	if !queryclient.HasPermission(s.Descriptor, permission) {
		return fmt.Errorf("server token lacks %s permission", permission)
	}
	available := queryclient.DescriptorCapabilities(s.Descriptor)
	for _, capability := range capabilities {
		if !available.Has(capability) {
			return fmt.Errorf("server does not support %s", capability)
		}
	}
	return nil
}
