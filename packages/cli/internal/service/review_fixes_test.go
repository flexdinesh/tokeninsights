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

func TestLegacySavedAuthenticationRequiresExplicitRestartAndPreservesReceipt(t *testing.T) {
	options := environment(t)
	state, err := Ensure(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	cleanupStartedDaemon(t, *state.Record)
	batch := savedBatch(t, state.Status.DataEpoch)
	receipt := postBatch(t, state.Record.URL, batch)
	if err := Stop(t.Context(), options.DBPath); err != nil {
		t.Fatal(err)
	}
	files, err := servicePaths(state.Record.Config.DatabaseKey, false)
	if err != nil {
		t.Fatal(err)
	}
	legacy := state.Record.Config
	legacy.Version = 2
	legacy.Token = "synthetic-legacy-token"
	legacy.Host = "0.0.0.0"
	if err := atomicFile(files.config, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(t.Context(), Options{DBPath: options.DBPath}); err == nil || !strings.Contains(err.Error(), "restart") {
		t.Fatal("legacy auth silently dropped", err)
	}
	restarted, err := Restart(t.Context(), Options{DBPath: options.DBPath})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Status.DataEpoch != state.Status.DataEpoch || restarted.Record.Config.Host != "0.0.0.0" || restarted.Record.Config.Token != "" {
		t.Fatal("legacy upgrade changed history or bind", restarted)
	}
	if replay := postBatch(t, restarted.Record.URL, batch); replay != receipt {
		t.Fatal("upgrade changed receipt")
	}
	var saved Config
	if err := readFile(files.config, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Token != "" || saved.Version != 3 {
		t.Fatal("legacy credentials retained")
	}
}

func TestRunningLocalRejectsRetiredTokenOptionsWithoutReplacingOwner(t *testing.T) {
	options := environment(t)
	state, err := Ensure(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	cleanupStartedDaemon(t, *state.Record)
	for _, token := range []string{"synthetic-added-token", ""} {
		if _, err := Ensure(t.Context(), Options{DBPath: options.DBPath, Token: &token}); err == nil {
			t.Fatal("retired token accepted")
		}
	}
	again, err := Ensure(t.Context(), Options{DBPath: options.DBPath})
	if err != nil || again.Record.InstanceID != state.Record.InstanceID {
		t.Fatal("rejected token replaced owner", err)
	}
}

func TestPrivateTokenComparisonRemovedAndOwnerGuardPreserved(t *testing.T) {
	options := environment(t)
	state, err := Ensure(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	cleanupStartedDaemon(t, *state.Record)
	client := Client{Record: *state.Record}
	if err := client.call(t.Context(), http.MethodPost, "/token-match", map[string]string{"token": "synthetic-token"}, &struct{}{}); err == nil {
		t.Fatal("retired token endpoint accepted")
	}
	client.Record.InstanceID = instanceID()
	if err := client.call(t.Context(), http.MethodPost, "/shutdown", nil, &struct{}{}); err == nil {
		t.Fatal("unverified owner shut down server")
	}
	encoded, err := json.Marshal(state)
	if err != nil || bytes.Contains(encoded, []byte("synthetic-token")) {
		t.Fatal("discovery disclosed credentials", err)
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
