// Package server serves the embedded web dashboard and canonical analytics API.
package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"time"

	application "github.com/flexdinesh/tokeninsights/packages/cli/internal/app"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

const DefaultPort = 8765
const logIndent = "  "
const queryTimeout = 30 * time.Second

//go:embed static
var assets embed.FS

type Options struct {
	DBPath   string
	Defaults viewer.Selection
}

type syncState struct {
	InstanceID       string
	DataEpoch        string
	DataReadiness    string
	PendingRefresh   bool
	CheckRequestedAt int64
	Running          bool              `json:"running"`
	Phase            string            `json:"phase"`
	Harnesses        map[string]string `json:"harnesses"`
	Error            string            `json:"error"`
	Progress         *db.SyncStatus    `json:"-"`
	Revision         uint64            `json:"revision"`
}

type app struct {
	controller *application.Controller
	options    Options
	ctx        context.Context
	log        io.Writer
}

func newApp(ctx context.Context, options Options, log io.Writer) *app {
	return &app{options: options, ctx: ctx, log: log}
}
func (a *app) status() syncState {
	if a.controller != nil {
		s := a.controller.Status(a.ctx)
		var progress *db.SyncStatus
		if s.Progress.JobID != 0 {
			progress = &s.Progress
		}
		return syncState{Running: s.Running, Phase: s.Phase, Error: s.Error, Harnesses: s.Progress.Harnesses, Revision: uint64(s.Progress.Revision), Progress: progress, InstanceID: s.InstanceID, DataEpoch: s.DataEpoch, DataReadiness: s.DataReadiness, PendingRefresh: s.PendingRefresh, CheckRequestedAt: s.CheckRequestedAt}
	}
	shared, _ := db.ReadSyncStatus(a.ctx, a.options.DBPath)
	return syncState{Running: shared.Running, Phase: shared.Phase, Error: shared.Error, Harnesses: shared.Harnesses, Revision: uint64(shared.Revision)}
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
			InstanceId: &state.InstanceID, DataEpoch: &state.DataEpoch, DataReadiness: readinessPointer(state.DataReadiness),
			ApiVersion:    serverapi.V1,
			ServerVersion: version.Version,
			Hostname:      a.dataHostname(r.Context()),
			Timezone:      time.Now().Format("MST -07:00"),
			Capabilities:  []serverapi.Capability{serverapi.Usage, serverapi.Facets, serverapi.Sync},
			Defaults:      apiSelection(a.options.Defaults),
		})
	})
	mux.HandleFunc("/api/v1/sync", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, apiSyncState(a.status()))
		case http.MethodPost:
			if a.controller != nil {
				if _, err := a.controller.RequestRefresh(r.Context()); err != nil {
					apiError(w, 503, serverapi.ErrorCodeUnavailable, "Refresh unavailable while resetting or stopping.")
					return
				}
			} else {
				apiError(w, 503, serverapi.ErrorCodeUnavailable, "No refresh controller.")
				return
			}
			writeJSON(w, http.StatusAccepted, apiSyncState(a.status()))
		default:
			apiMethodNotAllowed(w, "GET, POST")
		}
	})
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
		epoch, release, permitErr := a.readPermit(ctx)
		if permitErr != nil {
			a.queryError(w, permitErr)
			return
		}
		defer release()
		data, err := loadDashboard(ctx, a.options.DBPath, q, time.Now())
		if err != nil {
			a.queryError(w, err)
			return
		}
		response := apiDashboard(data)
		if a.controller != nil {
			instance := a.controller.Status(ctx).InstanceID
			response.InstanceId = &instance
			response.DataEpoch = &epoch
		}
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
		epoch, release, permitErr := a.readPermit(ctx)
		if permitErr != nil {
			a.queryError(w, permitErr)
			return
		}
		defer release()
		database, err := db.Open(a.options.DBPath)
		if err != nil {
			a.queryError(w, err)
			return
		}
		defer func() { _ = database.Close() }()
		tx, err := db.BeginAnalyticsRead(ctx, database)
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
		snapshotStatus, err := db.LoadSyncStatus(ctx, tx)
		if err != nil {
			a.queryError(w, err)
			return
		}
		values.Revision = &snapshotStatus.Revision
		if a.controller != nil {
			instance := a.controller.Status(ctx).InstanceID
			values.InstanceId = &instance
			values.DataEpoch = &epoch
		}
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
	database, err := db.Open(a.options.DBPath)
	if err != nil {
		return "unknown"
	}
	defer func() { _ = database.Close() }()
	hostname, err := db.LatestIngestHostname(ctx, database)
	if err != nil {
		return "unknown"
	}
	return hostname
}

func (a *app) queryError(w http.ResponseWriter, err error) {
	_, _ = fmt.Fprintf(a.log, "%sdashboard query failed: %v\n", logIndent, err)
	if errors.Is(err, db.ErrMetadataUpgradeRequired) {
		apiError(w, http.StatusServiceUnavailable, serverapi.ErrorCodeUnavailable, "Usage metadata upgrade is pending. Retry sync.")
		return
	}
	if errors.Is(err, db.ErrRebuildPending) || errors.Is(err, db.ErrRecoveryRequired) {
		apiError(w, http.StatusServiceUnavailable, serverapi.ErrorCodeUnavailable, "Usage recovery is incomplete. Sync to rebuild local usage data.")
		return
	}
	apiError(w, http.StatusServiceUnavailable, serverapi.ErrorCodeUnavailable, "Cannot read dashboard data. Check terminal details, then sync or reload.")
}

func NewHandler(ctx context.Context, path string, controller *application.Controller, log io.Writer, bindHost string) http.Handler {
	a := newApp(ctx, Options{DBPath: path, Defaults: viewer.Selection{Period: "month", Bucket: "day"}}, log)
	a.controller = controller
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
func (a *app) readPermit(ctx context.Context) (string, func(), error) {
	if a.controller == nil {
		return "", func() {}, nil
	}
	return a.controller.ReadPermit(ctx)
}
