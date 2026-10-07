// Package collector coordinates host-only collection and durable publication.
package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dbpath"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

const requestTimeout = 30 * time.Second
const maxRetryAfter = 24 * time.Hour

// Destination is resolved by command composition, before any source capture.
// Identity is the canonical endpoint; URL may address a private local transport.
type Destination struct {
	URL        string
	Identity   string
	DatabaseID string
	DatasetID  string
	Client     *http.Client
	Local      bool
}

type Options struct {
	Destination      *Destination
	CollectorDBPath  string
	ServerDBPath     string
	ServerURL        string
	Token            string
	PublishOnly      bool
	SyncOptions      pipeline.SyncOptions
	EnsureLocal      func(context.Context) (string, error)
	DeliveryProgress func(DeliveryProgress)
	LocalClient      *http.Client
}

// DeliveryProgress reports acknowledged work, never estimated upload progress.
type DeliveryProgress struct {
	Accepted     int64
	Batches      int64
	Pending      int64
	PendingKnown bool
}

type Result struct {
	Accepted        int64
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
	RetryAfter  time.Duration
}

func (e *StageError) Error() string {
	value := e.Stage + ": " + e.Code
	if e.BatchID != "" {
		value += " batch=" + e.BatchID
	}
	if e.RetryAfter > 0 {
		value += " retry-after=" + e.RetryAfter.String()
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
	if strings.TrimSpace(options.CollectorDBPath) == "" {
		return result, failure("configuration", "missing_database_path", nil)
	}
	if options.Destination != nil && options.Destination.Local || options.Destination == nil && strings.TrimSpace(options.ServerURL) == "" {
		if err := ValidatePaths(options.CollectorDBPath, options.ServerDBPath); err != nil {
			return result, err
		}
	}
	if options.Destination != nil {
		destination := *options.Destination
		var err error
		destination.URL, err = endpoint(destination.URL)
		if err != nil {
			return result, err
		}
		if destination.Identity == "" {
			destination.Identity = destination.URL
		}
		destination.Identity, err = endpoint(destination.Identity)
		if err != nil {
			return result, err
		}
		if destination.DatabaseID == "" || !evidence.ValidDatasetID(destination.DatasetID) {
			return result, failure("configuration", "invalid_destination", nil)
		}
		options.Destination = &destination
	} else if strings.TrimSpace(options.ServerURL) != "" {
		if _, err := endpoint(options.ServerURL); err != nil {
			return result, err
		}
	}
	options.SyncOptions.DBPath = options.CollectorDBPath
	if !options.PublishOnly {
		result.Collection, result.CollectionError = captureRaw(ctx, options)
	}
	if options.SyncOptions.DryRun {
		return result, result.CollectionError
	}
	result.DeliveryError = publishRaw(ctx, options, &result)
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

// CanonicalEndpoint gives discovery and publication the same delivery identity.
func CanonicalEndpoint(value string) (string, error) { return endpoint(value) }

func publishLegacy(ctx context.Context, options Options, result *Result) error {
	reportDeliveryProgress(options, result)
	target := options.ServerURL
	local := strings.TrimSpace(target) == ""
	identity := ""
	if options.Destination != nil {
		target, local = options.Destination.URL, options.Destination.Local
		identity = options.Destination.Identity
	} else if local {
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
	if identity == "" {
		identity = target
	}
	identity, err = endpoint(identity)
	if err != nil {
		return err
	}
	client := deliveryClient(nil)
	if options.Destination != nil && options.Destination.Client != nil {
		client = deliveryClient(options.Destination.Client)
	} else if options.LocalClient != nil {
		client = deliveryClient(options.LocalClient)
	}
	body, err := request(ctx, client, target+"/api/v1/ingestion/capabilities", options.Token, nil)
	if err != nil {
		return err
	}
	capabilities, err := publication.DecodeCapabilities(body)
	if err != nil {
		return failure("capabilities", "incompatible_server", err)
	}
	if options.Destination != nil && (options.Destination.DatasetID != "default" || options.Destination.DatabaseID != capabilities.DatabaseID) {
		return failure("binding", "server_database_changed", nil)
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
	hostname, _ := os.Hostname()
	if len(hostname) > publication.MaxStringBytes {
		hostname = ""
	}
	destinationID, err := store.ResolveDeliveryDestination(ctx, identity, capabilities.DatabaseID, local)
	if err != nil {
		return failure("binding", "server_database_changed", err)
	}
	result.Pending, err = store.Pending(ctx, destinationID)
	if err != nil {
		return failure("publication", "pending_read", err)
	}
	result.PendingKnown = true
	reportDeliveryProgress(options, result)
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
		destinationID, err = store.ResolveDeliveryDestination(ctx, identity, capabilities.DatabaseID, local)
		if err != nil {
			return failure("binding", "server_database_changed", err)
		}
		result.Pending, err = store.Pending(ctx, destinationID)
		if err != nil {
			return failure("publication", "pending_read", err)
		}
		reportDeliveryProgress(options, result)
	}
}

func reportDeliveryProgress(options Options, result *Result) {
	if options.DeliveryProgress != nil {
		options.DeliveryProgress(DeliveryProgress{Accepted: result.Accepted, Batches: result.Batches, Pending: result.Pending, PendingKnown: result.PendingKnown})
	}
}

func deliveryClient(source *http.Client) *http.Client {
	client := http.Client{}
	if source != nil {
		client = *source
	}
	if client.Timeout <= 0 || client.Timeout > requestTimeout {
		client.Timeout = requestTimeout
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
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
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		var responseError publication.ErrorResponse
		if json.Unmarshal(data, &responseError) == nil && safeServerCode(responseError.Code) && safeServerStage(responseError.Stage) {
			return nil, &StageError{Stage: responseError.Stage, Code: responseError.Code, RetryAfter: retryDelay(response.Header.Get("Retry-After"))}
		}
		return nil, &StageError{Stage: "delivery", Code: fmt.Sprintf("http_%d", response.StatusCode), RetryAfter: retryDelay(response.Header.Get("Retry-After"))}
	}
	return data, nil
}

// Servers return bounded delta-seconds; malformed or excessive delays are ignored.
func retryDelay(value string) time.Duration {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 || seconds > int64(maxRetryAfter/time.Second) {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func safeServerCode(code string) bool {
	switch code {
	case "busy", "body_limit", "body_read_failed", "invalid_request", "invalid_identity", "invalid_tokens", "invalid_revision", "incompatible", "database_mismatch", "batch_conflict", "revision_limit", "transaction_failed", "fact_conflict", "revision_conflict", "reference_conflict", "aggregate_limit", "dataset_mismatch", "unauthorized", "forbidden", "rate_limited", "user_busy", "unavailable", "not_found":
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
