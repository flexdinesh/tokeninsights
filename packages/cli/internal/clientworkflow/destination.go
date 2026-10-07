// Package clientworkflow composes destination discovery and feature negotiation.
package clientworkflow

import (
	"context"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

type Session struct {
	URL         string
	Descriptor  api.InstanceResponseV2
	Query       *queryclient.Client
	Destination *collector.Destination
	Local       *service.Client
	token       string
}

type Ensure func(context.Context) (service.State, error)

func Resolve(ctx context.Context, settings config.Settings, ensure Ensure) (Session, error) {
	var result Session
	result.token = settings.ServerToken
	if err := settings.ValidateDestination(); err != nil {
		return result, err
	}
	result.URL = settings.ServerURL
	if result.URL == "" {
		if ensure == nil {
			return result, fmt.Errorf("local server unavailable")
		}
		state, err := ensure(ctx)
		if err != nil {
			return result, err
		}
		if state.Record == nil {
			return result, fmt.Errorf("local query server unavailable")
		}
		result.URL = state.Record.URL
		if state.Record.Socket != "" {
			result.Local = &service.Client{Record: *state.Record}
		}
	}
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
	if serverfeatures.Kind(result.Descriptor.ServerKind) != settings.ServerKind {
		return result, fmt.Errorf("server kind mismatch; update server-kind or server-url")
	}
	result.Query = client.WithDataset(result.Descriptor.DatasetId)
	result.Destination = &collector.Destination{URL: result.URL, Identity: result.URL, DatabaseID: result.Descriptor.DataEpoch, DatasetID: result.Descriptor.DatasetId, Local: settings.ServerURL == ""}
	if result.Local != nil {
		result.Destination.URL = "http://local"
		result.Destination.Client = result.Local.IngestionClient()
	}
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
	capabilities, err := collector.NegotiateCapabilities(ctx, s.Destination, s.token)
	if err != nil {
		return err
	}
	if capabilities.DatabaseID != s.Descriptor.DataEpoch || capabilities.DatasetID != s.Descriptor.DatasetId {
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
