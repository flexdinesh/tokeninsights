package server

import (
	"context"
	"io"
	"io/fs"
	"net/http"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/processusage"
	api "github.com/flexdinesh/tokeninsights/packages/core/server/api"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

type DataHandlerOptions struct {
	Host, InstanceID, Hostname string
	Policy                     serverfeatures.Policy
	Progress                   *collectorprogress.Registry
	ProcessUsage               ProcessUsage
}

// ProcessUsage samples the command-owned process; missing measurements remain unknown.
type ProcessUsage interface {
	Snapshot(context.Context) processusage.Sample
}

func NewPersonalDataHandler(ctx context.Context, store DataSource, log io.Writer, host, instance string, progress *collectorprogress.Registry) http.Handler {
	policy, _ := serverfeatures.New(serverfeatures.Personal, progress != nil)
	return NewDataHandlerWithOptions(ctx, store, log, DataHandlerOptions{Host: host, InstanceID: instance, Policy: policy, Progress: progress})
}

func NewDataHandlerWithOptions(ctx context.Context, store DataSource, log io.Writer, options DataHandlerOptions) http.Handler {
	if options.Policy.Kind == "" {
		options.Policy, _ = serverfeatures.New(serverfeatures.Personal, options.Progress != nil)
	}
	if log == nil {
		log = io.Discard
	}
	if err := options.Policy.Validate(); err != nil || store == nil || string(options.Policy.Kind) != store.Kind() || options.Policy.Capabilities.Has(serverfeatures.CollectorProgress) && options.Progress == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiError(w, 503, api.ErrorCodeUnavailable, "Invalid server composition.")
		})
	}
	a := newApp(ctx, Options{InstanceID: options.InstanceID, Hostname: options.Hostname, Defaults: analytics.Selection{Period: "month", Bucket: "day"}}, log)
	a.queries = store.Queries(evidence.PersonalDatasetID)
	a.processUsage = options.ProcessUsage
	a.policy, a.progress = options.Policy, options.Progress
	handler := a.handler()
	return protectHandler(handler, options.Host)
}

func (a *app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/instance", a.instanceV2)
	mux.HandleFunc("GET /api/v2/status", a.statusV2)
	if a.policy.Capabilities.Has(serverfeatures.Usage) {
		mux.HandleFunc("GET /api/v2/usage", a.usageV2)
	}
	if a.policy.Capabilities.Has(serverfeatures.Facets) {
		mux.HandleFunc("GET /api/v2/usage/facets", a.facetsV2)
	}
	if a.progress != nil && a.policy.Capabilities.Has(serverfeatures.CollectorProgress) {
		mux.Handle("/api/v2/collector-progress", a.progress.ReadHandler())
	}
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) { apiNotFound(w) })
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { apiNotFound(w) })
	mux.HandleFunc("/control/", http.NotFound)
	static, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(static))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !a.policy.Capabilities.Has(serverfeatures.WebDashboard) {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if isDashboardRoute(r.URL.Path) {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			r.URL.RawPath = ""
		}
		files.ServeHTTP(w, r)
	})
	return mux
}

func (a *app) instanceV2(w http.ResponseWriter, r *http.Request) {
	instance, err := a.queryService().WithPresentation(a.options.Defaults, serverfeatures.Permissions{serverfeatures.Read}).Instance(r.Context())
	if err != nil {
		a.queryError(w, r, err)
		return
	}
	writeJSON(w, 200, api.InstanceResponse(instance))
}

func (a *app) queryService() *application.Queries {
	return application.NewQueries(a.queries, a.policy, a.options.InstanceID, a.options.Hostname)
}

func (a *app) statusV2(w http.ResponseWriter, r *http.Request) {
	state, err := a.queryService().Status(r.Context())
	if err != nil {
		a.queryError(w, r, err)
		return
	}
	response := api.StatusResponse(state)
	if a.processUsage != nil {
		usage := a.processUsage.Snapshot(r.Context())
		response.ProcessUsage = &api.ProcessUsage{CpuPercent: usage.CPUPercent, ResidentMemoryBytes: usage.ResidentMemoryBytes}
	}
	writeJSON(w, 200, response)
}

func (a *app) usageV2(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r.URL.Query())
	if err != nil {
		apiError(w, 400, api.ErrorCodeInvalidRequest, err.Error())
		return
	}
	data, err := a.queryService().Usage(r.Context(), q)
	if err != nil {
		a.queryError(w, r, err)
		return
	}
	v := api.UsageResponse(data)
	writeJSON(w, 200, v)
}

func (a *app) facetsV2(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r.URL.Query())
	if err != nil {
		apiError(w, 400, api.ErrorCodeInvalidRequest, err.Error())
		return
	}
	data, err := a.queryService().Facets(r.Context(), application.FacetRequest{Query: q, Search: r.URL.Query().Get("search")})
	if err != nil {
		a.queryError(w, r, err)
		return
	}
	v := api.FacetsResponse(data)
	writeJSON(w, 200, v)
}
