package cli

import (
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
