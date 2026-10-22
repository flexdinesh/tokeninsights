package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/operator"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/remoteserver"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "admin" {
		return runAdmin(ctx, args[1:], os.Stdin, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "healthcheck" {
		return healthcheck(ctx, args[1:], stderr)
	}
	settings, showVersion, err := serverSettings(args, stderr)
	if err != nil {
		return err
	}
	if showVersion {
		_, err := fmt.Fprintln(stdout, version.StringFor("tokeninsights-server"))
		return err
	}
	return remoteserver.Run(ctx, settings, stderr, func(url string) error { _, err := fmt.Fprintln(stdout, url); return err })
}

func runAdmin(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("tokeninsights-server admin", flag.ContinueOnError)
	flags.SetOutput(stderr)
	socket := flags.String("admin-socket", os.Getenv("TOKENINSIGHTS_ADMIN_SOCKET"), "running server private socket")
	if err := flags.Parse(args); err != nil {
		return err
	}
	command := flags.Args()
	if len(command) < 3 {
		return fmt.Errorf("use admin --admin-socket PATH user create|disable|reprocess|reset-password VALUE or token create|revoke ID")
	}
	request := operator.Request{}
	switch command[0] + " " + command[1] {
	case "user create":
		if len(command) != 3 {
			return fmt.Errorf("user create requires username")
		}
		request.Operation = "create-user"
		request.DisplayName = command[2]
	case "user reset-password":
		if len(command) != 3 {
			return fmt.Errorf("user reset-password requires user ID; password on stdin")
		}
		request.Operation = "reset-password"
		request.UserID = command[2]
		const maxPasswordBytes = accounts.MaxPasswordCharacters * utf8.UTFMax
		data, err := io.ReadAll(io.LimitReader(stdin, maxPasswordBytes+3))
		if err != nil {
			return fmt.Errorf("cannot read password from stdin")
		}
		request.Password = strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
		if request.Password == "" || len(request.Password) > maxPasswordBytes {
			return fmt.Errorf("invalid password input on stdin")
		}
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
	return operator.Call(ctx, *socket, request, stdout)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
