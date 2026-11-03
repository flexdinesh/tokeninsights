package server

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/core/accounts"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/ingestionhttp"
	"github.com/flexdinesh/tokeninsights/packages/core/processusage"
	"github.com/flexdinesh/tokeninsights/packages/core/publication"
	api "github.com/flexdinesh/tokeninsights/packages/core/server/api"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

type DataHandlerOptions struct {
	Host, InstanceID, Hostname string
	AllowIngestion             bool
	Policy                     serverfeatures.Policy
	Accounts                   *accounts.Service
	PublicURL                  string
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
	if err := options.Policy.Validate(); err != nil || store == nil || string(options.Policy.Kind) != store.Kind() || options.Policy.Kind == serverfeatures.Hosted && options.Accounts == nil || options.Policy.Capabilities.Has(serverfeatures.CollectorProgress) && options.Progress == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiError(w, 503, api.ErrorCodeUnavailable, "Invalid server composition.")
		})
	}
	if options.Policy.Kind == serverfeatures.Hosted {
		options.Hostname = ""
	}
	a := newApp(ctx, Options{InstanceID: options.InstanceID, Hostname: options.Hostname, Defaults: analytics.Selection{Period: "month", Bucket: "day"}}, log)
	a.source = store
	if options.Policy.Kind == serverfeatures.Personal {
		a.data = store.Receiver(evidence.PersonalDatasetID)
		a.queries = store.Queries(evidence.PersonalDatasetID)
		a.processUsage = options.ProcessUsage
	}
	a.allowIngestion = options.AllowIngestion && options.Policy.Capabilities.Has(serverfeatures.RawIngestion)
	a.policy, a.accounts, a.publicURL, a.progress = options.Policy, options.Accounts, options.PublicURL, options.Progress
	handler := a.handler()
	if a.policy.Kind == serverfeatures.Hosted {
		protection := http.NewCrossOriginProtection()
		if options.PublicURL != "" {
			if err := protection.AddTrustedOrigin(options.PublicURL); err != nil {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					apiError(w, 503, api.ErrorCodeUnavailable, "Invalid server origin.")
				})
			}
		}
		return protection.Handler(handler)
	}
	return protectHandler(handler, options.Host)
}

func (a *app) scoped(r *http.Request) (*app, accounts.Principal, error) {
	if a.policy.Kind != serverfeatures.Hosted {
		return a, accounts.Principal{UserID: evidence.PersonalDatasetID, DatasetID: evidence.PersonalDatasetID, CredentialKind: accounts.LocalCredential, Permissions: []string{accounts.Read, accounts.Ingest}}, nil
	}
	p, err := authenticateRequest(r.Context(), a.accounts, r)
	if err != nil {
		return nil, p, err
	}
	copy := *a
	copy.data = a.source.Receiver(p.DatasetID)
	copy.queries = a.source.Queries(p.DatasetID)
	copy.ctx = r.Context()
	return &copy, p, nil
}

// Raw acceptance uses protocol errors even when authentication/admission rejects
// before body decoding. Query routes retain their versioned read-API envelope.
func accessError(w http.ResponseWriter, r *http.Request, status int, code api.ErrorCode, message, rawCode string) {
	if strings.HasPrefix(r.URL.Path, ingestionhttp.IngestionPrefix) {
		writeJSON(w, status, publication.ErrorResponse{Stage: "admission", Code: rawCode})
		return
	}
	apiError(w, status, code, message)
}

func (a *app) access(capability serverfeatures.Capability, permission string, handler func(*app, accounts.Principal, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if capability != "" && !a.policy.Capabilities.Has(capability) {
			apiNotFound(w)
			return
		}
		scoped, p, err := a.scoped(r)
		if err != nil {
			if errors.Is(err, accounts.ErrUnauthenticated) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				accessError(w, r, 401, api.ErrorCodeInvalidRequest, "Authentication required.", "unauthorized")
			} else {
				accessError(w, r, 503, api.ErrorCodeUnavailable, "Authentication unavailable.", "unavailable")
			}
			return
		}
		if permission != "" && !p.HasPermission(permission) {
			accessError(w, r, 403, api.ErrorCodeInvalidRequest, "Permission required.", "forbidden")
			return
		}
		handler(scoped, p, w, r)
	}
}

