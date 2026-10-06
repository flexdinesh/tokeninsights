package collector

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/rawcollectorstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

func captureRaw(ctx context.Context, options Options) (pipeline.Summary, error) {
	path := options.CollectorDBPath
	if options.SyncOptions.DryRun {
		directory, err := os.MkdirTemp("", "tokeninsights-dry-run-*")
		if err != nil {
			return pipeline.Summary{}, err
		}
		defer func() { _ = os.RemoveAll(directory) }()
		path = filepath.Join(directory, "collector.sqlite")
	}
	release, err := db.AcquireWriterLock(ctx, path)
	if err != nil {
		return pipeline.Summary{}, err
	}
	defer release()
	store, err := rawcollectorstore.Open(ctx, path)
	if err != nil {
		return pipeline.Summary{}, err
	}
	defer func() { _ = store.Close() }()
	return pipeline.Extract(ctx, options.SyncOptions, store)
}

func publishRaw(ctx context.Context, options Options, result *Result) error {
	reportDeliveryProgress(options, result)
	target := options.ServerURL
	local := strings.TrimSpace(target) == ""
	client := &http.Client{Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if local {
		if options.EnsureLocal == nil {
			return failure("delivery", "local_server_unavailable", nil)
		}
		if _, err := options.EnsureLocal(ctx); err != nil {
			return failure("delivery", "local_server_unavailable", err)
		}
		if options.LocalClient != nil {
			client = options.LocalClient
		} else {
			state, err := service.Probe(ctx, options.ServerDBPath)
			if err != nil || state.Record == nil {
				return failure("delivery", "local_server_unavailable", err)
			}
			client = (service.Client{Record: *state.Record}).IngestionClient()
		}
		target = "http://local"
	} else {
		var err error
		target, err = endpoint(target)
		if err != nil {
			return err
		}
	}
	body, err := request(ctx, client, target+"/api/v2/ingestion/capabilities", options.Token, nil)
	if err != nil {
		return err
	}
	var capabilities evidence.Capabilities
	if evidence.StrictDecode(body, &capabilities) != nil || capabilities.ProtocolVersion != evidence.ProtocolVersion || capabilities.ExtractorVersion != evidence.ExtractorVersion || capabilities.DatabaseID == "" || capabilities.DatasetID != "default" || capabilities.Completion != "acceptance" || capabilities.MaxBodyBytes != evidence.MaxBodyBytes || capabilities.MaxEntries != evidence.MaxEntries {
		return failure("capabilities", "incompatible_server", nil)
	}
	if err := flushLegacy(ctx, options, result, target, client, local); err != nil {
		return err
	}
	release, err := db.AcquireWriterLock(ctx, options.CollectorDBPath)
	if err != nil {
		return failure("publication", "collector_busy", err)
	}
	defer release()
	store, err := rawcollectorstore.Open(ctx, options.CollectorDBPath)
	if err != nil {
		return failure("publication", "collector_database", err)
	}
	defer func() { _ = store.Close() }()
	identity := target
	if local {
		identity += "\x00" + capabilities.DatabaseID
	}
	destination := evidence.Hash([]byte(identity))
	if err := store.Bind(ctx, destination, target, capabilities.DatabaseID); err != nil {
		return failure("binding", "server_database_changed", err)
	}
	result.Pending, err = store.Pending(ctx, destination)
	if err != nil {
		return failure("publication", "pending_read", err)
	}
	result.PendingKnown = true
	reportDeliveryProgress(options, result)
	for {
		saved, err := store.Prepare(ctx, destination)
		if err != nil {
			return failure("publication", "prepare_batch", err)
		}
		if saved == nil {
			return nil
		}
		response, err := request(ctx, client, target+"/api/v2/ingestion/batches", options.Token, saved.Request)
		if err != nil {
			return &StageError{Stage: "delivery", Code: "acceptance_failed", BatchID: saved.Batch.BatchID, Cause: err}
		}
		if err := store.Ack(ctx, destination, response); err != nil {
			return &StageError{Stage: "receipt", Code: "acknowledgement_rejected", BatchID: saved.Batch.BatchID, Cause: err}
		}
		result.Batches++
		result.Accepted += int64(len(saved.Batch.Entries))
		result.Pending, err = store.Pending(ctx, destination)
		if err != nil {
			return failure("publication", "pending_read", err)
		}
		reportDeliveryProgress(options, result)
	}
}

// Retained protocol-1 requests keep their original semantics/bytes. They are
// delivered as legacy baselines before raw evidence can prove replacement.
func flushLegacy(ctx context.Context, options Options, result *Result, target string, client *http.Client, local bool) error {
	release, err := db.AcquireWriterLock(ctx, options.CollectorDBPath)
	if err != nil {
		return err
	}
	store, err := rawcollectorstore.Open(ctx, options.CollectorDBPath)
	if err != nil {
		release()
		return err
	}
	var count int64
	err = store.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM publication_journal").Scan(&count)
	_ = store.Close()
	release()
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	legacy := options
	legacy.LocalClient = client
	if local {
		legacy.ServerURL = ""
		legacy.EnsureLocal = func(context.Context) (string, error) { return target, nil }
	} else {
		legacy.ServerURL = target
	}
	return publishLegacy(ctx, legacy, result)
}
