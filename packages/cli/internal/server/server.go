// Package server serves the embedded web dashboard and canonical analytics API.
package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"time"

	"crypto/rand"
	"encoding/hex"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestion"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/networkprefs"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

const DefaultPort = networkprefs.DefaultPort
const logIndent = "  "
const queryTimeout = 30 * time.Second

//go:embed static
var assets embed.FS

type Options struct {
	DBPath     string
	Defaults   viewer.Selection
	InstanceID string
}

type syncState struct {
	Generation    int64
	InputRevision int64
	Pending       int64
	InstanceID    string
	DataEpoch     string
	DataReadiness string
	Running       bool              `json:"running"`
	Phase         string            `json:"phase"`
	Harnesses     map[string]string `json:"harnesses"`
	Error         string            `json:"error"`
	Revision      uint64            `json:"revision"`
}

type app struct {
	data           *datastore.Store
	allowIngestion bool
	policy         serverfeatures.Policy
	accounts       *accounts.Service
	publicURL      string
	progress       *collectorprogress.Registry
	core           *ingestion.Core
	options        Options
	ctx            context.Context
	log            io.Writer
}

func newApp(ctx context.Context, options Options, log io.Writer) *app {
	if options.InstanceID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			panic(err)
		}
		options.InstanceID = hex.EncodeToString(id[:])
	}
	policy, _ := serverfeatures.New(serverfeatures.Personal, false)
	return &app{options: options, ctx: ctx, log: log, policy: policy}
}
func (a *app) status() syncState {
	state := syncState{InstanceID: a.options.InstanceID, DataReadiness: "unavailable", Phase: "ready", Harnesses: map[string]string{}}
	if a.data != nil {
		status, err := analytics.Status(a.ctx, a.data)
		if err != nil {
			state.Error = "Server storage unavailable"
			return state
		}
		metadata := status.Metadata
		state.DataReadiness = "ready"
		state.DataEpoch = metadata.DatabaseID
		state.Revision = uint64(metadata.Revision)
		state.Generation = metadata.Generation
		state.InputRevision = metadata.InputRevision
		state.Pending = status.Pending
		return state
	}
	status, err := analytics.LegacyStatus(a.ctx, a.options.DBPath)
	if err != nil {
		state.Error = "Server storage unavailable"
		return state
	}
	metadata := status.Metadata
	state.DataReadiness = "ready"
	state.DataEpoch = metadata.DatabaseID
	state.Revision = uint64(metadata.Revision)
	return state
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func apiError(w http.ResponseWriter, status int, code serverapi.ErrorCode, message string) {
	writeJSON(w, status, serverapi.ErrorResponse{Code: code, Message: message})
}

func apiMethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	apiError(w, http.StatusMethodNotAllowed, serverapi.ErrorCodeMethodNotAllowed, "Method not allowed.")
}

func apiNotFound(w http.ResponseWriter) {
	apiError(w, http.StatusNotFound, serverapi.ErrorCodeNotFound, "Unknown API route.")
}

func isDashboardRoute(path string) bool {
	switch path {
	case "/tokens", "/models", "/providers", "/harnesses", "/sessions", "/context", "/repo":
		return true
	default:
		return false
	}
}

