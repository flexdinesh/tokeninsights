package localbackend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/collector"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/core/processusage"
	"github.com/flexdinesh/tokeninsights/packages/core/server"
	"github.com/flexdinesh/tokeninsights/packages/core/serverruntime"
)

type backend struct {
	runtime    *localruntime.Runtime
	descriptor localprotocol.Descriptor
	viewers    *viewers
	capture    *captures
	dashboard  *dashboard
}

// Serve opens storage once, ACKs only after the private socket is serving, and
// holds storage ownership through listener drain and all capture/processing joins.
// The public dashboard listener binds on demand for dashboard leases.
func Serve(parent context.Context, settings localprotocol.Settings, log io.Writer, ready func(localprotocol.Descriptor) error) error {
	return serve(parent, settings, log, ready, collector.Run)
}

func serve(parent context.Context, settings localprotocol.Settings, log io.Writer, ready func(localprotocol.Descriptor) error, collect func(context.Context, collector.Options) (collector.Result, error)) error {
	settings, err := settings.Canonical()
	if err != nil {
		return err
	}
	if err := settings.ValidateStorageIdentity(); err != nil {
		return err
	}
	socket, err := localprotocol.SocketPath(settings)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	runtime, err := localruntime.OpenWithAppOptions(ctx, settings.CollectorDBPath, settings.ServerDBPath, settings.AppDBPath, localruntime.Options{CaptureDetails: true})
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	// Only the lifetime owner can remove stale discovery state. An active owner
	// prevents Open above, including owners from incompatible executable builds.
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	private, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		return err
	}
	defer func() { _ = private.Close() }()
	if err := os.Chmod(socket, 0o600); err != nil {
		return err
	}
	b := &backend{runtime: runtime, descriptor: localprotocol.Descriptor{ProtocolVersion: localprotocol.ProtocolVersion, InstanceID: runtime.InstanceID, Settings: settings}, viewers: newViewers()}
	b.capture = &captures{runtime: runtime, ctx: ctx, run: collect}
	usage := processusage.New()
	dataHandler := func(host string) http.Handler {
		return server.NewDataHandlerWithOptions(ctx, runtime.DataSource(), log, server.DataHandlerOptions{Host: host, InstanceID: runtime.InstanceID, Hostname: runtime.Hostname, Policy: runtime.Policy, Progress: runtime.Progress, ProcessUsage: usage})
	}
	b.dashboard = newDashboard(ctx, runtime.Store, func(binding *dashboardBinding) http.Handler {
		data := dataHandler(binding.bindHost)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/local/v1/viewers" {
				data.ServeHTTP(w, r)
				return
			}
			if !viewerRequestAllowed(r, binding.bindHost) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			// A browser tab keeps the listener serving it, never another binding.
			releaseDashboard, ok := b.dashboard.acquireBound(binding)
			if !ok {
				http.Error(w, "shutting_down", http.StatusServiceUnavailable)
				return
			}
			defer releaseDashboard()
			b.stream(w, r, "")
		})
	})
	lifetimeDone := make(chan struct{})
	go func() { defer close(lifetimeDone); b.lifetime(ctx, cancel) }()
	defer func() { cancel(); <-lifetimeDone; b.dashboard.close(); b.capture.join() }()
	// The private mount uses loopback host rules; its callers already passed
	// the mode-0600 socket and the instance check.
	privateData := dataHandler("")
	return serverruntime.Serve(ctx, runtime.Store, []serverruntime.Binding{{Listener: private, Handler: b.privateHandler(privateData), UnencryptedHTTP2: true}}, func() error {
		if ready != nil {
			return ready(b.descriptor)
		}
		return nil
	})
}

func viewerRequestAllowed(r *http.Request, bindHost string) bool {
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		return false
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if bind, err := netip.ParseAddr(bindHost); err == nil && bind.IsLoopback() {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		address, err := netip.ParseAddr(host)
		return host == "localhost" || err == nil && address.IsLoopback()
	}
	return true
}

func (b *backend) lifetime(ctx context.Context, cancel context.CancelFunc) {
	for {
		b.viewers.mu.Lock()
		if b.viewers.state.expired(time.Now()) {
			b.viewers.mu.Unlock()
			cancel()
			return
		}
		delay := time.Until(b.viewers.state.deadline)
		if b.viewers.state.viewers > 0 {
			delay = StartupGrace
		}
		b.viewers.mu.Unlock()
		timer := time.NewTimer(max(delay, time.Millisecond))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-b.viewers.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// privateViewer holds a control lease, or a dashboard lease when the request
// names a listener. A dashboard lease reports its URL as the first event.
func (b *backend) privateViewer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	if !query.Has("host") && !query.Has("port") {
		b.stream(w, r, "")
		return
	}
	port, err := strconv.Atoi(query.Get("port"))
	if err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid_listen"})
		return
	}
	// Hold a viewer lease first so binding never races final shutdown.
	release, ok := b.viewers.attach()
	if !ok {
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": "shutting_down"})
		return
	}
	defer release()
	url, releaseDashboard, err := b.dashboard.acquire(r.Context(), localprotocol.Listen{Host: query.Get("host"), Port: port})
	if err != nil {
		switch {
		case errors.Is(err, localprotocol.ErrSettingsMismatch):
			respond(w, http.StatusConflict, map[string]string{"error": "listen_conflict"})
		case errors.Is(err, localprotocol.ErrListenUnavailable):
			respond(w, http.StatusConflict, map[string]string{"error": "listen_unavailable"})
		case errors.Is(err, localprotocol.ErrShuttingDown):
			respond(w, http.StatusServiceUnavailable, map[string]string{"error": "shutting_down"})
		default:
			respond(w, http.StatusBadRequest, map[string]string{"error": "invalid_listen"})
		}
		return
	}
	defer releaseDashboard()
	b.leaseStream(w, r, url)
}

