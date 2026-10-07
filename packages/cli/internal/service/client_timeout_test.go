package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func privateTestClient(t *testing.T, handler http.Handler) Client {
	t.Helper()
	directory, err := os.MkdirTemp("", "ti-client-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	path := filepath.Join(directory, "socket")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return Client{Record: Record{Socket: path, InstanceID: "expected-instance"}}
}

func TestAcceptanceOutlivesControlTimeout(t *testing.T) {
	t.Parallel()
	client := privateTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TokenInsights-Instance") != "expected-instance" {
			t.Error("acceptance lost instance verification")
		}
		timer := time.NewTimer(callTimeout + 100*time.Millisecond)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	ctx, cancel := context.WithTimeout(t.Context(), callTimeout+time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://local/api/v3/ingestion/batches", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.IngestionClient().Do(request)
	if err != nil {
		t.Fatal("slow durable acceptance inherited control deadline", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted {
		t.Fatal(response.StatusCode)
	}
}

func TestControlRetainsShortTimeout(t *testing.T) {
	t.Parallel()
	client := privateTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TokenInsights-Instance") != "expected-instance" {
			t.Error("control lost instance verification")
		}
		<-r.Context().Done()
	}))
	if _, err := client.Status(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("control did not keep short deadline", err)
	}
}

func TestAcceptanceCancellationAndRedirectFence(t *testing.T) {
	client := privateTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://foreign.invalid/", http.StatusTemporaryRedirect)
			return
		}
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://local/api/v3/ingestion/batches", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.IngestionClient().Do(request); !errors.Is(err, context.Canceled) {
		t.Fatal("acceptance ignored cancellation", err)
	}
	response, err := client.IngestionClient().Get("http://local/redirect")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusTemporaryRedirect {
		t.Fatal("acceptance followed redirect", response.StatusCode)
	}
}
