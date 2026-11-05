package localclient

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/core/writerlock"
)

type Options struct {
	CollectorDBPath, ServerDBPath, BackendPath string
	Capture, FullRefresh                       bool
	SourceDir                                  string
}

func Connect(parent context.Context, options Options) (*Session, error) {
	return ConnectPrepared(parent, options, nil)
}

// ConnectPrepared runs legacy local-storage recovery only when starting an
// unowned backend, under startup serialization. Attached viewers never inspect
// or reopen the owner's stores. A nil preparation leaves recovery to the caller.
func ConnectPrepared(parent context.Context, options Options, prepare func() error) (*Session, error) {
	settings, err := launchSettings(options)
	if err != nil {
		return nil, err
	}
	if options.FullRefresh && !options.Capture {
		return nil, errors.New("full refresh requires capture")
	}
	if options.SourceDir != "" {
		options.SourceDir, err = filepath.Abs(options.SourceDir)
		if err != nil {
			return nil, err
		}
	}
	startup, cancelStartup := context.WithTimeout(parent, StartupTimeout)
	defer cancelStartup()
	s, descriptor, err := launch(startup, settings, options.BackendPath, prepare)
	if err != nil {
		return nil, err
	}
	if err := s.attach(parent, startup, descriptor, attachOptions{Capture: options.Capture, FullRefresh: options.FullRefresh, SourceDir: options.SourceDir}); err != nil {
		s.transport.CloseIdleConnections()
		return nil, err
	}
	return s, nil
}

func launchSettings(options Options) (localprotocol.Settings, error) {
	settings := localprotocol.Settings{CollectorDBPath: options.CollectorDBPath, ServerDBPath: options.ServerDBPath}
	return settings.Canonical()
}

// launch discovers or starts the compatible owner of settings under startup
// serialization. It returns an unattached session over the private socket.
func launch(startup context.Context, settings localprotocol.Settings, backendPath string, prepare func() error) (*Session, localprotocol.Descriptor, error) {
	socket, err := localprotocol.SocketPath(settings)
	if err != nil {
		return nil, localprotocol.Descriptor{}, err
	}
	release, err := writerlock.Acquire(startup, socket+".startup")
	if err != nil {
		return nil, localprotocol.Descriptor{}, err
	}
	defer release()
	s := socketSession(socket)
	descriptor, err := s.descriptor(startup)
	if err != nil {
		// A reachable but incompatible or corrupt endpoint must never trigger
		// fallback. Only a failed Unix connection permits owner inspection.
		var connection *net.OpError
		if !errors.As(err, &connection) {
			s.transport.CloseIdleConnections()
			return nil, descriptor, err
		}
		if err := startOwner(startup, settings, backendPath, prepare); err != nil {
			s.transport.CloseIdleConnections()
			return nil, descriptor, err
		}
		if descriptor, err = s.descriptor(startup); err != nil {
			s.transport.CloseIdleConnections()
			return nil, descriptor, err
		}
	}
	if err := descriptor.Compatible(settings); err != nil {
		s.transport.CloseIdleConnections()
		return nil, descriptor, err
	}
	return s, descriptor, nil
}

func startOwner(startup context.Context, settings localprotocol.Settings, backendPath string, prepare func() error) error {
	owner, held, err := serverownership.Lifetime(settings.ServerDBPath, false)
	if err != nil {
		return err
	}
	if owner != nil {
		_ = owner.Close()
	}
	if held {
		return localprotocol.ErrOwned
	}
	if prepare != nil {
		if err := prepare(); err != nil {
			return err
		}
	}
	return start(startup, backendPath, settings)
}

// BundledBackend resolves the adjacent binary. No PATH or remote fallback.
func BundledBackend() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(executable), "tokeninsights-backend"), nil
}

// SocketDialer dials one backend's private mode-0600 Unix socket.
func SocketDialer(socket string) Dialer {
	return func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}
}

func socketSession(socket string) *Session {
	return newSession(SocketDialer(socket), false)
}
