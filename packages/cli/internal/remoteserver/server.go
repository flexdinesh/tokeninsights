// Package remoteserver composes an explicit foreground canonical server.
// Deployment supervision and backend/auth selection belong here.
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
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/clientaddress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverruntime"
)

type Settings struct {
	Listen, DBPath, AppDBPath, PublicURL, AdminSocket, Backend, PostgresDSN string
	TrustedProxies                                                          clientaddress.Policy
}

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
	switch settings.Backend {
	case "", "sqlite":
		if settings.DBPath == "" {
			return "", 0, fmt.Errorf("--server-db-path required")
		}
		if settings.PostgresDSN != "" {
			return "", 0, fmt.Errorf("postgres configuration requires --storage-backend=postgres")
		}
	case "postgres":
		if settings.PostgresDSN == "" {
			return "", 0, fmt.Errorf("TOKENINSIGHTS_POSTGRES_DSN required")
		}
		if settings.DBPath != "" || settings.AppDBPath != "" {
			return "", 0, fmt.Errorf("postgres backend rejects SQLite paths")
		}
		if settings.AdminSocket == "" {
			return "", 0, fmt.Errorf("postgres backend requires --admin-socket")
		}
	default:
		return "", 0, fmt.Errorf("invalid storage backend")
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
	listener, err := server.Listen(host, port)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	storage, err := openStorage(ctx, settings)
	if err != nil {
		return err
	}
	defer func() { _ = storage.close() }()
	if err := storage.accounts.Resume(storage.ctx); err != nil {
		return err
	}
	policy, _ := serverfeatures.New(serverfeatures.Hosted, false)
	options := server.DataHandlerOptions{Host: host, AllowIngestion: true, Policy: policy}
	if log == nil {
		log = io.Discard
	}
	runCtx, stop := context.WithCancel(storage.ctx)
	defer stop()
	options.PublicURL, err = canonicalPublicURL(settings.PublicURL)
	if err != nil {
		return err
	}
	options.Accounts = accounts.New(storage.accounts)
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		options.Accounts.RunCleanup(runCtx, func(err error) { _, _ = fmt.Fprintf(log, "account cleanup: %v\n", err) })
	}()
	defer func() { stop(); <-cleanupDone }()
	socket := storage.socket
	private, closePrivate, err := adminListener(socket)
	if err != nil {
		return err
	}
	defer closePrivate()
	handler := server.NewDataHandlerWithOptions(runCtx, storage.source, log, options)
	bindings := []serverruntime.Binding{{Listener: listener, Handler: settings.TrustedProxies.Handler(handler), Health: true}}
	bindings = append(bindings, serverruntime.Binding{Listener: private, Handler: options.Accounts.AdminHandler()})
	address := listener.Addr().String()
	if host == "0.0.0.0" {
		_, assignedPort, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		address = net.JoinHostPort("127.0.0.1", assignedPort)
	}
	runErr := serverruntime.Run(runCtx, storage.worker, log, bindings, func() error {
		if ready != nil {
			return ready("http://" + address)
		}
		return nil
	})
	return errors.Join(runErr, storage.failure())
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
