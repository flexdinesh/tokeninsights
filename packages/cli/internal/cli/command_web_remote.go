package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardproxy"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/core/serverruntime"
)

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

// remoteOpenMode keeps remote dashboards in the browser: desktop viewers
// attach their own endpoint instead of a proxied URL.
func remoteOpenMode(mode browser.Mode) (browser.Mode, error) {
	switch mode {
	case browser.ModeMyGo, browser.ModeElectron:
		return "", fmt.Errorf("web --endpoint opens only the browser; launch TokenInsights MyGo with --endpoint instead of --open=%s", mode)
	case browser.ModeAuto:
		return browser.ModeBrowser, nil
	}
	return mode, nil
}

// runRemoteWeb serves a remote backend's read-only dashboard on a local
// loopback-by-default listener. This foreground command holds one remote
// lease; browser tabs hold their own through the proxy. Remote failures never
// fall back to local storage.
func runRemoteWeb(invocation commandInvocation, settings config.LocalSettings, syncBefore bool, capture pipeline.SyncOptions) error {
	mode, err := remoteOpenMode(settings.Open)
	if err != nil {
		return err
	}
	if capture.SourceDir != "" && !filepath.IsAbs(capture.SourceDir) {
		return localclient.ErrRelativeRemoteSource
	}
	// Reserve the local address before connecting; an occupied port fails.
	listener, err := net.Listen("tcp4", net.JoinHostPort(settings.Host, strconv.Itoa(settings.Port)))
	if err != nil {
		return fmt.Errorf("listen on %s:%d: %w", settings.Host, settings.Port, err)
	}
	ctx, cancel := context.WithCancel(invocation.context)
	defer cancel()
	session, err := connectRemoteBackend(ctx, settings.Endpoint.SSH.Dialer(""), localclient.RemoteOptions{Capture: syncBefore, FullRefresh: capture.FullRefresh, SourceDir: capture.SourceDir})
	if err != nil {
		_ = listener.Close()
		return err
	}
	defer func() { _ = session.Close() }()
	serving, err := serverruntime.Start(ctx, sessionReadiness{session}, serverruntime.Binding{Listener: listener, Handler: dashboardproxy.Handler(session.PrivateRoundTripper(), settings.Host)})
	if err != nil {
		_ = listener.Close()
		return err
	}
	defer func() {
		shutdown, cancelShutdown := context.WithTimeout(context.Background(), serverruntime.ShutdownTimeout)
		defer cancelShutdown()
		_ = serving.Shutdown(shutdown)
	}()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return err
	}
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	dashboardURL := "http://" + net.JoinHostPort(host, port)
	printWebReady(invocation.stdout, dashboardURL, mode != browser.ModeOff && remoteSession())
	_, _ = fmt.Fprintf(invocation.stderr, "Serving %s from %s.\n", dashboardURL, settings.Endpoint)
	reportOpen(ctx, invocation, mode, dashboardURL)
	select {
	case <-ctx.Done():
		return nil
	case <-session.Done():
		return fmt.Errorf("remote backend %s disconnected", settings.Endpoint)
	}
}
