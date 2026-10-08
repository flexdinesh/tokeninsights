package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/remoteserver"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "admin" {
		return runAdmin(ctx, args[1:], stdout, stderr)
	}
	flags := flag.NewFlagSet("tokeninsights-server", flag.ContinueOnError)
	flags.SetOutput(stderr)
	settings := remoteserver.Settings{}
	flags.StringVar(&settings.Listen, "listen", "0.0.0.0:8765", "IPv4 listen address")
	flags.StringVar(&settings.AppDBPath, "app-db-path", "", "application SQLite (default: beside token database)")
	flags.StringVar(&settings.DBPath, "server-db-path", "", "required server DuckDB database path")
	flags.StringVar(&settings.LegacyDBPath, "legacy-server-db-path", "", "read-only SQLite baseline import into a new DuckDB")
	flags.StringVar(&settings.Kind, "kind", "hosted", "distributed server kind (hosted)")
	flags.StringVar(&settings.PublicURL, "public-url", os.Getenv("TOKENINSIGHTS_PUBLIC_URL"), "hosted canonical HTTPS public origin")
	flags.StringVar(&settings.AdminSocket, "admin-socket", "", "private operator socket (default: database path + .admin.sock)")
	showVersion := flags.Bool("version", false, "print version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		_, err := fmt.Fprintln(stdout, "tokeninsights-server "+version.Version)
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if os.Getenv("TOKENINSIGHTS_SERVER_TOKEN") != "" {
		return fmt.Errorf("TOKENINSIGHTS_SERVER_TOKEN removed; use hosted user tokens; unset it")
	}
	if settings.Kind != "hosted" {
		return fmt.Errorf("remote servers require hosted authentication; use tokeninsights web for local usage")
	}
	return remoteserver.Run(ctx, settings, stderr, func(url string) error { _, err := fmt.Fprintln(stdout, url); return err })
}

func runAdmin(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("tokeninsights-server admin", flag.ContinueOnError)
	flags.SetOutput(stderr)
	socket := flags.String("admin-socket", "", "running server private socket")
	if err := flags.Parse(args); err != nil {
		return err
	}
	command := flags.Args()
	if len(command) == 1 && command[0] == "reprocess" {
		return accounts.AdminCall(ctx, *socket, accounts.AdminRequest{Operation: "reprocess"}, stdout)
	}
	if len(command) < 3 {
		return fmt.Errorf("use admin --admin-socket PATH reprocess, user create|disable|reprocess VALUE or token create|revoke ID")
	}
	request := accounts.AdminRequest{}
	switch command[0] + " " + command[1] {
	case "user create":
		if len(command) != 3 {
			return fmt.Errorf("user create requires display name")
		}
		request.Operation = "create-user"
		request.DisplayName = command[2]
	case "user disable":
		if len(command) != 3 {
			return fmt.Errorf("user disable requires user ID")
		}
		request.Operation = "disable-user"
		request.UserID = command[2]
	case "user reprocess":
		if len(command) != 3 {
			return fmt.Errorf("user reprocess requires user ID")
		}
		request.Operation = "reprocess-user"
		request.UserID = command[2]
	case "token revoke":
		if len(command) != 3 {
			return fmt.Errorf("token revoke requires token ID")
		}
		request.Operation = "revoke-token"
		request.TokenID = command[2]
	case "token create":
		request.Operation = "create-token"
		request.UserID = command[2]
		tokenFlags := flag.NewFlagSet("token create", flag.ContinueOnError)
		tokenFlags.SetOutput(stderr)
		scopes := tokenFlags.String("scopes", "read,ingest", "comma-separated read/ingest permissions")
		expires := tokenFlags.String("expires", "", "optional RFC3339 expiration")
		if err := tokenFlags.Parse(command[3:]); err != nil {
			return err
		}
		if tokenFlags.NArg() != 0 {
			return fmt.Errorf("unexpected token arguments")
		}
		request.Permissions = strings.Split(*scopes, ",")
		if *expires != "" {
			expiry, err := time.Parse(time.RFC3339, *expires)
			if err != nil {
				return fmt.Errorf("invalid --expires")
			}
			request.ExpiresAt = &expiry
		}
	default:
		return fmt.Errorf("unknown admin operation")
	}
	return accounts.AdminCall(ctx, *socket, request, stdout)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
