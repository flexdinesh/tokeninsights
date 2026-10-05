// Package collector coordinates host-only collection and durable publication.
package collector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dbpath"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

const requestTimeout = 30 * time.Second

type Options struct {
	CollectorDBPath string
	ServerDBPath    string
	ServerURL       string
	Token           string
	PublishOnly     bool
	SyncOptions     pipeline.SyncOptions
	EnsureLocal     func(context.Context) (string, error)
}

type Result struct {
	Collection      pipeline.Summary
	Batches         int64
	Inserted        int64
	Updated         int64
	Noop            int64
	Pending         int64
	PendingKnown    bool
	CollectionError error
	DeliveryError   error
}

// StageError exposes a fixed stage/code without echoing credentials or URLs.
type StageError struct {
	Stage, Code string
	BatchID     string
	Cause       error
}

func (e *StageError) Error() string {
	value := e.Stage + ": " + e.Code
	if e.BatchID != "" {
		value += " batch=" + e.BatchID
	}
	return value
}
func (e *StageError) Unwrap() error { return e.Cause }
func failure(stage, code string, cause error) error {
	return &StageError{Stage: stage, Code: code, Cause: cause}
}

// ValidatePaths must precede creation or collection, including remote selection.
func ValidatePaths(collectorPath, serverPath string) error {
	if strings.TrimSpace(collectorPath) == "" || strings.TrimSpace(serverPath) == "" {
		return failure("configuration", "missing_database_path", nil)
	}
	first, err := dbpath.Canonical(collectorPath)
	if err != nil {
		return err
	}
	second, err := dbpath.Canonical(serverPath)
	if err != nil {
		return err
	}
	if first == second {
		return failure("configuration", "database_paths_alias", nil)
	}
	a, ae := os.Stat(first)
	b, be := os.Stat(second)
	if ae == nil && be == nil && os.SameFile(a, b) {
		return failure("configuration", "database_paths_alias", nil)
	}
	if ae != nil && !errors.Is(ae, os.ErrNotExist) {
		return ae
	}
	if be != nil && !errors.Is(be, os.ErrNotExist) {
		return be
	}
	return nil
}

func Run(ctx context.Context, options Options) (Result, error) {
	var result Result
	if err := ValidatePaths(options.CollectorDBPath, options.ServerDBPath); err != nil {
		return result, err
	}
	if strings.TrimSpace(options.ServerURL) != "" {
		if _, err := endpoint(options.ServerURL); err != nil {
			return result, err
		}
	}
	options.SyncOptions.DBPath = options.CollectorDBPath
	if !options.PublishOnly {
		result.Collection, result.CollectionError = pipeline.Sync(ctx, options.SyncOptions)
	}
	if options.SyncOptions.DryRun {
		return result, result.CollectionError
	}
	result.DeliveryError = publish(ctx, options, &result)
	return result, errors.Join(result.CollectionError, result.DeliveryError)
}

func endpoint(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", failure("configuration", "invalid_server_url", nil)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func publish(ctx context.Context, options Options, result *Result) error {
	target := options.ServerURL
	local := strings.TrimSpace(target) == ""
	if local {
		if options.EnsureLocal == nil {
			return failure("delivery", "local_server_unavailable", nil)
		}
		var err error
		target, err = options.EnsureLocal(ctx)
		if err != nil {
			return failure("delivery", "local_server_unavailable", err)
		}
	}
	target, err := endpoint(target)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: requestTimeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	body, err := request(ctx, client, target+"/api/v1/ingestion/capabilities", options.Token, nil)
	if err != nil {
		return err
	}
	capabilities, err := publication.DecodeCapabilities(body)
	if err != nil {
		return failure("capabilities", "incompatible_server", err)
	}
	release, err := db.AcquireWriterLock(ctx, options.CollectorDBPath)
	if err != nil {
		return failure("publication", "collector_busy", err)
	}
	defer release()
	database, _, err := db.CreateIfMissing(options.CollectorDBPath)
	if err != nil {
		return failure("publication", "collector_database", err)
	}
	defer func() { _ = database.Close() }()
	store := collectorstore.Store{DB: database}
	// Remote bindings pin one database for an endpoint. A local replacement gets
	// a new binding and replays retained publication rather than inheriting a cursor.
	identity := target
	if local {
		identity += "\x00" + capabilities.DatabaseID
	}
	sum := sha256.Sum256([]byte(identity))
	destinationID := hex.EncodeToString(sum[:])
	if err := store.BindDestination(ctx, destinationID, target, capabilities.DatabaseID); err != nil {
		return failure("binding", "server_database_changed", err)
	}
	result.Pending, err = store.Pending(ctx, destinationID)
	if err != nil {
		return failure("publication", "pending_read", err)
	}
	result.PendingKnown = true
	hostname, _ := os.Hostname()
	if len(hostname) > publication.MaxStringBytes {
		hostname = ""
	}
	for {
		saved, err := store.PrepareBatch(ctx, destinationID, hostname, time.Now().UnixMilli())
		if err != nil {
			return failure("publication", "prepare_batch", err)
		}
		if saved == nil {
			return nil
		}
		receiptBytes, err := request(ctx, client, target+"/api/v1/ingestion/batches", options.Token, saved.Request)
		if err != nil {
			var stage *StageError
			if errors.As(err, &stage) {
				stage.BatchID = saved.Batch.BatchID
			}
			return err
		}
		receipt, err := publication.DecodeReceipt(receiptBytes)
		if err != nil {
			return &StageError{Stage: "receipt", Code: "invalid_receipt", BatchID: saved.Batch.BatchID, Cause: err}
		}
		if err := store.Ack(ctx, destinationID, receiptBytes, time.Now().UnixMilli()); err != nil {
			return &StageError{Stage: "receipt", Code: "acknowledgement_rejected", BatchID: saved.Batch.BatchID, Cause: err}
		}
		result.Batches++
		result.Inserted += receipt.Inserted
		result.Updated += receipt.Updated
		result.Noop += receipt.Noop
		result.Pending, err = store.Pending(ctx, destinationID)
		if err != nil {
			return failure("publication", "pending_read", err)
		}
	}
}

func request(ctx context.Context, client *http.Client, target, token string, body []byte) ([]byte, error) {
	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, failure("delivery", "invalid_request", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, failure("delivery", "transport_failed", err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, publication.MaxBodyBytes+1))
	if err != nil {
		return nil, failure("delivery", "response_failed", err)
	}
	if len(data) > publication.MaxBodyBytes {
		return nil, failure("delivery", "response_too_large", nil)
	}
	if response.StatusCode != http.StatusOK {
		var responseError publication.ErrorResponse
		if json.Unmarshal(data, &responseError) == nil && safeServerCode(responseError.Code) && safeServerStage(responseError.Stage) {
			return nil, failure(responseError.Stage, responseError.Code, nil)
		}
		return nil, failure("delivery", fmt.Sprintf("http_%d", response.StatusCode), nil)
	}
	return data, nil
}

func safeServerCode(code string) bool {
	switch code {
	case "busy", "body_limit", "body_read_failed", "invalid_request", "invalid_identity", "invalid_tokens", "invalid_revision", "incompatible", "database_mismatch", "batch_conflict", "revision_limit", "transaction_failed", "fact_conflict", "revision_conflict", "reference_conflict", "aggregate_limit":
		return true
	}
	return false
}
func safeServerStage(stage string) bool {
	switch stage {
	case "admission", "validation", "identity", "receipt", "database", "facts", "references":
		return true
	}
	return false
}
