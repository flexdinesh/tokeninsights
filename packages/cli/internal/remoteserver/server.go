// Package remoteserver composes an explicit foreground canonical server.
// Deployment supervision and future backend/auth selection belong here.
package remoteserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
)

const shutdownTimeout = 15 * time.Second

type Settings struct{ Listen, DBPath, LegacyDBPath string }

func (settings Settings) Validate() (string, int, error) {
	host, portString, err := net.SplitHostPort(settings.Listen)
	if err != nil {
		return "", 0, fmt.Errorf("invalid --listen; use IPv4:port")
	}
	if err := server.ValidateHost(host); err != nil {
		return "", 0, err
	}
	if host == "" {
		host = "0.0.0.0"
	}
	port, err := strconv.Atoi(portString)
	if err != nil || port < 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid listen port")
	}
	if settings.DBPath == "" {
		return "", 0, fmt.Errorf("--server-db-path required")
	}
	return host, port, nil
}

func Run(ctx context.Context, settings Settings, log io.Writer, ready func(string) error) error {
	host, port, err := settings.Validate()
	if err != nil {
		return err
	}
	path, _, err := serverownership.Identify(settings.DBPath)
	if err != nil {
		return err
	}
	release, err := db.AcquireWriterLock(ctx, path+".service.op")
	if err != nil {
		return err
	}
	defer release()
	owner, held, err := serverownership.Lifetime(path, true)
	if err != nil {
		return err
	}
	if held {
		return fmt.Errorf("server already owns database")
	}
	defer func() { _ = owner.Close() }()
	listener, err := server.Listen(host, port)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	writer, err := db.AcquireWriterLock(ctx, path)
	if err != nil {
		return err
	}
	store, err := datastore.OpenWithLegacy(ctx, path, settings.LegacyDBPath)
	writer()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		store.Run(workerCtx, func(err error) { _, _ = fmt.Fprintf(log, "processing: %v\n", err) })
	}()
	defer func() { stopWorker(); <-workerDone }()
	handler := server.NewDataHandler(ctx, store, log, host, "", true)
	httpServer := &http.Server{Handler: handler, BaseContext: func(net.Listener) context.Context { return ctx }, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second}
	defer func() { _ = httpServer.Close() }()
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	release()
	address := listener.Addr().String()
	if host == "0.0.0.0" {
		_, assignedPort, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		address = net.JoinHostPort("127.0.0.1", assignedPort)
	}
	if ready != nil {
		if err := ready("http://" + address); err != nil {
			return err
		}
	}
	select {
	case <-ctx.Done():
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return errors.Join(err, httpServer.Shutdown(shutdown))
}
