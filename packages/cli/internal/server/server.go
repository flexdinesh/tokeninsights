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
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	serverapi "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

const DefaultPort = 8765
const shutdownTimeout = 10 * time.Second
const queryTimeout = 30 * time.Second

//go:embed static
var assets embed.FS

type Options struct {
	DBPath              string
	NoSync              bool
	Port                int
	Host                string
	ResolvePortConflict bool
	Input               io.Reader
	Defaults            viewer.Selection
}

type syncState struct {
	Running   bool              `json:"running"`
	Phase     string            `json:"phase"`
	Harnesses map[string]string `json:"harnesses"`
	Error     string            `json:"error"`
	Progress  *db.SyncStatus    `json:"-"`
	Revision  uint64            `json:"revision"`
}

type app struct {
	options Options
	ctx     context.Context
	mu      sync.Mutex
	jobs    sync.WaitGroup
	state   syncState
	syncer  func(context.Context, pipeline.SyncOptions) (pipeline.Summary, error)
	log     io.Writer
}

func newApp(ctx context.Context, options Options, log io.Writer) *app {
	return &app{options: options, ctx: ctx, log: log, syncer: pipeline.Sync, state: syncState{Phase: "ready", Harnesses: map[string]string{}}}
}

func (a *app) status() syncState {
	if a.options.DBPath != "" {
		shared, err := db.ReadSyncStatus(a.ctx, a.options.DBPath)
		if err == nil && (shared.JobID > 0 || shared.Revision > 0) {
			local := a.localStatus()
			if !local.Running || shared.Running {
				return syncState{Running: shared.Running, Phase: shared.Phase, Harnesses: shared.Harnesses, Error: shared.Error, Revision: uint64(shared.Revision), Progress: &shared}
			}
			local.Revision = uint64(shared.Revision)
			return local
		}
	}
	return a.localStatus()
}

func (a *app) localStatus() syncState {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.state
	s.Harnesses = map[string]string{}
	for k, v := range a.state.Harnesses {
		s.Harnesses[k] = v
	}
	return s
}

func (a *app) startSync() {
	if a.options.DBPath != "" {
		shared, err := db.ReadSyncStatus(a.ctx, a.options.DBPath)
		if err == nil && shared.Running && shared.AllHarnesses && shared.Normalize {
			return
		}
	}
	a.mu.Lock()
	if a.state.Running || a.ctx.Err() != nil {
		a.mu.Unlock()
		return
	}
	a.state.Running, a.state.Error, a.state.Phase = true, "", "syncing"
	a.state.Harnesses = map[string]string{}
	for _, h := range pipeline.SupportedHarnesses {
		a.state.Harnesses[string(h)] = "pending"
	}
	a.jobs.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.jobs.Done()
		_, err := a.syncer(a.ctx, pipeline.SyncOptions{DBPath: a.options.DBPath, Harnesses: pipeline.SupportedHarnesses, Normalize: true, Now: time.Now(), Progress: func(e pipeline.SyncProgressEvent) {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.state.Phase != string(pipeline.SyncProgressRebuilding) {
				a.state.Phase = string(e.Status)
			}
			if e.Published {
				a.state.Revision++
			}
			if e.Harness != "" {
				a.state.Harnesses[string(e.Harness)] = string(e.Status)
			}
		}})
		a.mu.Lock()
		defer a.mu.Unlock()
		a.state.Running = false
		a.state.Revision++
		a.state.Phase = "ready"
		if err != nil {
			// Pipeline errors can contain local paths; details belong in the terminal.
			_, _ = fmt.Fprintf(a.log, "%ssync failed: %v\n", logIndent, err)
			a.state.Error = "Sync failed. See terminal details, retry, or inspect existing data."
			a.state.Phase = "failed"
			if errors.Is(err, db.ErrRebuildPending) || errors.Is(err, db.ErrRecoveryRequired) || errors.Is(err, db.ErrMetadataUpgradeRequired) {
				a.state.Error = "Usage recovery is incomplete. Retry sync with the original source configuration and database. See terminal details."
				a.state.Phase = "rebuild_failed"
			}
		}
	}()
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
		writeJSON(w, http.StatusOK, serverapi.InstanceResponse{
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
			a.startSync()
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
		data, err := loadDashboard(ctx, a.options.DBPath, q, time.Now())
		if err != nil {
			a.queryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, apiDashboard(data))
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

func Run(parent context.Context, options Options, stdout, stderr io.Writer) error {
	return runServer(parent, options, stdout, stderr, openBrowser)
}

func runServer(parent context.Context, options Options, stdout, stderr io.Writer, open func(string) error) error {
	ctx, cancel := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if options.NoSync {
		database, err := db.Open(options.DBPath)
		if err != nil {
			return err
		}
		if err := database.Close(); err != nil {
			return err
		}
	}
	listener, err := acquireListener(ctx, options, stdout)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return err
	}
	if err := newConsole(stdout).startup(machineHostname(), displayURL(options.Host, port)); err != nil {
		return err
	}
	a := newApp(ctx, options, stderr)
	if !options.NoSync {
		a.startSync()
	}
	httpServer := &http.Server{Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, WriteTimeout: queryTimeout + 5*time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	failures := make(chan error, 1)
	go func() { failures <- httpServer.Serve(listener) }()
	if ctx.Err() == nil {
		host := options.Host
		if host == "0.0.0.0" {
			host = ""
		}
		url := displayURL(host, port)
		if openErr := open(url); openErr != nil {
			_, _ = fmt.Fprintf(stderr, "%scould not open browser: %v; open %s manually.\n", logIndent, openErr, url)
		}
	}
	select {
	case <-ctx.Done():
	case err = <-failures:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), shutdownTimeout)
	defer stop()
	if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
		_ = httpServer.Close()
		if err == nil {
			err = shutdownErr
		}
	}
	a.jobs.Wait()
	return err
}