func (a *app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/auth", a.authInfo)
	mux.HandleFunc("GET /api/v2/instance", a.access("", "", func(scoped *app, p accounts.Principal, w http.ResponseWriter, r *http.Request) {
		scoped.instanceV2(w, r, p)
	}))
	mux.HandleFunc("GET /api/v2/status", a.access("", accounts.Read, func(scoped *app, _ accounts.Principal, w http.ResponseWriter, r *http.Request) { scoped.statusV2(w, r) }))
	mux.HandleFunc("GET /api/v2/usage", a.access(serverfeatures.Usage, accounts.Read, func(scoped *app, _ accounts.Principal, w http.ResponseWriter, r *http.Request) { scoped.usageV2(w, r) }))
	mux.HandleFunc("GET /api/v2/usage/facets", a.access(serverfeatures.Facets, accounts.Read, func(scoped *app, _ accounts.Principal, w http.ResponseWriter, r *http.Request) { scoped.facetsV2(w, r) }))
	if a.allowIngestion {
		mux.Handle(ingestionhttp.IngestionPrefix, a.access(serverfeatures.RawIngestion, "", func(scoped *app, p accounts.Principal, w http.ResponseWriter, r *http.Request) {
			permission := accounts.Ingest
			if r.Method == http.MethodGet && r.URL.Path != ingestionhttp.IngestionPrefix+"capabilities" {
				permission = accounts.Read
			}
			if !p.HasPermission(permission) {
				accessError(w, r, 403, api.ErrorCodeInvalidRequest, "Permission required.", "forbidden")
				return
			}
			if a.accounts != nil && r.Method == http.MethodPost {
				release, err := a.accounts.Admit(p)
				if err != nil {
					var limit *accounts.AdmissionError
					code := "unavailable"
					if errors.As(err, &limit) {
						code = limit.Code
						w.Header().Set("Retry-After", strconv.Itoa(limit.RetryAfter))
					}
					accessError(w, r, 429, api.ErrorCodeUnavailable, "Ingestion temporarily limited.", code)
					return
				}
				defer release()
			}
			ingestionhttp.Handler(scoped.data, ingestionhttp.IngestionPrefix, evidence.ProtocolVersion).ServeHTTP(w, r)
		}))
	}
	if a.policy.Kind == serverfeatures.Personal {
		if a.progress != nil && a.policy.Capabilities.Has(serverfeatures.CollectorProgress) {
			mux.Handle("/api/v2/collector-progress", a.progress.ReadHandler())
		}
	} else {
		mux.HandleFunc("POST /api/v2/auth/session", a.login)
		mux.HandleFunc("DELETE /api/v2/auth/session", a.logout)
		if a.policy.SignupEnabled {
			mux.HandleFunc("POST /api/v2/auth/signup", a.signup)
		}
		mux.HandleFunc("GET /api/v2/account", a.accountAccess(a.currentUser))
		mux.HandleFunc("PUT /api/v2/account/password", a.accountAccess(a.changePassword))
		mux.HandleFunc("GET /api/v2/account/keys", a.accountAccess(a.listKeys))
		mux.HandleFunc("POST /api/v2/account/keys", a.accountAccess(a.createKey))
		mux.HandleFunc("DELETE /api/v2/account/keys/{keyId}", a.accountAccess(a.revokeKey))
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

func (a *app) instanceV2(w http.ResponseWriter, r *http.Request, p accounts.Principal) {
	permissions := make(serverfeatures.Permissions, 0, len(p.Permissions))
	for _, permission := range p.Permissions {
		permissions = append(permissions, serverfeatures.Permission(permission))
	}
	instance, err := a.queryService().WithPresentation(a.options.Defaults, permissions).Instance(r.Context())
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
	if a.policy.Kind == serverfeatures.Personal && a.processUsage != nil {
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
