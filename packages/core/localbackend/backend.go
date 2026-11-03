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
}

// Serve opens storage once, ACKs only after both listeners are serving, and
// holds storage ownership through listener drain and all capture/processing joins.
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
	public, err := net.Listen("tcp4", net.JoinHostPort(settings.Host, fmt.Sprint(settings.Port)))
	if err != nil {
		return err
	}
	defer func() { _ = public.Close() }()
	host, port, err := net.SplitHostPort(public.Addr().String())
	if err != nil {
		return err
	}
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	b := &backend{runtime: runtime, descriptor: localprotocol.Descriptor{ProtocolVersion: localprotocol.ProtocolVersion, InstanceID: runtime.InstanceID, URL: "http://" + net.JoinHostPort(host, port), Settings: settings}, viewers: newViewers()}
	b.capture = &captures{runtime: runtime, ctx: ctx, run: collect}
	lifetimeDone := make(chan struct{})
	go func() { defer close(lifetimeDone); b.lifetime(ctx, cancel) }()
	defer func() { cancel(); <-lifetimeDone; b.capture.join() }()
	data := server.NewDataHandlerWithOptions(ctx, runtime.DataSource(), log, server.DataHandlerOptions{Host: settings.Host, InstanceID: runtime.InstanceID, Hostname: runtime.Hostname, Policy: runtime.Policy, Progress: runtime.Progress, ProcessUsage: processusage.New()})
	publicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/local/v1/viewers" {
			data.ServeHTTP(w, r)
			return
		}
		if !viewerRequestAllowed(r, settings.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		b.stream(w, r)
	})
	return serverruntime.Serve(ctx, runtime.Store, []serverruntime.Binding{{Listener: public, Handler: publicHandler}, {Listener: private, Handler: b.privateHandler()}}, func() error {
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

func (b *backend) stream(w http.ResponseWriter, r *http.Request) {
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
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
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

func (b *backend) privateHandler() http.Handler {
	queries := queryHandler(b.runtime.Application())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/local/v1/descriptor" && r.Method == http.MethodGet {
			respond(w, http.StatusOK, b.descriptor)
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
			b.stream(w, r)
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
