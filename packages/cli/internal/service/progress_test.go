package service

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
)

func TestManagedCollectorProgressReadOnlyPublicAndEphemeral(t *testing.T) {
	options := environment(t)
	state, err := Ensure(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	client := Client{Record: *state.Record}
	if err := client.PublishCollectorProgress(t.Context(), collectorprogress.Message{Operation: "begin", AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}); err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(state.Record.URL + "/api/v2/collector-progress")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot collectorprogress.Snapshot
	err = json.NewDecoder(response.Body).Decode(&snapshot)
	_ = response.Body.Close()
	if err != nil || snapshot.InstanceID != state.Record.InstanceID || len(snapshot.Attempts) != 1 {
		t.Fatal("public progress unavailable", snapshot, err)
	}
	for _, path := range []string{"/api/v2/collector-progress", "/control/v1/collector-progress"} {
		response, err := http.Post(state.Record.URL+path, "application/json", strings.NewReader(`{"operation":"begin","attemptId":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatal("public progress writer exposed", path, response.StatusCode)
		}
	}
	restarted, err := Restart(t.Context(), Options{DBPath: options.DBPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.PublishCollectorProgress(t.Context(), collectorprogress.Message{Operation: "heartbeat", AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}); err == nil {
		t.Fatal("old publisher reached new instance")
	}
	response, err = http.Get(restarted.Record.URL + "/api/v2/collector-progress")
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewDecoder(response.Body).Decode(&snapshot)
	_ = response.Body.Close()
	if err != nil || snapshot.InstanceID != restarted.Record.InstanceID || len(snapshot.Attempts) != 0 {
		t.Fatal("progress survived service replacement", snapshot, err)
	}
}

func TestCollectorProgressPrivateInstanceVerification(t *testing.T) {
	const id = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	record := Record{InstanceID: "current-instance"}
	registry := collectorprogress.New(record.InstanceID)
	handler := controlHandlerWithProgress(record, func() {}, registry)
	for _, identity := range []string{"", "replaced-instance"} {
		request := httptest.NewRequest(http.MethodPost, "/control/v1/collector-progress", strings.NewReader(`{"operation":"begin","attemptId":"`+id+`"}`))
		request.Header.Set("X-TokenInsights-Instance", identity)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusConflict {
			t.Fatal("foreign service updated progress", response.Code)
		}
	}
	if len(registry.Snapshot().Attempts) != 0 {
		t.Fatal("unverified state stored")
	}
	request := httptest.NewRequest(http.MethodPost, "/control/v1/collector-progress", strings.NewReader(`{"operation":"begin","attemptId":"`+id+`"}`))
	request.Header.Set("X-TokenInsights-Instance", record.InstanceID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || len(registry.Snapshot().Attempts) != 1 {
		t.Fatal(response.Code)
	}
}

func TestCollectorProgressClientUsesPrivateSocketAndRejectsReplacement(t *testing.T) {
	root, err := os.MkdirTemp("", "ti-progress-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	record := Record{InstanceID: "current-instance", Socket: filepath.Join(root, "control.sock")}
	listener, err := net.Listen("unix", record.Socket)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(record.Socket, 0o600); err != nil {
		t.Fatal(err)
	}
	registry := collectorprogress.New(record.InstanceID)
	server := &http.Server{Handler: controlHandlerWithProgress(record, func() {}, registry)}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(listener) }()
	client := Client{Record: record}
	if err := client.PublishCollectorProgress(context.Background(), collectorprogress.Message{Operation: "begin", AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}); err != nil {
		t.Fatal(err)
	}
	client.Record.InstanceID = "old-instance"
	if err := client.PublishCollectorProgress(context.Background(), collectorprogress.Message{Operation: "begin", AttemptID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}); err == nil {
		t.Fatal("replaced service accepted publisher")
	}
	if len(registry.Snapshot().Attempts) != 1 {
		t.Fatal("replacement update changed registry")
	}
	info, err := os.Stat(record.Socket)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatal("control socket permissions", info.Mode())
	}
}
