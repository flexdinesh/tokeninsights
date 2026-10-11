package main

import (
	"flag"
	"fmt"
	"io"
	"os"
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

// Deployment input is resolved only at the executable boundary. Explicit flags
// override environment defaults. Secret values never enter argv or diagnostics.
func serverSettings(args []string, stderr io.Writer) (remoteserver.Settings, bool, error) {
	flags := flag.NewFlagSet("tokeninsights-server", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var s remoteserver.Settings
	flags.StringVar(&s.Backend, "storage-backend", envDefault("TOKENINSIGHTS_STORAGE_BACKEND", "sqlite"), "sqlite or postgres (TOKENINSIGHTS_STORAGE_BACKEND)")
	flags.StringVar(&s.Listen, "listen", envDefault("TOKENINSIGHTS_LISTEN", defaultListen), "IPv4 listen address (TOKENINSIGHTS_LISTEN)")
	flags.StringVar(&s.DBPath, "server-db-path", os.Getenv("TOKENINSIGHTS_SERVER_DB_PATH"), "SQLite token path (TOKENINSIGHTS_SERVER_DB_PATH)")
	flags.StringVar(&s.AppDBPath, "app-db-path", os.Getenv("TOKENINSIGHTS_APP_DB_PATH"), "SQLite account path (TOKENINSIGHTS_APP_DB_PATH; default beside token database)")
	flags.StringVar(&s.PublicURL, "public-url", os.Getenv("TOKENINSIGHTS_PUBLIC_URL"), "canonical HTTPS origin (TOKENINSIGHTS_PUBLIC_URL)")
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
	var err error
	s.PostgresDSN, err = postgresSecret()
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

func postgresSecret() (string, error) {
	value, path := os.Getenv("TOKENINSIGHTS_POSTGRES_DSN"), os.Getenv("TOKENINSIGHTS_POSTGRES_DSN_FILE")
	if value != "" && path != "" {
		return "", fmt.Errorf("set only one of TOKENINSIGHTS_POSTGRES_DSN and TOKENINSIGHTS_POSTGRES_DSN_FILE")
	}
	if path == "" {
		return value, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot read TOKENINSIGHTS_POSTGRES_DSN_FILE")
	}
	defer func() { _ = f.Close() }()
	const maxSecretBytes = 64 << 10
	data, err := io.ReadAll(io.LimitReader(f, maxSecretBytes+1))
	if err != nil || len(data) > maxSecretBytes {
		return "", fmt.Errorf("cannot read TOKENINSIGHTS_POSTGRES_DSN_FILE")
	}
	value = strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("TOKENINSIGHTS_POSTGRES_DSN_FILE is empty")
	}
	return value, nil
}
