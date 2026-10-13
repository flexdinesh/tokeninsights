package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/clientaddress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/remoteserver"
)

const defaultListen = "0.0.0.0:8766"

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func listenDefault() string {
	return envDefault("TOKENINSIGHTS_LISTEN", "0.0.0.0:"+envDefault("TOKENINSIGHTS_SERVER_PORT", strings.TrimPrefix(defaultListen, "0.0.0.0:")))
}

// Deployment input is resolved only at the executable boundary. Explicit flags
// override environment defaults. Secret values never enter argv or diagnostics.
func serverSettings(args []string, stderr io.Writer) (remoteserver.Settings, bool, error) {
	flags := flag.NewFlagSet("tokeninsights-server", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var s remoteserver.Settings
	signup, signupErr := strconv.ParseBool(envDefault("TOKENINSIGHTS_SIGNUP_ENABLED", "false"))
	flags.BoolVar(&s.SignupEnabled, "signup", signup, "allow hosted signup (TOKENINSIGHTS_SIGNUP_ENABLED; default false)")
	flags.StringVar(&s.Backend, "storage-backend", envDefault("TOKENINSIGHTS_STORAGE_BACKEND", "sqlite"), "sqlite or postgres (TOKENINSIGHTS_STORAGE_BACKEND)")
	flags.StringVar(&s.Listen, "listen", listenDefault(), "IPv4 listen address (TOKENINSIGHTS_LISTEN)")
	flags.StringVar(&s.DBPath, "server-db-path", os.Getenv("TOKENINSIGHTS_SERVER_DB_PATH"), "SQLite token path (TOKENINSIGHTS_SERVER_DB_PATH)")
	flags.StringVar(&s.AppDBPath, "app-db-path", os.Getenv("TOKENINSIGHTS_APP_DB_PATH"), "SQLite account path (TOKENINSIGHTS_APP_DB_PATH; default beside token database)")
	flags.StringVar(&s.PublicURL, "public-url", os.Getenv("TOKENINSIGHTS_PUBLIC_URL"), "optional browser origin (TOKENINSIGHTS_PUBLIC_URL)")
	flags.StringVar(&s.AdminSocket, "admin-socket", os.Getenv("TOKENINSIGHTS_ADMIN_SOCKET"), "private absolute socket path (TOKENINSIGHTS_ADMIN_SOCKET)")
	trusted := flags.String("trusted-proxies", os.Getenv("TOKENINSIGHTS_TRUSTED_PROXIES"), "trusted proxy CIDRs, comma-separated (TOKENINSIGHTS_TRUSTED_PROXIES)")
	version := flags.Bool("version", false, "print version")
	if err := flags.Parse(args); err != nil {
		return s, false, err
	}
	if flags.NArg() != 0 {
		return s, false, fmt.Errorf("unexpected arguments")
	}
	if *version {
		return s, true, nil
	}
	if os.Getenv("TOKENINSIGHTS_SERVER_TOKEN") != "" {
		return s, false, fmt.Errorf("TOKENINSIGHTS_SERVER_TOKEN removed; use hosted user tokens; unset it")
	}
	if os.Getenv("ADMIN_ACCOUNT") != "" || os.Getenv("ADMIN_TOKEN") != "" {
		return s, false, fmt.Errorf("ADMIN_ACCOUNT and ADMIN_TOKEN removed; use TOKENINSIGHTS_BOOTSTRAP_USERNAME and TOKENINSIGHTS_BOOTSTRAP_PASSWORD; unset old variables")
	}
	if signupErr != nil {
		overridden := false
		flags.Visit(func(f *flag.Flag) {
			if f.Name == "signup" {
				overridden = true
			}
		})
		if !overridden {
			return s, false, fmt.Errorf("invalid TOKENINSIGHTS_SIGNUP_ENABLED; use true or false")
		}
	}
	s.BootstrapUsername = os.Getenv("TOKENINSIGHTS_BOOTSTRAP_USERNAME")
	var err error
	s.BootstrapPassword, err = deploymentSecret("TOKENINSIGHTS_BOOTSTRAP_PASSWORD")
	if err != nil {
		return s, false, err
	}
	s.PostgresDSN, err = deploymentSecret("TOKENINSIGHTS_POSTGRES_DSN")
	if err != nil {
		return s, false, err
	}
	s.TrustedProxies, err = clientaddress.Parse(*trusted)
	if err != nil {
		return s, false, err
	}
	_, _, err = s.Validate()
	return s, false, err
}

func deploymentSecret(key string) (string, error) {
	value, path := os.Getenv(key), os.Getenv(key+"_FILE")
	if value != "" && path != "" {
		return "", fmt.Errorf("set only one of %s and %s_FILE", key, key)
	}
	if path == "" {
		return value, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s_FILE", key)
	}
	defer func() { _ = f.Close() }()
	const maxSecretBytes = 64 << 10
	data, err := io.ReadAll(io.LimitReader(f, maxSecretBytes+1))
	if err != nil || len(data) > maxSecretBytes {
		return "", fmt.Errorf("cannot read %s_FILE", key)
	}
	value = strings.TrimSpace(string(data))
	if key == "TOKENINSIGHTS_BOOTSTRAP_PASSWORD" {
		value = strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	}
	if value == "" {
		return "", fmt.Errorf("%s_FILE is empty", key)
	}
	return value, nil
}
