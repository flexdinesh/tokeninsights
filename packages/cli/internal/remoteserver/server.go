// Package remoteserver composes an explicit foreground canonical server.
// Deployment supervision and future backend/auth selection belong here.
package remoteserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/duckdb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqliteaccounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/appstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverruntime"
)

type Settings struct{ Listen, DBPath, AppDBPath, PublicURL, AdminSocket string }

func canonicalPublicURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" || u.RawPath != "" {
		return "", fmt.Errorf("hosted --public-url requires canonical HTTPS origin")
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("hosted --public-url requires canonical HTTPS origin")
	}
	u.Host = strings.TrimSuffix(strings.ToLower(u.Host), ":443")
	return strings.TrimSuffix(u.String(), "/"), nil
}

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
	if settings.AdminSocket != "" && !filepath.IsAbs(settings.AdminSocket) {
		return "", 0, fmt.Errorf("--admin-socket requires absolute path")
	}
	if _, err := canonicalPublicURL(settings.PublicURL); err != nil {
		return "", 0, err
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
	if settings.AppDBPath == "" {
		settings.AppDBPath = filepath.Join(filepath.Dir(path), "app.sqlite")
	}
	if err := collector.ValidatePaths(settings.AppDBPath, path+".application.json"); err != nil {
		return err
	}
	if err := collector.ValidatePaths(settings.AppDBPath, path); err != nil {
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
	policy, _ := serverfeatures.New(serverfeatures.Hosted, false)
	store, err := duckdb.Open(ctx, path, datastore.Options{Kind: datastore.KindHosted})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	identity, err := store.DatabaseIdentity(ctx)
	if err != nil {
		return err
	}
	app, err := appstore.OpenPaired(ctx, settings.AppDBPath, path, identity, datastore.KindHosted)
	if err != nil {
		return err
	}
	defer func() { _ = app.Close() }()
	repository := sqliteaccounts.NewSQLite(app, store)
	if err := repository.Resume(ctx); err != nil {
		return err
	}
	options := server.DataHandlerOptions{Host: host, AllowIngestion: true, Policy: policy}
	if log == nil {
		log = io.Discard
	}
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	options.PublicURL, err = canonicalPublicURL(settings.PublicURL)
	if err != nil {
		return err
	}
	options.Accounts = accounts.New(repository)
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		options.Accounts.RunCleanup(runCtx, func(err error) { _, _ = fmt.Fprintf(log, "account cleanup: %v\n", err) })
	}()
	defer func() { stop(); <-cleanupDone }()
	socket := settings.AdminSocket
	if socket == "" {
		socket = path + ".admin.sock"
	}
	private, closePrivate, err := adminListener(socket)
	if err != nil {
		return err
	}
	defer closePrivate()
	handler := server.NewDataHandlerWithOptions(runCtx, duckdb.Source{Store: store}, log, options)
	bindings := []serverruntime.Binding{{Listener: listener, Handler: handler, Health: true}}
	bindings = append(bindings, serverruntime.Binding{Listener: private, Handler: options.Accounts.AdminHandler()})
	release()
	address := listener.Addr().String()
	if host == "0.0.0.0" {
		_, assignedPort, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		address = net.JoinHostPort("127.0.0.1", assignedPort)
	}
	return serverruntime.Run(runCtx, store, log, bindings, func() error {
		if ready != nil {
			return ready("http://" + address)
		}
		return nil
	})
}

func adminListener(path string) (net.Listener, func(), error) {
	if !filepath.IsAbs(path) {
		return nil, nil, fmt.Errorf("--admin-socket requires absolute path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, err
	}
	owner, held, err := serverownership.Lifetime(path, true)
	if err != nil {
		return nil, nil, err
	}
	if held {
		return nil, nil, fmt.Errorf("admin socket already owned")
	}
	closeOwner := func() { _ = owner.Close() }
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			closeOwner()
			return nil, nil, fmt.Errorf("admin socket path occupied")
		}
		connection, dialErr := net.DialTimeout("unix", path, time.Second)
		if dialErr == nil {
			_ = connection.Close()
			closeOwner()
			return nil, nil, fmt.Errorf("admin socket already in use")
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) && !errors.Is(dialErr, os.ErrNotExist) {
			closeOwner()
			return nil, nil, fmt.Errorf("admin socket unavailable")
		}
		if err := os.Remove(path); err != nil {
			closeOwner()
			return nil, nil, err
		}
	} else if !os.IsNotExist(err) {
		closeOwner()
		return nil, nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		closeOwner()
		return nil, nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		closeOwner()
		return nil, nil, err
	}
	return listener, func() { _ = listener.Close(); _ = os.Remove(path); closeOwner() }, nil
}