func (a *app) legacyHandler() http.Handler {
	mux := http.NewServeMux()
	if a.data != nil && a.allowIngestion {
		mux.Handle(datastore.IngestionPrefix, a.data.Handler())
		mux.Handle(datastore.LegacyIngestionPrefix, a.data.Handler())
		mux.Handle(datastore.ProcessingPrefix, a.data.Handler())
	}
	mux.HandleFunc("/api/v1/instance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			apiMethodNotAllowed(w, http.MethodGet)
			return
		}
		state := a.status()
		capabilities := []serverapi.Capability{serverapi.Usage, serverapi.Facets}
		if a.data == nil || a.allowIngestion {
			capabilities = append(capabilities, serverapi.Capability("ingestion"))
		}
		writeJSON(w, http.StatusOK, serverapi.InstanceResponse{
			InstanceId: state.InstanceID, DataEpoch: state.DataEpoch, DataReadiness: serverapi.InstanceResponseDataReadiness(state.DataReadiness),
			ApiVersion:    serverapi.V1,
			ServerVersion: version.Version,
			Hostname:      a.dataHostname(r.Context()),
			Timezone:      reportingTimezone(time.Local, time.Now()),
			Capabilities:  capabilities,
			Defaults:      apiSelection(a.options.Defaults),
		})
	})
	mux.HandleFunc("/api/v1/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			apiMethodNotAllowed(w, "GET")
			return
		}
		writeJSON(w, http.StatusOK, apiSyncState(a.status()))
	})
	if a.core != nil {
		mux.HandleFunc("GET /api/v1/ingestion/capabilities", a.core.HandleCapabilities)
		mux.HandleFunc("POST /api/v1/ingestion/batches", func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
			defer cancel()
			a.core.HandleBatches(w, r.WithContext(ctx))
		})
	}
	mux.HandleFunc("/api/v1/usage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			apiMethodNotAllowed(w, http.MethodGet)
			return
		}
		q, err := parseQuery(r.URL.Query())
		if err != nil {
			apiError(w, http.StatusBadRequest, serverapi.ErrorCodeInvalidRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
		defer cancel()
		var data dashboard
		if a.data != nil {
			data, err = loadDataDashboard(ctx, a.data, q, time.Now())
		} else {
			data, err = loadDashboard(ctx, a.options.DBPath, q, time.Now())
		}
		if err != nil {
			a.queryError(w, err)
			return
		}
		response := apiDashboard(data)
		state := a.status()
		response.InstanceId = state.InstanceID
		response.DataEpoch = data.DatabaseID
		writeJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("/api/v1/usage/facets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			apiMethodNotAllowed(w, http.MethodGet)
			return
		}
		q, err := parseQuery(r.URL.Query())
		if err != nil {
			apiError(w, http.StatusBadRequest, serverapi.ErrorCodeInvalidRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
		defer cancel()
		if a.data != nil {
			values, err := loadDataFacets(ctx, a.data, q, r.URL.Query().Get("search"), time.Now())
			if err != nil {
				a.queryError(w, err)
				return
			}
			values.InstanceId = a.options.InstanceID
			writeJSON(w, 200, values)
			return
		}
		facets, err := analytics.LoadLegacyFacets(ctx, a.options.DBPath, q, r.URL.Query().Get("search"), time.Now())
		if err != nil {
			a.queryError(w, err)
			return
		}
		values := apiFacets(facets)
		values.InstanceId = a.options.InstanceID
		writeJSON(w, http.StatusOK, values)
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) { apiNotFound(w) })
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { apiNotFound(w) })
	static, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(static))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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

func (a *app) dataHostname(ctx context.Context) string {
	if a.data != nil {
		return "unknown"
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	status, err := analytics.LegacyStatus(ctx, a.options.DBPath)
	if err != nil {
		return "unknown"
	}
	return status.Hostname
}

func (a *app) queryError(w http.ResponseWriter, err error) {
	_, _ = fmt.Fprintf(a.log, "%sdashboard query failed: %v\n", logIndent, err)
	apiError(w, http.StatusServiceUnavailable, serverapi.ErrorCodeUnavailable, "Cannot read saved usage. Check server storage and reload.")
}

func NewHandler(ctx context.Context, path string, core *ingestion.Core, log io.Writer, bindHost string) http.Handler {
	return NewHandlerWithInstance(ctx, path, core, log, bindHost, "")
}
func NewHandlerWithInstance(ctx context.Context, path string, core *ingestion.Core, log io.Writer, bindHost, instance string) http.Handler {
	a := newApp(ctx, Options{DBPath: path, InstanceID: instance, Defaults: viewer.Selection{Period: "month", Bucket: "day"}}, log)
	a.core = core
	return protectHandler(a.handler(), bindHost)
}

// Local compositions expose writes only through their private control socket.
func NewDataHandler(ctx context.Context, store *datastore.Store, log io.Writer, bindHost, instance string, allowIngestion bool) http.Handler {
	policy, _ := serverfeatures.New(serverfeatures.Personal, false)
	return NewDataHandlerWithOptions(ctx, store, log, DataHandlerOptions{Host: bindHost, InstanceID: instance, AllowIngestion: allowIngestion, Policy: policy})
}
func protectHandler(handler http.Handler, bindHost string) http.Handler {
	handler = http.NewCrossOriginProtection().Handler(handler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, err := netip.ParseAddr(bindHost)
		if bindHost == "" || err == nil && ip.IsLoopback() {
			host, _, err := net.SplitHostPort(r.Host)
			if err != nil {
				host = r.Host
			}
			address, ipErr := netip.ParseAddr(host)
			if host != "localhost" && (ipErr != nil || !address.IsLoopback()) {
				apiError(w, 403, serverapi.ErrorCodeInvalidRequest, "Invalid dashboard host.")
				return
			}
		}
		handler.ServeHTTP(w, r)
	})
}