func (b *backend) stream(w http.ResponseWriter, r *http.Request, url string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	release, ok := b.viewers.attach()
	if !ok {
		http.Error(w, "shutting_down", http.StatusServiceUnavailable)
		return
	}
	defer release()
	b.leaseStream(w, r, url)
}

func (b *backend) leaseStream(w http.ResponseWriter, r *http.Request, url string) {
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	if url != "" {
		body, err := json.Marshal(localprotocol.Dashboard{URL: url})
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "event: dashboard\ndata: %s\n\n", body); err != nil {
			return
		}
	}
	ticker := time.NewTicker(StreamHeartbeat)
	defer ticker.Stop()
	for {
		if _, err := io.WriteString(w, ": viewer\n\n"); err != nil {
			return
		}
		if err := controller.Flush(); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func respond(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func operationError(w http.ResponseWriter, err error) {
	code, status := "operation_failed", http.StatusInternalServerError
	switch {
	case errors.Is(err, localprotocol.ErrCaptureBusy):
		code, status = "capture_busy", http.StatusConflict
	case errors.Is(err, localruntime.ErrProcessingFailed):
		code, status = "processing_failed", http.StatusConflict
	case errors.Is(err, localruntime.ErrProcessingTimeout), errors.Is(err, context.DeadlineExceeded):
		code, status = "processing_timeout", http.StatusRequestTimeout
	case errors.Is(err, context.Canceled):
		code, status = "cancelled", http.StatusRequestTimeout
	}
	respond(w, status, map[string]string{"error": code})
}

func (b *backend) privateHandler(data http.Handler) http.Handler {
	queries := queryHandler(b.runtime.Application())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/local/v1/descriptor" && r.Method == http.MethodGet {
			descriptor := b.descriptor
			descriptor.URL, descriptor.Listen = b.dashboard.descriptor()
			respond(w, http.StatusOK, descriptor)
			return
		}
		if r.Header.Get("X-TokenInsights-Instance") != b.descriptor.InstanceID {
			http.Error(w, "instance_changed", http.StatusConflict)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/local/v1/queries/"):
			queries.ServeHTTP(w, r)
		case r.URL.Path == "/api/local/v1/viewers":
			b.privateViewer(w, r)
		case r.URL.Path == "/api/local/v1/progress" && r.Method == http.MethodGet:
			respond(w, http.StatusOK, b.runtime.Progress.Details())
		case r.URL.Path == "/api/local/v1/processing" && r.Method == http.MethodGet:
			status, err := b.runtime.ProcessingStatus(r.Context())
			if err != nil {
				operationError(w, err)
				return
			}
			respond(w, http.StatusOK, status)
		case r.URL.Path == "/api/local/v1/capture" && r.Method == http.MethodPost:
			var request localprotocol.CaptureRequest
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				http.Error(w, "invalid_capture", http.StatusBadRequest)
				return
			}
			if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
				http.Error(w, "invalid_capture", http.StatusBadRequest)
				return
			}
			if request.SourceDir != "" {
				path, err := filepath.Abs(request.SourceDir)
				if err != nil {
					http.Error(w, "invalid_source", http.StatusBadRequest)
					return
				}
				request.SourceDir = filepath.Clean(path)
			}
			ticket, err := b.capture.request(request)
			if err != nil {
				operationError(w, err)
				return
			}
			if r.URL.Query().Get("wait") == "true" {
				b.collectionStream(w, r, ticket)
				return
			}
			respond(w, http.StatusAccepted, struct{}{})
		case (r.URL.Path == "/api/local/v1/reprocess" || r.URL.Path == "/api/local/v1/wait") && r.Method == http.MethodPost:
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			if r.URL.Path == "/api/local/v1/reprocess" {
				if _, err := b.runtime.Store.Reprocess(ctx); err != nil {
					operationError(w, err)
					return
				}
			}
			if err := b.runtime.WaitVisible(ctx); err != nil {
				operationError(w, err)
				return
			}
			respond(w, http.StatusOK, struct{}{})
		case !strings.HasPrefix(r.URL.Path, "/api/local/"):
			// Read-only dashboard and API v2 for private bridges, such as an
			// SSH-forwarded web proxy, without binding a public listener.
			data.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func (b *backend) collectionStream(w http.ResponseWriter, r *http.Request, ticket *captureTicket) {
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ticker := time.NewTicker(StreamHeartbeat)
	defer ticker.Stop()
	for {
		if _, err := io.WriteString(w, ": collection\n\n"); err != nil {
			return
		}
		if err := controller.Flush(); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		case <-ticket.done:
			body, err := json.Marshal(ticket.result)
			if err == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
				_ = controller.Flush()
			}
			return
		}
	}
}
