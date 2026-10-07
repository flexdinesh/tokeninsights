package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
)

type ready struct {
	Record *Record `json:"record,omitempty"`
	Error  string  `json:"error,omitempty"`
}

func Child(ctx context.Context) error {
	input := os.NewFile(3, "configuration")
	output := os.NewFile(4, "readiness")
	if input == nil || output == nil {
		return fmt.Errorf("missing startup descriptors")
	}
	defer func() { _ = input.Close(); _ = output.Close() }()
	var config Config
	if err := decode(input, &config); err != nil {
		return err
	}
	if err := validateConfig(config); err != nil {
		return err
	}
	p, err := servicePaths(config.DatabaseKey, true)
	if err != nil {
		return err
	}
	log, err := openLog(p.log)
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()
	err = runtime(ctx, config, log, func(r Record) error {
		err := json.NewEncoder(output).Encode(ready{Record: &r})
		_ = output.Close()
		return err
	})
	if err != nil {
		_ = json.NewEncoder(output).Encode(ready{Error: err.Error()})
	}
	return err
}

func runtime(parent context.Context, config Config, log io.Writer, onReady func(Record) error) error {
	if err := validateConfig(config); err != nil {
		return err
	}
	if err := server.ValidateHost(config.Host); err != nil {
		return err
	}
	p, err := servicePaths(config.DatabaseKey, true)
	if err != nil {
		return err
	}
	owner, held, err := lifetime(config.DBPath, true)
	if err != nil {
		return err
	}
	if held {
		return fmt.Errorf("service already owns database")
	}
	defer func() { _ = owner.Close() }()
	// Lifetime ownership is acquired before removing stale per-instance files.
	for _, recordPath := range []string{p.record, p.fallbackRecord} {
		var stale Record
		if err := readFile(recordPath, &stale); err == nil {
			if stale.Config.DatabaseKey != config.DatabaseKey || stale.Config.DBPath != config.DBPath {
				return fmt.Errorf("service discovery identity collision")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, path := range []string{p.socket, p.record, p.fallbackRecord} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	private, err := net.Listen("unix", p.socket)
	if err != nil {
		return err
	}
	defer func() { _ = private.Close(); _ = os.Remove(p.socket) }()
	if err := os.Chmod(p.socket, 0o600); err != nil {
		return err
	}
	public, err := server.Listen(config.Host, config.Port)
	if err != nil {
		return err
	}
	defer func() { _ = public.Close() }()
	store, err := serverruntime.Open(parent, config.DBPath, datastore.Options{Kind: datastore.KindPersonal})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	if err := validateConfig(config); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	_, port, err := net.SplitHostPort(public.Addr().String())
	if err != nil {
		return err
	}
	host := config.Host
	if host == "0.0.0.0" || host == "" {
		host = server.DefaultHost
	}
	record := Record{SchemaVersion: datastore.SchemaVersion, Config: config, InstanceID: instanceID(), PID: os.Getpid(), Protocol: protocolVersion, Version: version.Version, URL: "http://" + net.JoinHostPort(host, port), Address: public.Addr().String(), Socket: p.socket, StartedAt: time.Now()}
	record.Config.Token = ""
	progress := collectorprogress.New(record.InstanceID)
	defer progress.InterruptAll()
	publicHandler := server.NewPersonalDataHandler(ctx, store, log, config.Host, record.InstanceID, progress)
	defer func() {
		for _, recordPath := range []string{p.record, p.fallbackRecord} {
			var current Record
			if readFile(recordPath, &current) == nil && current.InstanceID == record.InstanceID {
				_ = os.Remove(recordPath)
			}
		}
	}()
	return serverruntime.Run(ctx, store, log, []serverruntime.Binding{
		{Listener: public, Handler: publicHandler, Health: true},
		{Listener: private, Handler: controlHandlerWithProgress(record, cancel, progress, store)},
	}, func() error {
		if err := atomicFile(p.record, record); err != nil {
			return err
		}
		// Stable fallback discovery supports callers with a different runtime
		// environment. All writes remain on the private instance-verified socket.
		if p.fallbackRecord != p.record {
			if err := atomicFile(p.fallbackRecord, record); err != nil {
				return err
			}
		}
		if err := atomicFile(p.config, config); err != nil {
			return err
		}
		// Parent exit after successful startup must not kill the detached child.
		if err := onReady(record); err != nil {
			_, _ = fmt.Fprintf(log, "readiness delivery: %v\n", err)
		}
		return nil
	})
}

func controlHandlerWithProgress(record Record, shutdown context.CancelFunc, progress *collectorprogress.Registry, data ...*datastore.Store) http.Handler {
	mux := http.NewServeMux()
	if progress != nil {
		mux.Handle("/control/v1/collector-progress", progress.ControlHandler())
	}
	if len(data) > 0 {
		mux.Handle(datastore.IngestionPrefix, data[0].Handler())
		mux.Handle(datastore.RawV2IngestionPrefix, data[0].Handler())
		mux.Handle(datastore.LegacyIngestionPrefix, data[0].Handler())
		mux.Handle(datastore.ProcessingPrefix, data[0].Handler())
	}
	write := func(w http.ResponseWriter, status int, value interface{}) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("GET /control/v1/instance", func(w http.ResponseWriter, r *http.Request) { write(w, 200, record) })
	mux.HandleFunc("GET /control/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if len(data) > 0 {
			metadata, err := data[0].Metadata(r.Context())
			if err != nil {
				write(w, 503, struct {
					Error string `json:"error"`
				}{"server storage unavailable"})
				return
			}
			var pending int64
			if err := data[0].SQL().QueryRowContext(r.Context(), "SELECT COUNT(*) FROM processing.scopes WHERE dataset_id=? AND (processed_revision<>revision OR generation<>?)", metadata.DatasetID, metadata.TargetGeneration).Scan(&pending); err != nil {
				write(w, 503, map[string]string{"error": "data_unavailable"})
				return
			}
			write(w, 200, Status{InstanceID: record.InstanceID, DataEpoch: metadata.DatabaseID, DataReadiness: "ready", Revision: metadata.Revision, LastIngestionAtMS: metadata.LastIngestionAtMs, PendingProcessing: pending})
			return
		}
		write(w, http.StatusServiceUnavailable, map[string]string{"error": "server storage unavailable"})
	})
	mux.HandleFunc("POST /control/v1/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if err := decode(r.Body, &struct{}{}); err != nil && err != io.EOF {
			write(w, 400, struct {
				Error string `json:"error"`
			}{"invalid shutdown request"})
			return
		}
		write(w, 202, struct {
			Accepted bool `json:"accepted"`
		}{true})
		go shutdown()
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TokenInsights-Instance") != record.InstanceID {
			write(w, 409, struct {
				Error string `json:"error"`
			}{"service identity mismatch"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}
