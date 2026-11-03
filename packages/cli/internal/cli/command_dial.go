package cli

import (
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
)

// runDialStdio is the remote end of an SSH endpoint. It always serves this
// machine's backend: it resolves local configuration, attaches to or starts the
// backend, then splices stdio with its private socket. Diagnostics go to
// stderr, which ssh relays; stdout carries only protocol bytes.
func runDialStdio(invocation commandInvocation) error {
	local := endpoint.Local
	settings, err := invocation.resolveLocalOverrides(config.Overrides{Endpoint: &local})
	if err != nil {
		return err
	}
	return localclient.Bridge(invocation.context, localclient.Options{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath, AppDBPath: settings.ApplicationPath()},
		func() error { return invocation.prepareLocalStorage(settings) }, invocation.stdin, invocation.stdout)
}
