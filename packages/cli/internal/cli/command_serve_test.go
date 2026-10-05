package cli

import (
	"errors"
	"flag"
	"io"
	"testing"
)

func TestServiceFlags(t *testing.T) {
	t.Setenv("TOKENINSIGHTS_SERVER_TOKEN", "")
	for _, action := range []string{"start", "restart", "run"} {
		options, err := parseServiceOptions(action, []string{"--host", "0.0.0.0", "--port", "0"}, io.Discard)
		if err != nil || options.Host == nil || *options.Host != "0.0.0.0" || options.Port == nil || *options.Port != 0 {
			t.Fatalf("flags: %+v %v", options, err)
		}
		defaults, err := parseServiceOptions(action, nil, io.Discard)
		if err != nil || defaults.Host != nil || defaults.Port != nil {
			t.Fatal("omitted settings must remain omitted")
		}
		for _, arg := range [][]string{{"--port", "65536"}, {"--host", "example.com"}, {"--week"}, {"--no-sync"}} {
			if _, err := parseServiceOptions(action, arg, io.Discard); !errors.Is(err, ErrUsage) {
				t.Fatalf("accepted %v: %v", arg, err)
			}
		}
		if _, err := parseServiceOptions(action, []string{"--help"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
			t.Fatal(err)
		}
	}
	for _, action := range []string{"status", "stop"} {
		if _, err := parseServiceOptions(action, []string{"--host", "0.0.0.0"}, io.Discard); !errors.Is(err, ErrUsage) {
			t.Fatal("host accepted by", action)
		}
	}
}

func TestServiceAuthenticationFlagsAndEnvironment(t *testing.T) {
	t.Setenv("TOKENINSIGHTS_SERVER_TOKEN", "synthetic-environment-token")
	for _, action := range []string{"start", "restart", "run"} {
		options, err := parseServiceOptions(action, nil, io.Discard)
		if err != nil || options.Token == nil || *options.Token != "synthetic-environment-token" {
			t.Fatalf("environment %s: %+v %v", action, options, err)
		}
		options, err = parseServiceOptions(action, []string{"--token", "synthetic-flag-token"}, io.Discard)
		if err != nil || options.Token == nil || *options.Token != "synthetic-flag-token" {
			t.Fatalf("flag %s: %+v %v", action, options, err)
		}
	}
}

func TestForegroundServerRejectsInvalidArguments(t *testing.T) {
	invocation := commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard}
	for _, args := range [][]string{{}, {"start"}, {"run", "--port", "65536"}, {"run", "--port", "-1"}, {"run", "--host", "example.com"}, {"run", "unexpected"}} {
		if err := runServer(invocation, args); !errors.Is(err, ErrUsage) {
			t.Fatalf("args%v: %v", args, err)
		}
	}
}
