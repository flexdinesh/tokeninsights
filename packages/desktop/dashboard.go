package main

import (
	"context"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/core/localstorage"
)

type dashboard struct {
	*desktoplaunch.Dashboard
	handoff *desktoplaunch.Handoff
}

func (d *dashboard) Close() error {
	if d.handoff != nil {
		_ = d.handoff.Close()
	}
	return d.Dashboard.Close()
}
func (d *dashboard) acknowledge() error {
	select {
	case <-d.Done():
		return errors.New("requested backend disconnected")
	default:
	}
	if d.handoff != nil {
		return d.handoff.Acknowledge(browser.ModeDesktop)
	}
	return nil
}

// A standalone launch captures locally. Handoffs carry the originating policy;
// the launcher never captured, so exactly this new window requests capture.
func connectDashboard(ctx context.Context, target string, connection *string) (*dashboard, error) {
	var spec desktoplaunch.Spec
	var handoff *desktoplaunch.Handoff
	if target != "" {
		var err error
		handoff, err = desktoplaunch.Receive(ctx, target)
		if err != nil {
			return nil, err
		}
		spec = handoff.Spec
	} else {
		path, err := config.Path("")
		if err != nil {
			return nil, err
		}
		settings, err := config.ResolveLocal(path, true, config.Overrides{Connection: connection})
		if err != nil {
			return nil, err
		}
		spec = desktoplaunch.Specification(settings, true, false, "")
	}
	connected, err := desktoplaunch.Open(ctx, spec, "127.0.0.1", 0, func() error {
		return localstorage.RequireCurrent(ctx, spec.CollectorDBPath, spec.ServerDBPath)
	})
	if err != nil {
		if handoff != nil {
			_ = handoff.Close()
		}
		return nil, err
	}
	return &dashboard{Dashboard: connected, handoff: handoff}, nil
}
