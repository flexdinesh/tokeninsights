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
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestion"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

const DefaultPort = 8765
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
	core    *ingestion.Core
	options Options
	ctx     context.Context
	log     io.Writer
}

func newApp(ctx context.Context, options Options, log io.Writer) *app {
	if options.InstanceID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			panic(err)
		}
		options.InstanceID = hex.EncodeToString(id[:])
	}
	return &app{options: options, ctx: ctx, log: log}
}
func (a *app) status() syncState {
	state := syncState{InstanceID: a.options.InstanceID, DataReadiness: "unavailable", Phase: "ready", Harnesses: map[string]string{}}
	store, err := serverstore.Open(a.options.DBPath)
	if err != nil {
		state.Error = "Server storage unavailable"
		return state
	}
	defer func() { _ = store.Close() }()
	metadata, err := store.Metadata(a.ctx)
	if err != nil {
		state.Error = "Server storage unavailable"
		return state
	}
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

func (a *app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/instance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			apiMethodNotAllowed(w, http.MethodGet)
			return
		}
		state := a.status()
		writeJSON(w, http.StatusOK, serverapi.InstanceResponse{
			InstanceId: state.InstanceID, DataEpoch: state.DataEpoch, DataReadiness: serverapi.InstanceResponseDataReadiness(state.DataReadiness),
			ApiVersion:    serverapi.V1,
			ServerVersion: version.Version,
			Hostname:      a.dataHostname(r.Context()),
			Timezone:      reportingTimezone(time.Local, time.Now()),
			Capabilities:  []serverapi.Capability{serverapi.Usage, serverapi.Facets, serverapi.Capability("ingestion")},
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
		data, err := loadDashboard(ctx, a.options.DBPath, q, time.Now())
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
		database, err := serverstore.Open(a.options.DBPath)
		if err != nil {
			a.queryError(w, err)
			return
		}
		defer func() { _ = database.Close() }()
		tx, err := database.BeginRead(ctx)
		if err != nil {
			a.queryError(w, err)
			return
		}
		defer func() { _ = tx.Rollback() }()
		f := q.Selection.Filter(time.Now())
		if q.Tab == "repo" {
			f.RepositoryKeys, f.DirectoryKeys = q.RepositoryKeys, q.DirectoryKeys
		}
		values := serverapi.UsageFacetsResponse{Providers: []string{}, Models: []string{}, Harnesses: []serverapi.Harness{}, Sessions: []string{}, Repositories: []serverapi.LocationOption{}, Directories: []serverapi.LocationOption{}}
		values.Providers, err = db.AvailableProviders(ctx, tx, f)
		if err != nil {
			a.queryError(w, err)
			return
		}
		values.Models, err = db.AvailableModels(ctx, tx, f)
		if err != nil {
			a.queryError(w, err)
			return
		}
		harnesses, err := db.AvailableHarnesses(ctx, tx, f)
		if err != nil {
			a.queryError(w, err)
			return
		}
		values.Harnesses = apiHarnesses(harnesses)
		values.Sessions, err = db.AvailableSessions(ctx, tx, f, r.URL.Query().Get("search"), sessionOptionLimit)
		if err != nil {
			a.queryError(w, err)
			return
		}
		if q.Tab == "repo" {
			locations, err := db.AvailableLocations(ctx, tx, f)
			if err != nil {
				a.queryError(w, err)
				return
			}
			values.Repositories = apiLocationOptions(locations.Repositories)
			values.Directories = apiLocationOptions(locations.Directories)
		}
		snapshotStatus, err := serverstore.ReadMetadata(ctx, tx)
		if err != nil {
			a.queryError(w, err)
			return
		}
		values.Revision = snapshotStatus.Revision
		values.InstanceId = a.options.InstanceID
		values.DataEpoch = snapshotStatus.DatabaseID
		if err = tx.Commit(); err != nil {
			a.queryError(w, err)
			return
		}
		values.Providers = nonNilStrings(values.Providers)
		values.Models = nonNilStrings(values.Models)
		values.Sessions = nonNilStrings(values.Sessions)
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
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	store, err := serverstore.Open(a.options.DBPath)
	if err != nil {
		return "unknown"
	}
	defer func() { _ = store.Close() }()
	var count int
	var hostname string
	if err := store.SQL().QueryRowContext(ctx, "SELECT COUNT(DISTINCT hostname), COALESCE(MIN(hostname),'') FROM ingestion_producers WHERE hostname <> ''").Scan(&count, &hostname); err != nil {
		return "unknown"
	}
	if count > 1 {
		return "multiple machines"
	}
	if count == 1 {
		return hostname
	}
	return "unknown"
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
	handler := http.NewCrossOriginProtection().Handler(a.handler())
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
