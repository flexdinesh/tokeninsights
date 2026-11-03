// Package application owns viewer operations independently of presentation and transport.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/core/version"
)

var (
	ErrUnavailable     = errors.New("server analytics unavailable")
	ErrForbidden       = errors.New("query permission or capability unavailable")
	ErrSnapshotChanged = errors.New("server analytics changed during pagination; retry")
)

type InvalidRequest struct{ Message string }

func (e *InvalidRequest) Error() string { return e.Message }

type Instance struct {
	InstanceID, DatabaseID, DatasetID string
	Readiness                         string
	Kind                              serverfeatures.Kind
	Capabilities                      serverfeatures.Capabilities
	Permissions                       serverfeatures.Permissions
	ServerVersion, Hostname, Timezone string
	Defaults                          analytics.Selection
}

type Usage struct {
	analytics.Dashboard
	InstanceID string
}

type FacetRequest struct {
	Query  analytics.Query
	Search string
}

type Facets struct {
	analytics.Facets
	InstanceID string
}

type ProcessUsage struct {
	CPUPercent          *float64
	ResidentMemoryBytes *int64
}

// Publication identifies visible data and the accepted processing frontier.
type Publication struct {
	DatabaseID, DatasetID                                 string
	Revision, Generation, TargetGeneration, InputRevision int64
}

type Status struct {
	Metadata                         Publication
	Pending, Failed, FailedRetryAtMs int64
	InstanceID, Readiness            string
	ProcessUsage                     *ProcessUsage
}

// Reader observes one authorized dataset. Each operation owns its consistent
// snapshot and honors cancellation. AllUsage rejects limits rather than
// truncating. Filters never select the authorized dataset or request capture.
type Reader interface {
	Instance(context.Context) (Instance, error)
	Usage(context.Context, analytics.Query) (Usage, error)
	AllUsage(context.Context, analytics.Query, int) (Usage, error)
	Facets(context.Context, FacetRequest) (Facets, error)
	Status(context.Context) (Status, error)
}

// Queries borrows a dataset-scoped repository; its caller owns its lifetime.
type Queries struct {
	source             analytics.Repository
	policy             serverfeatures.Policy
	instance, hostname string
	defaults           analytics.Selection
	permissions        serverfeatures.Permissions
}

func NewQueries(source analytics.Repository, policy serverfeatures.Policy, instance, hostname string) *Queries {
	return &Queries{source: source, policy: policy, instance: instance, hostname: hostname,
		defaults: analytics.Selection{Period: "month", Bucket: "day"}, permissions: serverfeatures.Permissions{serverfeatures.Read}}
}

// WithPresentation returns a scoped copy for an authenticated HTTP composition.
// Repository scoping must already have occurred at the authentication boundary.
func (q *Queries) WithPresentation(defaults analytics.Selection, permissions serverfeatures.Permissions) *Queries {
	copy := *q
	copy.defaults, copy.permissions = cloneSelection(defaults), append(serverfeatures.Permissions{}, permissions...)
	return &copy
}

func describe(policy serverfeatures.Policy, permissions serverfeatures.Permissions, status analytics.ProcessingStatus, instance, hostname string, defaults analytics.Selection) Instance {
	return Instance{InstanceID: instance, DatabaseID: status.Metadata.DatabaseID, DatasetID: status.Metadata.DatasetID,
		Readiness: "ready", Kind: policy.Kind, Capabilities: append(serverfeatures.Capabilities{}, policy.Capabilities...), Permissions: append(serverfeatures.Permissions{}, permissions...),
		ServerVersion: version.Version, Hostname: hostname, Timezone: analytics.ReportingTimezone(time.Local, time.Now()), Defaults: cloneSelection(defaults)}
}

func (q *Queries) check(ctx context.Context, capability serverfeatures.Capability) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := q.policy.Validate(); err != nil {
		return ErrUnavailable
	}
	if !q.permissions.Has(serverfeatures.Read) || capability != "" && !q.policy.Capabilities.Has(capability) {
		return ErrForbidden
	}
	return nil
}

func (q *Queries) Instance(ctx context.Context) (Instance, error) {
	if err := ctx.Err(); err != nil {
		return Instance{}, err
	}
	if err := q.policy.Validate(); err != nil {
		return Instance{}, ErrUnavailable
	}
	status, err := q.source.Status(ctx)
	if err != nil {
		return Instance{}, queryFailure(err)
	}
	hostname := status.Hostname
	if q.hostname != "" && q.policy.Kind == serverfeatures.Personal {
		hostname = q.hostname
	}
	return describe(q.policy, q.permissions, status, q.instance, hostname, q.defaults), nil
}

const queryTimeout = 30 * time.Second

func (q *Queries) Usage(ctx context.Context, request analytics.Query) (Usage, error) {
	return q.usage(ctx, request, 0)
}

func (q *Queries) AllUsage(ctx context.Context, request analytics.Query, maxRows int) (Usage, error) {
	if maxRows < 1 {
		return Usage{}, &InvalidRequest{Message: "invalid analytics row limit"}
	}
	return q.usage(ctx, request, maxRows)
}

func (q *Queries) usage(ctx context.Context, request analytics.Query, maxRows int) (Usage, error) {
	if err := q.check(ctx, serverfeatures.Usage); err != nil {
		return Usage{}, err
	}
	request, err := NormalizeQuery(request)
	if err != nil {
		return Usage{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	var data analytics.Dashboard
	if maxRows > 0 {
		data, err = q.source.AllDashboard(ctx, request, time.Now(), maxRows)
	} else {
		data, err = q.source.Dashboard(ctx, request, time.Now())
	}
	if err != nil {
		return Usage{}, queryFailure(err)
	}
	return Usage{Dashboard: data, InstanceID: q.instance}, nil
}

func (q *Queries) Facets(ctx context.Context, request FacetRequest) (Facets, error) {
	if err := q.check(ctx, serverfeatures.Facets); err != nil {
		return Facets{}, err
	}
	query, err := NormalizeQuery(request.Query)
	if err != nil {
		return Facets{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	data, err := q.source.Facets(ctx, query, request.Search, time.Now())
	if err != nil {
		return Facets{}, queryFailure(err)
	}
	return Facets{Facets: data, InstanceID: q.instance}, nil
}

func (q *Queries) Status(ctx context.Context) (Status, error) {
	if err := q.check(ctx, ""); err != nil {
		return Status{}, err
	}
	status, err := q.source.Status(ctx)
	if err != nil {
		return Status{}, queryFailure(err)
	}
	m := status.Metadata
	return Status{Metadata: Publication{DatabaseID: m.DatabaseID, DatasetID: m.DatasetID, Revision: m.Revision, Generation: m.Generation, TargetGeneration: m.TargetGeneration, InputRevision: m.InputRevision}, Pending: status.Pending, Failed: status.Failed, FailedRetryAtMs: status.FailedRetryAtMs, InstanceID: q.instance, Readiness: "ready"}, nil
}

func cloneSelection(selection analytics.Selection) analytics.Selection {
	selection.Providers = append([]string{}, selection.Providers...)
	selection.Models = append([]string{}, selection.Models...)
	selection.Harnesses = append([]string{}, selection.Harnesses...)
	selection.Sessions = append([]string{}, selection.Sessions...)
	return selection
}

func queryFailure(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}
