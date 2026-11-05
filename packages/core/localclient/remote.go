package localclient

import (
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

// RemoteOptions describe one lease on a backend reached through a private
// bridge. The remote machine validates its own storage settings.
type RemoteOptions struct {
	Capture, FullRefresh bool
	// SourceDir is interpreted on the remote machine and must be absolute.
	SourceDir string
	Token     string
}

var ErrRelativeRemoteSource = errors.New("remote --source-dir must be an absolute path on the remote machine")

// ConnectRemote attaches through dial, which carries prior-knowledge HTTP/2 to
// a remote backend's private socket. Only the protocol is checked here; the
// remote bridge already checked its backend against its own settings. Failure
// never selects local storage.
func ConnectRemote(parent context.Context, dial Dialer, options RemoteOptions) (*Session, error) {
	if options.FullRefresh && !options.Capture {
		return nil, errors.New("full refresh requires capture")
	}
	if options.SourceDir != "" && !filepath.IsAbs(options.SourceDir) {
		return nil, ErrRelativeRemoteSource
	}
	startup, cancel := context.WithTimeout(parent, StartupTimeout)
	defer cancel()
	s := newSession(dial, true)
	s.token = options.Token
	descriptor, err := s.descriptor(startup)
	if err != nil {
		s.transport.CloseIdleConnections()
		return nil, err
	}
	if err := descriptor.CompatibleProtocol(); err != nil {
		s.transport.CloseIdleConnections()
		return nil, err
	}
	if err := s.attach(parent, startup, descriptor, attachOptions{Capture: options.Capture, FullRefresh: options.FullRefresh, SourceDir: options.SourceDir}); err != nil {
		s.transport.CloseIdleConnections()
		return nil, err
	}
	return s, nil
}

// Bridge serves `system dial-stdio` on the remote machine: it discovers or
// starts the compatible owner of options' storage, then splices in/out with
// that owner's private socket until either side closes. It never writes
// diagnostics to out, which carries only protocol bytes.
func Bridge(parent context.Context, options Options, prepare func() error, in io.Reader, out io.Writer) error {
	settings, err := launchSettings(options)
	if err != nil {
		return err
	}
	startup, cancel := context.WithTimeout(parent, StartupTimeout)
	s, _, err := launch(startup, settings, options.BackendPath, prepare)
	cancel()
	if err != nil {
		return err
	}
	s.transport.CloseIdleConnections()
	socket, err := localprotocol.SocketPath(settings)
	if err != nil {
		return err
	}
	conn, err := (&net.Dialer{}).DialContext(parent, "unix", socket)
	if err != nil {
		return err
	}
	unix, ok := conn.(*net.UnixConn)
	if !ok {
		_ = conn.Close()
		return errors.New("local backend socket is not a Unix connection")
	}
	defer func() { _ = unix.Close() }()
	stop := context.AfterFunc(parent, func() { _ = unix.Close() })
	defer stop()
	// A process-scoped bridge may leave this copy blocked on stdin after the
	// backend closes; the command exits when the downstream copy returns.
	go func() {
		_, _ = io.Copy(unix, in)
		// Client EOF: stop sending so the backend closes the connection.
		_ = unix.CloseWrite()
	}()
	_, err = io.Copy(out, unix)
	if parent.Err() != nil {
		return nil
	}
	return err
}
