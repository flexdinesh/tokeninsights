package cli

import (
	"context"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

func localOptions(path string, settings config.Settings) service.Options {
	options := service.Options{DBPath: path}
	if settings.Host != "" {
		options.DefaultHost = &settings.Host
		options.DefaultPort = &settings.Port
	}
	return options
}
func ensureConfiguredLocal(ctx context.Context, path string, settings config.Settings) (service.State, error) {
	return service.Ensure(ctx, localOptions(path, settings))
}
func ensureViewConfiguredLocal(ctx context.Context, path string, settings config.Settings) (service.State, error) {
	return ensureViewServer(ctx, localOptions(path, settings))
}
