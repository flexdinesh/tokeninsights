// Package session owns one native window's backend lease: it receives a CLI
// handoff or resolves a standalone local launch, connects exactly the requested
// transport and acknowledges the native application only once the lease is live.
package session

import (
	"context"
	"errors"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localstorage"
)

// Session is one live backend lease and its presentation identity.
type Session struct {
	*localclient.Session
	Info    endpoint.Info
	handoff *desktoplaunch.Handoff
}

// Connector attaches a session for a launch link, or for local when link is empty.
type Connector func(ctx context.Context, link string) (*Session, error)

// Connect attaches the requested backend. A standalone launch captures locally;
// a handoff carries the launcher's exact connection and capture policy.
func Connect(ctx context.Context, link string) (*Session, error) {
	var spec desktoplaunch.Spec
	var handoff *desktoplaunch.Handoff
	if link != "" {
		var err error
		handoff, err = desktoplaunch.Receive(ctx, browser.NativeApp, link)
		if err != nil {
			return nil, err
		}
		spec = handoff.Spec
	} else {
		path, err := config.Path("")
		if err != nil {
			return nil, err
		}
		settings, err := config.ResolveLocal(path, true, config.Overrides{})
		if err != nil {
			return nil, err
		}
		spec = desktoplaunch.Specification(settings, true, false, "")
	}
	connected, err := desktoplaunch.Connect(ctx, spec, func() error {
		return localstorage.RequireCurrent(ctx, spec.CollectorDBPath, spec.ServerDBPath)
	})
	if err != nil {
		closeHandoff(handoff)
		return nil, err
	}
	info, err := desktoplaunch.Describe(ctx, spec, connected)
	if err != nil {
		closeHandoff(handoff)
		return nil, errors.Join(err, connected.Close())
	}
	return &Session{Session: connected, Info: info, handoff: handoff}, nil
}

// Acknowledge tells the launching CLI that this native window holds the live
// requested session. Standalone launches have nobody to tell.
func (s *Session) Acknowledge() error {
	select {
	case <-s.Done():
		return errors.New("requested backend disconnected")
	default:
	}
	if s.handoff != nil {
		return s.handoff.Acknowledge(browser.ModeNative)
	}
	return nil
}

// Close rejects an unacknowledged handoff and releases the lease.
func (s *Session) Close() error {
	closeHandoff(s.handoff)
	return s.Session.Close()
}

func closeHandoff(handoff *desktoplaunch.Handoff) {
	if handoff != nil {
		_ = handoff.Close()
	}
}
