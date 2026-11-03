package application_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/datastore"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

func queries(t *testing.T, policy serverfeatures.Policy) *application.Queries {
	t.Helper()
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return application.NewQueries(sqlanalytics.Queries{Store: store}, policy, "instance", "local-host")
}

func TestQueryDefaultsAndCancellationAgainstStorage(t *testing.T) {
	policy, err := serverfeatures.NewLocalViewer(false)
	if err != nil {
		t.Fatal(err)
	}
	service := queries(t, policy)
	request := analytics.Query{Selection: analytics.Selection{Period: "all"}}
	usage, err := service.Usage(t.Context(), request)
	if err != nil || usage.InstanceID != "instance" || usage.Page != 1 || usage.PageSize != 50 || usage.Quality != "confirmed" || usage.DatasetID != "default" || usage.DatabaseID == "" || usage.RowCount != 0 {
		t.Fatalf("default query: %+v, %v", usage, err)
	}
	instance, err := service.Instance(t.Context())
	if err != nil || instance.DatabaseID != usage.DatabaseID || instance.DatasetID != usage.DatasetID || instance.Hostname != "local-host" {
		t.Fatalf("identity: %+v, %v", instance, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for name, read := range map[string]func() error{
		"instance": func() error { _, err := service.Instance(ctx); return err },
		"usage":    func() error { _, err := service.Usage(ctx, request); return err },
		"all":      func() error { _, err := service.AllUsage(ctx, request, 1); return err },
		"facets":   func() error { _, err := service.Facets(ctx, application.FacetRequest{Query: request}); return err },
		"status":   func() error { _, err := service.Status(ctx); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := read(); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestSemanticInvalidRequests(t *testing.T) {
	policy, err := serverfeatures.NewLocalViewer(false)
	if err != nil {
		t.Fatal(err)
	}
	service := queries(t, policy)
	for _, request := range []analytics.Query{
		{Tab: "invalid"}, {Page: -1}, {PageSize: 201}, {Quality: "combined"},
		{Selection: analytics.Selection{From: "2026-02-02", To: "2026-02-01"}},
		{Tab: "context", Sort: "total"}, {Tab: "models", RepositoryKeys: []string{"repo"}},
	} {
		result, err := service.Usage(t.Context(), request)
		var invalid *application.InvalidRequest
		if !errors.As(err, &invalid) || len(result.Rows) != 0 {
			t.Fatalf("request %+v: %+v, %v", request, result, err)
		}
	}
	if _, err := service.AllUsage(t.Context(), analytics.Query{}, 0); err == nil {
		t.Fatal("zero row limit accepted")
	}
}

func TestCapabilitiesAndPermissionsRemainDistinct(t *testing.T) {
	policy, err := serverfeatures.New(serverfeatures.Personal, false, serverfeatures.Usage, serverfeatures.Facets, serverfeatures.WebDashboard, serverfeatures.TerminalDashboard)
	if err != nil {
		t.Fatal(err)
	}
	service := queries(t, policy)
	if _, err := service.Usage(t.Context(), analytics.Query{}); !errors.Is(err, application.ErrForbidden) {
		t.Fatal("disabled usage accepted", err)
	}
	if _, err := service.Facets(t.Context(), application.FacetRequest{}); !errors.Is(err, application.ErrForbidden) {
		t.Fatal("disabled facets accepted", err)
	}
	// Status is independent of the optional usage capability, but requires read.
	if _, err := service.Status(t.Context()); err != nil {
		t.Fatal(err)
	}
	service = service.WithPresentation(analytics.Selection{Period: "month", Bucket: "day"}, nil)
	if _, err := service.Status(t.Context()); !errors.Is(err, application.ErrForbidden) {
		t.Fatal("unauthorized status accepted", err)
	}
	if instance, err := service.Instance(t.Context()); err != nil || len(instance.Permissions) != 0 {
		t.Fatal("descriptor requires read", instance, err)
	}
}
