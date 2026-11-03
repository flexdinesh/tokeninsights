package main

// Backend startup and query I/O stay here. query_adapter.go maps application
// values; the native view uses injected presentation values, never HTTP types.
import (
	"context"
	"errors"
	"fmt"

	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/desktop-mygo/internal/link"
)

type localSource struct{ session *localclient.Session }

func connectLocal(ctx context.Context, target string, capture bool) (dataSource, error) {
	if target != "" {
		// Verify link identity before attaching an existing owner.
		parsed, err := link.Dashboard(target)
		if err != nil {
			return nil, err
		}
		if !parsed.Loopback {
			return nil, &externalTarget{URL: target}
		}
		client, err := queryclient.New(parsed.Origin, nil)
		if err != nil {
			return nil, err
		}
		instance, err := client.Instance(ctx)
		if err == nil {
			if instance.Kind != serverfeatures.Personal {
				return nil, &externalTarget{URL: target}
			}
		} else {
			var status *queryclient.StatusError
			if errors.As(err, &status) {
				return nil, &externalTarget{URL: target}
			}
			return nil, fmt.Errorf("cannot inspect local dashboard: %w", err)
		}
		// A URL handoff inherits its launcher's capture policy; attach only.
		session, err := localclient.ConnectEndpoint(ctx, parsed.Origin, false, false)
		if errors.Is(err, localclient.ErrEndpointMissing) {
			return nil, &externalTarget{URL: target}
		}
		if err != nil {
			return nil, err
		}
		return &localSource{session: session}, nil
	}
	path, err := config.Path("")
	if err != nil {
		return nil, err
	}
	settings, err := config.ResolveLocal(path, true, config.Overrides{})
	if err != nil {
		return nil, err
	}
	options := localclient.Options{
		CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath,
		AppDBPath: settings.ApplicationPath(), Host: settings.Host, Port: settings.Port, Capture: capture,
	}
	session, err := localclient.Connect(ctx, options)
	if err != nil {
		return nil, err
	}
	return &localSource{session: session}, nil
}

func (s *localSource) Close() error { return s.session.Close() }

func (s *localSource) Retry(ctx context.Context) error { return s.session.Capture(ctx, false, "") }

func (s *localSource) Load(ctx context.Context, f filters) (snapshot, error) {
	select {
	case <-s.session.Done():
		return snapshot{}, errDisconnected
	default:
	}
	query := nativeQuery(f)
	data, err := s.session.Query.AllUsage(ctx, query)
	if err != nil {
		return snapshot{}, err
	}
	result := usageSnapshot(data)
	// Facet/progress failures retain readable saved usage and active filters.
	facets, err := s.session.Query.Facets(ctx, application.FacetRequest{Query: query})
	if err == nil {
		result.Providers, result.Models, result.Harnesses = facets.Providers, facets.Models, facets.Harnesses
	}
	status, err := s.session.Query.Status(ctx)
	if err != nil {
		result.Processing = "Processing status unavailable"
	} else {
		result.Processing = processingText(status)
	}
	progress, err := s.session.Progress(ctx)
	if err != nil {
		result.Collection = "Collection status unavailable"
	} else {
		result.Collection, result.CollectionFailed = collectionText(progress)
	}
	return result, nil
}
