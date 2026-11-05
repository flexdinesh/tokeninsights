package localbackend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
)

func TestPrivateViewersShareLifetimeAndFinalDisconnectReleasesStorage(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	root := t.TempDir()
	settings := localprotocol.Settings{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite")}
	ready, finished := make(chan localprotocol.Descriptor, 1), make(chan error, 1)
	go func() {
		finished <- Serve(ctx, settings, io.Discard, func(d localprotocol.Descriptor) error { ready <- d; return nil })
	}()
	joined := false
	defer func() {
		cancel()
		if !joined {
			if err := <-finished; err != nil {
				t.Error(err)
			}
		}
	}()
	select {
	case descriptor := <-ready:
		body, err := json.Marshal(descriptor)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"url", "listen"} {
			if _, ok := fields[field]; ok {
				t.Fatal("backend advertised dashboard listener", field)
			}
		}
	case err := <-finished:
		joined = true
		t.Fatal("backend failed before ready", err)
	case <-ctx.Done():
		t.Fatal("backend did not become ready")
	}
	options := localclient.Options{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath}
	first, err := localclient.Connect(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := localclient.Connect(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	if first.InstanceID != second.InstanceID {
		t.Fatal("viewers selected different backend owners")
	}
	client := &http.Client{Transport: second.PrivateRoundTripper()}
	for _, route := range []string{"/api/v2/instance", "/api/v2/status", "/api/v2/usage?period=all", "/"} {
		response, err := client.Get("http://localhost" + route)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatal("private read unavailable", route, response.StatusCode)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		joined = true
		t.Fatal("closing one viewer stopped another", err)
	case <-time.After(ReconnectGrace + 100*time.Millisecond):
	}
	if _, err := second.Query.Instance(ctx); err != nil {
		t.Fatal("remaining viewer lost storage", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		joined = true
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("final viewer did not release backend")
	}
	reopened, err := localruntime.Open(ctx, settings.CollectorDBPath, settings.ServerDBPath)
	if err != nil {
		t.Fatal("final lease retained storage ownership", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}
