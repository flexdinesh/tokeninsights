package remoteserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/accounts"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func hostedRequest(t *testing.T, url, token, method string, body []byte) (int, []byte) {
	t.Helper()
	r, err := http.NewRequestWithContext(t.Context(), method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}

func hostedAdmin(t *testing.T, socket string, request accounts.AdminRequest, result interface{}) {
	t.Helper()
	var output bytes.Buffer
	if err := accounts.AdminCall(t.Context(), socket, request, &output); err != nil {
		t.Fatal(err)
	}
	if result != nil {
		if err := json.Unmarshal(output.Bytes(), result); err != nil {
			t.Fatal(err)
		}
	}
}

func startHosted(t *testing.T, settings Settings) (string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- Run(ctx, settings, io.Discard, func(url string) error { ready <- url; return nil }) }()
	var target string
	select {
	case target = <-ready:
	case err := <-done:
		cancel()
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("hosted startup timeout")
	}
	var once sync.Once
	return target, func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(10 * time.Second):
				t.Error("hosted shutdown timeout")
			}
		})
	}
}

func TestHostedSharedDatabaseIsolationAndProvisioning(t *testing.T) {
	root := t.TempDir()
	socket := filepath.Join(root, "admin.sock")
	settings := Settings{Listen: "127.0.0.1:0", DBPath: filepath.Join(root, "server.duckdb"), PublicURL: "https://usage.example", AdminSocket: socket}
	target, stop := startHosted(t, settings)
	t.Cleanup(stop)
	info, err := os.Stat(socket)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("operator socket permissions", info, err)
	}
	for _, path := range []string{"/healthz", "/readyz", "/"} {
		if code, body := hostedRequest(t, target+path, "", "GET", nil); code != http.StatusOK {
			t.Fatal(path, code, string(body))
		}
	}
	if code, _ := hostedRequest(t, target+"/api/v2/instance", "", "GET", nil); code != http.StatusUnauthorized {
		t.Fatal("unauthenticated descriptor", code)
	}
	users := make([]accounts.User, 2)
	tokens := make([]accounts.Token, 2)
	caps := make([]evidence.Capabilities, 2)
	for i, name := range []string{"Alice", "Bob"} {
		hostedAdmin(t, socket, accounts.AdminRequest{Operation: "create-user", DisplayName: name}, &users[i])
		hostedAdmin(t, socket, accounts.AdminRequest{Operation: "create-token", UserID: users[i].UserID, Permissions: []string{accounts.Read, accounts.Ingest}}, &tokens[i])
		code, data := hostedRequest(t, target+"/api/v3/ingestion/capabilities", tokens[i].Secret, "GET", nil)
		if code != http.StatusOK {
			t.Fatal(code, string(data))
		}
		if err := json.Unmarshal(data, &caps[i]); err != nil {
			t.Fatal(err)
		}
		if caps[i].DatasetID != users[i].DatasetID {
			t.Fatal("auth dataset", caps[i], users[i])
		}
	}
	if caps[0].DatabaseID != caps[1].DatabaseID || caps[0].DatasetID == caps[1].DatasetID {
		t.Fatal("not one shared database with distinct datasets", caps)
	}
	for _, path := range []string{"/api/v1/instance", "/api/v2/ingestion/capabilities", "/api/v2/collector-progress", "/api/v2/processing/reprocess", "/control/v1/accounts"} {
		if code, body := hostedRequest(t, target+path, tokens[0].Secret, "GET", nil); code != http.StatusNotFound {
			t.Fatal("hosted legacy/operator/progress route", path, code, string(body))
		}
	}
	if code, _ := hostedRequest(t, target+"/api/v3/ingestion/batches/shared-stream/shared-batch", tokens[1].Secret, "GET", nil); code != http.StatusNotFound {
		t.Fatal("empty foreign receipt", code)
	}
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "shared-source", Lineage: "shared-lineage", Ordinal: 2, Data: json.RawMessage(`{"type":"message","id":"shared-message","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":100,"output":20}}}`), Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"shared-session"}`)}}}
	for i := range users {
		batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: caps[i].DatabaseID, DatasetID: users[i].DatasetID, StreamID: "shared-stream", BatchID: "shared-batch", FromSequence: 1, ToSequence: 1, Entries: []evidence.Entry{{Sequence: 1, Record: record}}}
		body, err := json.Marshal(batch)
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			code, responseBody := hostedRequest(t, target+"/api/v3/ingestion/batches", tokens[i].Secret, "POST", body)
			if code != http.StatusOK && code != http.StatusAccepted {
				t.Fatal("ingest", code, string(responseBody))
			}
			var receipt evidence.Response
			if err := json.Unmarshal(responseBody, &receipt); err != nil {
				t.Fatal(err)
			}
			if receipt.Receipt.DatasetID != users[i].DatasetID {
				t.Fatal("foreign receipt", receipt)
			}
		}
		if i == 0 {
			if code, _ := hostedRequest(t, target+"/api/v3/ingestion/batches/shared-stream/shared-batch", tokens[1].Secret, "GET", nil); code != http.StatusNotFound {
				t.Fatal("foreign receipt leaked", code)
			}
			batch.DatasetID = users[1].DatasetID
			body, err = json.Marshal(batch)
			if err != nil {
				t.Fatal(err)
			}
			if code, _ := hostedRequest(t, target+"/api/v3/ingestion/batches", tokens[0].Secret, "POST", body); code != http.StatusConflict && code != http.StatusBadRequest {
				t.Fatal("spoofed dataset accepted", code)
			}
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for i := range users {
		for {
			code, data := hostedRequest(t, target+"/api/v2/usage?period=all", tokens[i].Secret, "GET", nil)
			if code != http.StatusOK {
				t.Fatal("query", code, string(data))
			}
			var usage struct {
				DatasetID string `json:"datasetId"`
				Pending   int64  `json:"pending"`
				Summary   struct {
					Total    int64 `json:"total"`
					Sessions int64 `json:"sessions"`
				} `json:"summary"`
			}
			if err := json.Unmarshal(data, &usage); err != nil {
				t.Fatal(err)
			}
			if usage.Pending == 0 && usage.Summary.Total == 120 {
				if usage.DatasetID != users[i].DatasetID || usage.Summary.Sessions != 1 {
					t.Fatal("dataset query crossed users", usage)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("isolated totals never published", usage)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	var rebuilt struct {
		Generation int64 `json:"generation"`
	}
	hostedAdmin(t, socket, accounts.AdminRequest{Operation: "reprocess-user", UserID: users[0].UserID}, &rebuilt)
	if rebuilt.Generation < 2 {
		t.Fatal("operator reprocess did not stage generation", rebuilt)
	}
	hostedAdmin(t, socket, accounts.AdminRequest{Operation: "revoke-token", TokenID: tokens[0].TokenID}, nil)
	if code, _ := hostedRequest(t, target+"/api/v2/instance", tokens[0].Secret, "GET", nil); code != http.StatusUnauthorized {
		t.Fatal("revoked user token", code)
	}
	if code, _ := hostedRequest(t, target+"/api/v2/instance", tokens[1].Secret, "GET", nil); code != http.StatusOK {
		t.Fatal("other user token affected", code)
	}
	stop()
	target, stopRestart := startHosted(t, settings)
	t.Cleanup(stopRestart)
	if code, _ := hostedRequest(t, target+"/api/v2/instance", tokens[0].Secret, "GET", nil); code != http.StatusUnauthorized {
		t.Fatal("revocation lost on restart", code)
	}
	code, data := hostedRequest(t, target+"/api/v3/ingestion/capabilities", tokens[1].Secret, "GET", nil)
	if code != http.StatusOK {
		t.Fatal("credentials lost on restart", code, string(data))
	}
	var restart evidence.Capabilities
	if err := json.Unmarshal(data, &restart); err != nil {
		t.Fatal(err)
	}
	if restart.DatabaseID != caps[1].DatabaseID || restart.DatasetID != caps[1].DatasetID {
		t.Fatal("restart identity changed", restart)
	}
}

func TestHostedSettingsValidateBeforeMutation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "server.duckdb")
	for _, settings := range []Settings{
		{Listen: "127.0.0.1:0", DBPath: path},
		{Listen: "127.0.0.1:0", DBPath: path, PublicURL: "http://usage.example"},
		{Listen: "127.0.0.1:0", DBPath: path, PublicURL: "https://usage.example/secret"},
		{Listen: "127.0.0.1:0", DBPath: path, PublicURL: "https://token@usage.example"},
		{Listen: "127.0.0.1:0", DBPath: path, PublicURL: "https://usage.example", AdminSocket: "relative.sock"},
	} {
		if err := Run(t.Context(), settings, io.Discard, nil); err == nil {
			t.Fatal("invalid hosted settings accepted", settings)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid settings mutated storage", entries, err)
	}
}
