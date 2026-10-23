package config

import (
	"flag"
	"fmt"
	"strconv"
)

// FlagOverrides returns overrides for configuration flags explicitly set on a
// command's parsed flag set. Commands parse arguments once; resolution then
// applies flag > environment > file > default precedence.
func FlagOverrides(flags *flag.FlagSet) (Overrides, error) {
	var overrides Overrides
	var err error
	flags.Visit(func(f *flag.Flag) {
		value := f.Value.String()
		switch f.Name {
		case "app-db-path":
			overrides.AppDBPath = &value
		case "server-db-path":
			overrides.ServerDBPath = &value
		case "collector-db-path":
			overrides.CollectorDBPath = &value
		case "server-url":
			overrides.ServerURL = &value
		case "host":
			if value == "" {
				value = Defaults().Host
			}
			overrides.Host = &value
		case "port":
			port, parseErr := strconv.Atoi(value)
			if parseErr != nil {
				err = fmt.Errorf("invalid port")
				return
			}
			overrides.Port = &port
		}
	})
	return overrides, err
}
