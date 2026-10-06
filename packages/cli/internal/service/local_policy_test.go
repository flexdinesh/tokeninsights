package service

import (
	"net/http"
	"testing"
)

func TestLocalWildcardKeepsIngestionPrivateAndRestartPreservesReceipt(t *testing.T) {
	options := environment(t)
	host := "0.0.0.0"
	options.Host = &host
	state, err := Ensure(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	cleanupStartedDaemon(t, *state.Record)
	batch := savedBatch(t, state.Status.DataEpoch)
	for _, endpoint := range []string{"/api/v1/ingestion/batches", "/api/v2/ingestion/batches", "/api/v2/processing/reprocess"} {
		status, _ := authenticatedRequest(t, http.MethodPost, state.Record.URL+endpoint, "", batch)
		if status != http.StatusNotFound {
			t.Fatal("public mutation exposed", endpoint, status)
		}
	}
	receipt := postBatch(t, *state.Record, batch)
	restarted, err := Restart(t.Context(), Options{DBPath: options.DBPath})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Status.DataEpoch != state.Status.DataEpoch || restarted.Record.InstanceID == state.Record.InstanceID {
		t.Fatal("restart identity", restarted)
	}
	if replay := postBatch(t, *restarted.Record, batch); replay != receipt {
		t.Fatal("restart lost receipt", replay, receipt)
	}
	client := Client{Record: *restarted.Record}
	client.Record.InstanceID = instanceID()
	if err := client.call(t.Context(), http.MethodPost, "/shutdown", nil, &struct{}{}); err == nil {
		t.Fatal("unverified owner could shut down service")
	}
	for _, endpoint := range []string{"/control/v1/shutdown", "/control/v1/token-match"} {
		status, _ := authenticatedRequest(t, http.MethodPost, restarted.Record.URL+endpoint, "", nil)
		if status == http.StatusOK || status == http.StatusAccepted {
			t.Fatal("public lifecycle endpoint", endpoint)
		}
	}
}
