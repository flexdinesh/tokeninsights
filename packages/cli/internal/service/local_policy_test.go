package service

import (
	"bytes"
	"net/http"
	"testing"
)

func TestLocalWildcardUnauthenticatedIngestionAndRestartPreserveReceipt(t *testing.T) {
	options := environment(t)
	host := "0.0.0.0"
	options.Host = &host
	state, err := Ensure(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	cleanupStartedDaemon(t, *state.Record)
	batch := savedBatch(t, state.Status.DataEpoch)
	status, receipt := authenticatedRequest(t, http.MethodPost, state.Record.URL+"/api/v1/ingestion/batches", "", batch)
	if status != http.StatusOK {
		t.Fatal(status, string(receipt))
	}
	restarted, err := Restart(t.Context(), Options{DBPath: options.DBPath})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Status.DataEpoch != state.Status.DataEpoch || restarted.Record.InstanceID == state.Record.InstanceID {
		t.Fatal("restart identity", restarted)
	}
	status, replay := authenticatedRequest(t, http.MethodPost, restarted.Record.URL+"/api/v1/ingestion/batches", "", batch)
	if status != http.StatusOK || !bytes.Equal(receipt, replay) {
		t.Fatal("restart lost receipt", status)
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
