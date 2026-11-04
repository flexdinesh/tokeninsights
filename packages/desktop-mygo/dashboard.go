package main

// Backend attachment stays here. The window loads the dashboard the backend
// serves; every dashboard view lives in packages/dashboard and packages/web.
import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardproxy"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/core/serverruntime"
	"github.com/flexdinesh/tokeninsights/packages/desktop-mygo/internal/link"
)

// proxyHost binds the local proxy of a remote dashboard to this machine only.
const proxyHost = "127.0.0.1"

// externalTarget is a dashboard this shell does not host: it opens in the browser.
type externalTarget struct{ URL string }

func (*externalTarget) Error() string { return "open this server in your browser" }

// dashboard is one held viewer lease and the URL serving it.
type dashboard struct {
	URL     string
	session *localclient.Session
	// proxy serves a remote backend locally; nil for local dashboards.
	proxy *serverruntime.Listening
}

func (d *dashboard) Done() <-chan struct{} { return d.session.Done() }

// Close stops any local proxy, then releases the backend lease.
func (d *dashboard) Close() error {
	var proxyErr error
	if d.proxy != nil {
		ctx, cancel := context.WithTimeout(context.Background(), serverruntime.ShutdownTimeout)
		defer cancel()
		proxyErr = d.proxy.Shutdown(ctx)
	}
	return errors.Join(proxyErr, d.session.Close())
}

// connectDashboard attaches the configured backend. endpoint, when non-nil, is
// the launch --endpoint flag; URL handoffs always attach this machine's owner.
func connectDashboard(ctx context.Context, target string, endpoint *string) (*dashboard, error) {
	if target != "" {
		return attachLink(ctx, target)
	}
	path, err := config.Path("")
	if err != nil {
		return nil, err
	}
	settings, err := config.ResolveLocal(path, true, config.Overrides{Endpoint: endpoint})
	if err != nil {
		return nil, err
	}
	if remote := settings.Endpoint.SSH; remote != nil {
		session, err := localclient.ConnectRemote(ctx, remote.Dialer(""), localclient.RemoteOptions{Capture: true})
		if err != nil {
			return nil, err
		}
		return serveProxy(ctx, session)
	}
	// A standalone launch collects, as every other viewer's launch does.
	session, err := localclient.Connect(ctx, localclient.Options{
		CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath,
		AppDBPath: settings.ApplicationPath(), Host: settings.Host, Port: settings.Port,
		Capture: true, Dashboard: true,
	})
	if err != nil {
		return nil, err
	}
	return &dashboard{URL: session.URL, session: session}, nil
}

// attachLink holds a lease on the local owner a URL handoff names. A handoff
// inherits its launcher's capture policy, so it never captures. Anything other
// than this machine's personal owner stays in the browser.
func attachLink(ctx context.Context, target string) (*dashboard, error) {
	parsed, err := link.Dashboard(target)
	if err != nil {
		return nil, err
	}
	if !parsed.Loopback {
		return nil, &externalTarget{URL: target}
	}
	client, err := queryclient.New(parsed.Origin, nil)
	if err != nil {
		return nil, err
	}
	instance, err := client.Instance(ctx)
	if err == nil {
		if instance.Kind != serverfeatures.Personal {
			return nil, &externalTarget{URL: target}
		}
	} else {
		var status *queryclient.StatusError
		if errors.As(err, &status) {
			return nil, &externalTarget{URL: target}
		}
		return nil, fmt.Errorf("cannot inspect local dashboard: %w", err)
	}
	session, err := localclient.ConnectEndpoint(ctx, parsed.Origin, false, true)
	if errors.Is(err, localclient.ErrEndpointMissing) {
		return nil, &externalTarget{URL: target}
	}
	if err != nil {
		return nil, err
	}
	return &dashboard{URL: parsed.URL, session: session}, nil
}

// sessionReadiness reports the remote session as the proxy's readiness.
type sessionReadiness struct{ session *localclient.Session }

func (s sessionReadiness) Ready(context.Context) error {
	select {
	case <-s.session.Done():
		return errors.New("remote backend disconnected")
	default:
		return nil
	}
}

// serveProxy takes ownership of session and serves its read-only dashboard on
// an available loopback port, as `web --endpoint` does.
func serveProxy(ctx context.Context, session *localclient.Session) (*dashboard, error) {
	listener, err := net.Listen("tcp4", net.JoinHostPort(proxyHost, "0"))
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	serving, err := serverruntime.Start(ctx, sessionReadiness{session}, serverruntime.Binding{
		Listener: listener, Handler: dashboardproxy.Handler(session.PrivateRoundTripper(), proxyHost),
	})
	if err != nil {
		_ = listener.Close()
		_ = session.Close()
		return nil, err
	}
	return &dashboard{URL: "http://" + listener.Addr().String(), session: session, proxy: serving}, nil
}
