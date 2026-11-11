// Package desktoplaunch composes native viewer sessions and private desktop handoffs.
// Credentials stay in native processes; renderers receive only a loopback bridge.
package desktoplaunch

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardproxy"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/sshtransport"
	"github.com/flexdinesh/tokeninsights/packages/core/tcptransport"
)

// Spec is transferred only over a user-private Unix socket, never through argv,
// a URL, environment or a credential file.
type Spec struct {
	Connection                                                string
	CollectorDBPath, ServerDBPath, BackendPath, RemoteCommand string
	Capture, FullRefresh                                      bool
	SourceDir                                                 string
}

func Specification(settings config.LocalSettings, capture, fullRefresh bool, sourceDir string) Spec {
	command := ""
	if settings.Endpoint.SSH != nil {
		command = settings.Endpoint.SSH.Command
	}
	return Spec{Connection: settings.Endpoint.ConnectionString(), CollectorDBPath: settings.CollectorDBPath,
		ServerDBPath: settings.ServerDBPath, RemoteCommand: command, Capture: capture, FullRefresh: fullRefresh, SourceDir: sourceDir}
}

// Connect selects exactly the requested transport. A failed remote connection
// never starts a local backend. prepare runs only for a newly started local owner.
func Connect(ctx context.Context, spec Spec, prepare func() error) (*localclient.Session, error) {
	target, err := endpoint.Parse(spec.Connection)
	if err != nil {
		return nil, err
	}
	if !target.IsLocal() && spec.SourceDir != "" && !filepath.IsAbs(spec.SourceDir) {
		return nil, localclient.ErrRelativeRemoteSource
	}
	remote := localclient.RemoteOptions{Capture: spec.Capture, FullRefresh: spec.FullRefresh, SourceDir: spec.SourceDir}
	if target.SSH != nil {
		target.SSH.Command = spec.RemoteCommand
		return localclient.ConnectRemote(ctx, sshtransport.Dialer(*target.SSH, ""), remote)
	}
	if target.TCP != nil {
		if spec.FullRefresh || spec.SourceDir != "" {
			return nil, errors.New("TCP connections allow default capture only; use local or SSH for source overrides")
		}
		remote.Token = target.TCP.Token
		return localclient.ConnectRemote(ctx, tcptransport.Dialer(*target.TCP), remote)
	}
	return localclient.ConnectPrepared(ctx, localclient.Options{CollectorDBPath: spec.CollectorDBPath, ServerDBPath: spec.ServerDBPath,
		BackendPath: spec.BackendPath, Capture: spec.Capture, FullRefresh: spec.FullRefresh, SourceDir: spec.SourceDir}, prepare)
}

// Describe names the connected session's transport, exact target and backend
// host for presentation. It never includes credentials.
func Describe(ctx context.Context, spec Spec, session *localclient.Session) (endpoint.Info, error) {
	target, err := endpoint.Parse(spec.Connection)
	if err != nil {
		return endpoint.Info{}, err
	}
	instance, err := session.Query.Instance(ctx)
	if err != nil {
		return endpoint.Info{}, err
	}
	transport := "local"
	if target.SSH != nil {
		transport = "ssh"
	}
	if target.TCP != nil {
		transport = "tcp"
	}
	return endpoint.Info{Transport: transport, Target: target.String(), State: "connected", Hostname: instance.Hostname}, nil
}

// Dashboard owns one session and one bridge for one window.
type Dashboard struct {
	URL     string
	Session *localclient.Session
	bridge  *dashboardproxy.Bridge
}

func Open(ctx context.Context, spec Spec, host string, port int, prepare func() error) (*Dashboard, error) {
	session, err := Connect(ctx, spec, prepare)
	if err != nil {
		return nil, err
	}
	info, err := Describe(ctx, spec, session)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	bridge, err := dashboardproxy.Start(ctx, session.PrivateRoundTripper(), dashboardproxy.Options{Host: host, Port: port, Connection: info})
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	return &Dashboard{URL: bridge.URL, Session: session, bridge: bridge}, nil
}

func (d *Dashboard) Close() error          { return errors.Join(d.bridge.Close(), d.Session.Close()) }
func (d *Dashboard) Done() <-chan struct{} { return d.Session.Done() }
