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
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

const DefaultPort = 8765
const shutdownTimeout = 10 * time.Second
const queryTimeout = 30 * time.Second

//go:embed static
var assets embed.FS

type Options struct {
	DBPath   string
	NoSync   bool
	Port     int
	Host     string
	Defaults viewer.Selection
}

type syncState struct {
	Running   bool              `json:"running"`
	Phase     string            `json:"phase"`
	Harnesses map[string]string `json:"harnesses"`
	Error     string            `json:"error"`
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
			if e.Harness == "" || a.state.Phase != string(pipeline.SyncProgressRebuilding) {
				a.state.Phase = string(e.Status)
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
			fmt.Fprintf(a.log, "sync failed: %v\n", err)
			a.state.Error = "Sync failed. See terminal details, retry, or inspect existing data."
			a.state.Phase = "failed"
			if errors.Is(err, db.ErrRebuildPending) || errors.Is(err, db.ErrRecoveryRequired) {
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

func apiError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{message})
}

func (a *app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		hostname, err := os.Hostname()
		if err != nil || hostname == "" {
			hostname = "unknown"
		}
		writeJSON(w, http.StatusOK, struct {
			Defaults viewer.Selection `json:"defaults"`
			Hostname string           `json:"hostname"`
			Timezone string           `json:"timezone"`
		}{a.options.Defaults, hostname, time.Now().Format("MST -07:00")})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, a.status()) })
	mux.HandleFunc("POST /api/sync", func(w http.ResponseWriter, r *http.Request) {
		a.startSync()
		writeJSON(w, http.StatusAccepted, a.status())
	})
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		q, err := parseQuery(r.URL.Query())
		if err != nil {
			apiError(w, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
		defer cancel()
		data, err := loadDashboard(ctx, a.options.DBPath, q, time.Now())
		if err != nil {
			a.queryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, data)
	})
	mux.HandleFunc("GET /api/filters", func(w http.ResponseWriter, r *http.Request) {
		q, err := parseQuery(r.URL.Query())
		if err != nil {
			apiError(w, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
		defer cancel()
		database, err := db.Open(a.options.DBPath)
		if err != nil {
			a.queryError(w, err)
			return
		}
		defer database.Close()
		tx, err := db.BeginAnalyticsRead(ctx, database)
		if err != nil {
			a.queryError(w, err)
			return
		}
		defer tx.Rollback()
		f := q.Selection.Filter(time.Now())
		values := map[string][]string{}
		values["providers"], err = db.AvailableProviders(ctx, tx, f)
		if err != nil {
			a.queryError(w, err)
			return
		}
		values["models"], err = db.AvailableModels(ctx, tx, f)
		if err != nil {
			a.queryError(w, err)
			return
		}
		values["harnesses"], err = db.AvailableHarnesses(ctx, tx, f)
		if err != nil {
			a.queryError(w, err)
			return
		}
		values["sessions"], err = db.AvailableSessions(ctx, tx, f, r.URL.Query().Get("search"), sessionOptionLimit)
		if err != nil {
			a.queryError(w, err)
			return
		}
		if err = tx.Commit(); err != nil {
			a.queryError(w, err)
			return
		}
		for k, v := range values {
			if v == nil {
				values[k] = []string{}
			}
		}
		writeJSON(w, http.StatusOK, values)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { apiError(w, http.StatusNotFound, "Unknown API route") })
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
		files.ServeHTTP(w, r)
	})
	// Same-origin browser requests work on the selected LAN address.
	return http.NewCrossOriginProtection().Handler(mux)
}

func (a *app) queryError(w http.ResponseWriter, err error) {
	fmt.Fprintf(a.log, "dashboard query failed: %v\n", err)
	if errors.Is(err, db.ErrRebuildPending) || errors.Is(err, db.ErrRecoveryRequired) {
		apiError(w, http.StatusServiceUnavailable, "Usage recovery is incomplete. Sync to rebuild local usage data.")
		return
	}
	apiError(w, http.StatusServiceUnavailable, "Cannot read dashboard data. Check terminal details, then sync or reload.")
}

func Run(parent context.Context, options Options, stdout, stderr io.Writer) error {
	ctx, cancel := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if options.NoSync {
		database, err := db.Open(options.DBPath)
		if err != nil {
			return err
		}
		database.Close()
	}
	listener, err := listen(options.Host, options.Port)
	if err != nil {
		return err
	}
	defer listener.Close()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return err
	}
	var lan string
	if host == DefaultHost {
		var lanErr error
		lan, lanErr = primaryLANIPv4()
		if lanErr != nil {
			fmt.Fprintf(stderr, "LAN URL unavailable: %v\n", lanErr)
		}
	}
	for _, address := range displayURLs(host, port, lan) {
		fmt.Fprintln(stdout, address)
	}
	fmt.Fprintln(stdout, "Press Ctrl+C to stop.")
	a := newApp(ctx, options, stderr)
	if !options.NoSync {
		a.startSync()
	}
	httpServer := &http.Server{Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, WriteTimeout: queryTimeout + 5*time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	failures := make(chan error, 1)
	go func() { failures <- httpServer.Serve(listener) }()
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
