package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Known test children must also be cleaned up when a lifecycle regression fails.
func cleanupStartedDaemon(t *testing.T, record Record) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if Stop(ctx, record.Config.DBPath) == nil {
			return
		}
		process, err := os.FindProcess(record.PID)
		if err != nil {
			t.Error(err)
			return
		}
		_ = process.Signal(syscall.SIGTERM)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			file, held, err := lifetime(record.Config.DBPath, false)
			if file != nil {
				_ = file.Close()
			}
			if err != nil || !held {
				return
			}
			select {
			case <-ctx.Done():
				t.Error("test daemon cleanup timed out")
				return
			case <-ticker.C:
			}
		}
	})
}

func authenticatedRequest(t *testing.T, method, target, token string, body []byte) (int, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, payload
}

func TestRunningStartChecksRuntimeTokenAndRestartRotatesWithoutLosingReceipts(t *testing.T) {
	options := environment(t)
	host, oldToken, newToken := "0.0.0.0", "synthetic-old-token", "synthetic-new-token"
	options.Host, options.Token = &host, &oldToken
	state, err := Ensure(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	cleanupStartedDaemon(t, *state.Record)
	batch := savedBatch(t, state.Status.DataEpoch)
	status, receipt := authenticatedRequest(t, http.MethodPost, state.Record.URL+"/api/v1/ingestion/batches", oldToken, batch)
	if status != http.StatusOK {
		t.Fatalf("initial ingestion: %d %s", status, receipt)
	}
	// Runtime memory, rather than an edited saved config, governs token reuse.
	files, err := servicePaths(state.Record.Config.DatabaseKey, false)
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := readFile(files.config, &config); err != nil {
		t.Fatal(err)
	}
	config.Token = newToken
	if err := atomicFile(files.config, config); err != nil {
		t.Fatal(err)
	}
	for name, token := range map[string]*string{"same": &oldToken, "omitted": nil} {
		again, err := Ensure(t.Context(), Options{DBPath: options.DBPath, Token: token})
		if err != nil || again.Record.InstanceID != state.Record.InstanceID {
			t.Fatalf("%s token replaced/rejected running daemon: %v", name, err)
		}
	}
	emptyToken := ""
	for name, token := range map[string]*string{"different": &newToken, "empty": &emptyToken} {
		_, err := Ensure(t.Context(), Options{DBPath: options.DBPath, Token: token})
		if err == nil || !strings.Contains(err.Error(), "restart") {
			t.Errorf("%s token change silently accepted: %v", name, err)
		} else if strings.Contains(err.Error(), oldToken) || strings.Contains(err.Error(), newToken) {
			t.Fatal("token disclosed in mismatch error")
		}
	}
	if status, _ := authenticatedRequest(t, http.MethodGet, state.Record.URL+"/api/v1/instance", oldToken, nil); status != http.StatusOK {
		t.Fatal("rejected change invalidated active token")
	}
	if status, _ := authenticatedRequest(t, http.MethodGet, state.Record.URL+"/api/v1/instance", newToken, nil); status != http.StatusUnauthorized {
		t.Fatal("rejected change authorized new token")
	}
	restarted, err := Restart(t.Context(), Options{DBPath: options.DBPath, Token: &newToken})
	if err != nil {
		t.Fatal(err)
	}
	cleanupStartedDaemon(t, *restarted.Record)
	if restarted.Record.InstanceID == state.Record.InstanceID || restarted.Status.DataEpoch != state.Status.DataEpoch {
		t.Fatal("rotation failed to preserve database identity or replace daemon")
	}
	if status, _ := authenticatedRequest(t, http.MethodGet, restarted.Record.URL+"/api/v1/instance", oldToken, nil); status != http.StatusUnauthorized {
		t.Fatal("old token remains authorized after restart")
	}
	status, replay := authenticatedRequest(t, http.MethodPost, restarted.Record.URL+"/api/v1/ingestion/batches", newToken, batch)
	if status != http.StatusOK || !bytes.Equal(replay, receipt) {
		t.Fatal("rotation changed committed replay receipt", status)
	}
}

func TestRunningAnonymousStartRejectsAddingToken(t *testing.T) {
	options := environment(t)
	state, err := Ensure(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	cleanupStartedDaemon(t, *state.Record)
	token := "synthetic-added-token"
	if _, err := Ensure(t.Context(), Options{DBPath: options.DBPath, Token: &token}); err == nil {
		t.Fatal("adding token to running anonymous daemon silently accepted")
	}
	empty := ""
	if again, err := Ensure(t.Context(), Options{DBPath: options.DBPath, Token: &empty}); err != nil || again.Record.InstanceID != state.Record.InstanceID {
		t.Fatal("explicit empty token did not reuse anonymous daemon", err)
	}
	if status, _ := authenticatedRequest(t, http.MethodGet, state.Record.URL+"/api/v1/instance", "", nil); status != http.StatusOK {
		t.Fatal("rejected change altered active authentication")
	}
}

func TestPrivateTokenComparisonIsOwnerGuardedAndDisclosesOnlyBoolean(t *testing.T) {
	options := environment(t)
	token := "synthetic-private-runtime-token"
	options.Token = &token
	state, err := Ensure(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	cleanupStartedDaemon(t, *state.Record)
	client := Client{Record: *state.Record}
	for _, candidate := range []string{token, "synthetic-other-token", ""} {
		var reply map[string]bool
		if err := client.call(t.Context(), http.MethodPost, "/token-match", map[string]string{"token": candidate}, &reply); err != nil {
			t.Fatal(err)
		}
		if len(reply) != 1 || reply["matches"] != (candidate == token) {
			t.Fatal("token comparison reply includes more than the correct boolean", reply)
		}
	}
	client.Record.InstanceID = instanceID()
	if err := client.call(t.Context(), http.MethodPost, "/token-match", map[string]string{"token": token}, &struct{}{}); err == nil {
		t.Fatal("unverified instance compared runtime token")
	}
	client.Record.InstanceID = state.Record.InstanceID
	for _, request := range []any{struct{}{}, map[string]any{"token": nil}, map[string]string{"token": token, "private": token}} {
		var reply map[string]bool
		if err := client.call(t.Context(), http.MethodPost, "/token-match", request, &reply); err == nil {
			t.Fatal("invalid token comparison request accepted")
		} else if strings.Contains(err.Error(), token) {
			t.Fatal("token disclosed in validation error")
		}
	}
	for _, endpoint := range []string{"/api/v1/instance", "/api/v1/token-match"} {
		_, payload := authenticatedRequest(t, http.MethodGet, state.Record.URL+endpoint, token, nil)
		if bytes.Contains(payload, []byte(token)) || bytes.Contains(payload, []byte("matches")) {
			t.Fatal("public response exposed private authentication configuration")
		}
	}
	encoded, err := json.Marshal(state)
	if err != nil || bytes.Contains(encoded, []byte(token)) {
		t.Fatal("discovery exposed token", err)
	}
}

func TestStopVerifiedDaemonDespiteMissingOrMalformedDatabase(t *testing.T) {
	for _, missing := range []bool{true, false} {
		t.Run(map[bool]string{true: "missing", false: "malformed"}[missing], func(t *testing.T) {
			options := environment(t)
			state, err := Ensure(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			cleanupStartedDaemon(t, *state.Record)
			if missing {
				err = os.Remove(options.DBPath)
			} else {
				err = os.WriteFile(options.DBPath, []byte("synthetic-malformed-database"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Probe(t.Context(), options.DBPath); err == nil {
				t.Fatal("unhealthy daemon reported ready")
			}
			if err := Stop(t.Context(), options.DBPath); err != nil {
				t.Fatal("storage health blocked verified shutdown", err)
			}
			stopped, err := Probe(t.Context(), options.DBPath)
			if err != nil || stopped.Running {
				t.Fatal("shutdown retained lifetime ownership", err)
			}
		})
	}
}

func TestRestartMissingDatabaseCreatesFreshAndMalformedDatabaseRemainsUntouched(t *testing.T) {
	for _, missing := range []bool{true, false} {
		t.Run(map[bool]string{true: "missing", false: "malformed"}[missing], func(t *testing.T) {
			options := environment(t)
			state, err := Ensure(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			cleanupStartedDaemon(t, *state.Record)
			malformed := []byte("synthetic-malformed-restart-database")
			if missing {
				err = os.Remove(options.DBPath)
			} else {
				err = os.WriteFile(options.DBPath, malformed, 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			restarted, restartErr := Restart(t.Context(), Options{DBPath: options.DBPath})
			if missing {
				if restartErr != nil {
					t.Fatal("missing database restart failed", restartErr)
				}
				cleanupStartedDaemon(t, *restarted.Record)
				if restarted.Status.DataEpoch == state.Status.DataEpoch || restarted.Record.InstanceID == state.Record.InstanceID {
					t.Fatal("replacement storage reused old database/instance identity")
				}
			} else {
				if restartErr == nil {
					t.Fatal("malformed storage restarted successfully")
				}
				if contents, err := os.ReadFile(options.DBPath); err != nil || !bytes.Equal(contents, malformed) {
					t.Fatal("restart changed malformed storage", err)
				}
				stopped, err := Probe(t.Context(), options.DBPath)
				if err != nil || stopped.Running {
					t.Fatal("rejected restart retained unhealthy old owner", err)
				}
			}
		})
	}
}

func TestStopDoesNotTakeOverHeldUnreachableOwnership(t *testing.T) {
	options := environment(t)
	owner, held, err := lifetime(options.DBPath, true)
	if err != nil || held {
		t.Fatal("acquire test ownership", err)
	}
	defer func() { _ = owner.Close() }()
	if err := Stop(t.Context(), options.DBPath); err == nil {
		t.Fatal("unreachable held ownership treated as safely stopped")
	}
	other, stillHeld, err := lifetime(options.DBPath, false)
	if other != nil {
		_ = other.Close()
	}
	if err != nil || !stillHeld {
		t.Fatal("unknown owner's lifetime lock changed", err)
	}
	// Release before the environment helper's cleanup probes the test path.
	if err := owner.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
}

func TestStopRejectsUnverifiedDiscoveryDespiteHeldLifetimeLock(t *testing.T) {
	for _, protocolMismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "wrong-instance", true: "wrong-protocol"}[protocolMismatch], func(t *testing.T) {
			options := environment(t)
			state, err := Ensure(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			cleanupStartedDaemon(t, *state.Record)
			files, err := servicePaths(state.Record.Config.DatabaseKey, false)
			if err != nil {
				t.Fatal(err)
			}
			// Restore both discovery records before any daemon cleanup.
			t.Cleanup(func() {
				for _, path := range []string{files.record, files.fallbackRecord} {
					if err := atomicFile(path, state.Record); err != nil {
						t.Error(err)
					}
				}
			})
			forged := *state.Record
			if protocolMismatch {
				forged.Protocol++
			} else {
				forged.InstanceID = instanceID()
			}
			for _, path := range []string{files.record, files.fallbackRecord} {
				if err := atomicFile(path, forged); err != nil {
					t.Fatal(err)
				}
			}
			if err := Stop(t.Context(), options.DBPath); err == nil {
				t.Fatal("shutdown accepted unverified discovery identity")
			}
			if status, _ := authenticatedRequest(t, http.MethodGet, state.Record.URL+"/api/v1/instance", "", nil); status != http.StatusOK {
				t.Fatal("unverified discovery stopped actual owner")
			}
		})
	}
}
