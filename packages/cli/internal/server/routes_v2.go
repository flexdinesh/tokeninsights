package server

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestionhttp"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

type DataHandlerOptions struct {
	Host, InstanceID, Hostname string
	AllowIngestion             bool
	Policy                     serverfeatures.Policy
	Accounts                   *accounts.Service
	PublicURL                  string
	Progress                   *collectorprogress.Registry
}

const loginBodyMaxBytes = 4 << 10

func NewPersonalDataHandler(ctx context.Context, store *datastore.Store, log io.Writer, host, instance string, progress *collectorprogress.Registry) http.Handler {
	policy, _ := serverfeatures.New(serverfeatures.Personal, progress != nil)
	return NewDataHandlerWithOptions(ctx, store, log, DataHandlerOptions{Host: host, InstanceID: instance, Policy: policy, Progress: progress})
}

func NewDataHandlerWithOptions(ctx context.Context, store *datastore.Store, log io.Writer, options DataHandlerOptions) http.Handler {
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
	a := newApp(ctx, Options{InstanceID: options.InstanceID, Hostname: options.Hostname, Defaults: viewer.Selection{Period: "month", Bucket: "day"}}, log)
	a.data = store
	a.queries = analytics.DuckDB{Store: store}
	a.allowIngestion = options.AllowIngestion && options.Policy.Capabilities.Has(serverfeatures.RawIngestion)
	a.policy, a.accounts, a.publicURL, a.progress = options.Policy, options.Accounts, options.PublicURL, options.Progress
	handler := a.handler()
	if a.policy.Kind == serverfeatures.Hosted {
		protection := http.NewCrossOriginProtection()
		if err := protection.AddTrustedOrigin(options.PublicURL); err != nil {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				apiError(w, 503, api.ErrorCodeUnavailable, "Invalid server origin.")
			})
		}
		return protection.Handler(handler)
	}
	return protectHandler(handler, options.Host)
}

func (a *app) scoped(r *http.Request) (*app, accounts.Principal, error) {
	if a.policy.Kind != serverfeatures.Hosted {
		return a, accounts.Principal{DatasetID: datastore.DatasetID, Permissions: []string{accounts.Read, accounts.Ingest}}, nil
	}
	p, err := a.accounts.AuthenticateRequest(r.Context(), r)
	if err != nil {
		return nil, p, err
	}
	copy := *a
	copy.data = a.data.ForDataset(p.DatasetID)
	copy.queries = analytics.DuckDB{Store: copy.data}
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
	status, err := a.queries.Status(r.Context())
	if err != nil {
		a.queryError(w, err)
		return
	}
	caps := make([]string, 0, len(a.policy.Capabilities))
	for _, capability := range a.policy.Capabilities {
		caps = append(caps, string(capability))
	}
	permissions := make([]api.InstanceResponseV2Permissions, 0, len(p.Permissions))
	for _, permission := range p.Permissions {
		permissions = append(permissions, api.InstanceResponseV2Permissions(permission))
	}
	writeJSON(w, 200, api.InstanceResponseV2{ApiVersion: "v2", ServerKind: api.InstanceResponseV2ServerKind(a.policy.Kind), Capabilities: caps, Permissions: permissions, DataEpoch: status.Metadata.DatabaseID, DatasetId: status.Metadata.DatasetID, InstanceId: a.options.InstanceID, DataReadiness: "ready", ServerVersion: version.Version, Hostname: a.instanceHostname(status.Hostname), Timezone: reportingTimezone(time.Local, time.Now()), Defaults: apiSelection(a.options.Defaults)})
}

func (a *app) statusV2(w http.ResponseWriter, r *http.Request) {
	status, err := a.queries.Status(r.Context())
	if err != nil {
		a.queryError(w, err)
		return
	}
	m := status.Metadata
	writeJSON(w, 200, api.StatusResponseV2{InstanceId: a.options.InstanceID, DataEpoch: m.DatabaseID, DatasetId: m.DatasetID, DataReadiness: "ready", Generation: m.Generation, TargetGeneration: m.TargetGeneration, InputRevision: m.InputRevision, Revision: m.Revision, Pending: status.Pending, Failed: &status.Failed, FailedRetryAtMs: &status.FailedRetryAtMs})
}

func (a *app) usageV2(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r.URL.Query())
	if err != nil {
		apiError(w, 400, api.ErrorCodeInvalidRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()
	data, err := a.queries.Dashboard(ctx, q, time.Now())
	if err != nil {
		a.queryError(w, err)
		return
	}
	v := apiDashboard(data)
	v.InstanceId = a.options.InstanceID
	writeJSON(w, 200, v)
}

func (a *app) facetsV2(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r.URL.Query())
	if err != nil {
		apiError(w, 400, api.ErrorCodeInvalidRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()
	data, err := a.queries.Facets(ctx, q, r.URL.Query().Get("search"), time.Now())
	if err != nil {
		a.queryError(w, err)
		return
	}
	v := apiFacets(data)
	v.InstanceId = a.options.InstanceID
	writeJSON(w, 200, v)
}

func (a *app) sameOrigin(r *http.Request) bool {
	u, err := url.Parse(a.publicURL)
	return err == nil && u != nil && u.Host != "" && r.Header.Get("Origin") == u.Scheme+"://"+u.Host
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	if !a.sameOrigin(r) {
		apiError(w, 403, api.ErrorCodeInvalidRequest, "Invalid origin.")
		return
	}
	if err := a.accounts.AdmitLogin(r.RemoteAddr); err != nil {
		w.Header().Set("Retry-After", "60")
		apiError(w, 429, api.ErrorCodeUnavailable, "Login temporarily limited.")
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, loginBodyMaxBytes))
	if err != nil || r.Header.Get("Content-Type") != "application/json" || evidence.StrictDecode(body, &input) != nil || input.Token == "" {
		apiError(w, 400, api.ErrorCodeInvalidRequest, "Invalid login request.")
		return
	}
	secret, expires, err := a.accounts.CreateSession(r.Context(), input.Token)
	if err != nil {
		if errors.Is(err, accounts.ErrUnauthenticated) {
			apiError(w, 401, api.ErrorCodeInvalidRequest, "Token cannot sign in.")
		} else {
			apiError(w, 503, api.ErrorCodeUnavailable, "Login unavailable.")
		}
		return
	}
	http.SetCookie(w, accounts.SessionCookie(secret, expires))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if !a.sameOrigin(r) {
		apiError(w, 403, api.ErrorCodeInvalidRequest, "Invalid origin.")
		return
	}
	if cookie, err := r.Cookie(accounts.SessionCookieName); err == nil {
		if err := a.accounts.Logout(r.Context(), cookie.Value); err != nil {
			apiError(w, 503, api.ErrorCodeUnavailable, "Logout unavailable.")
			return
		}
	}
	http.SetCookie(w, accounts.ClearSessionCookie())
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
