package collector

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/rawcollectorstore"
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

// NegotiateCapabilities validates the raw wire contract before source capture.
// Client composition also compares its authenticated descriptor binding.
func NegotiateCapabilities(ctx context.Context, destination *Destination, token string) (evidence.Capabilities, error) {
	var capabilities evidence.Capabilities
	if destination == nil {
		return capabilities, failure("configuration", "invalid_destination", nil)
	}
	target, err := endpoint(destination.URL)
	if err != nil {
		return capabilities, err
	}
	body, err := request(ctx, deliveryClient(destination.Client), target+"/api/v3/ingestion/capabilities", token, nil)
	if err != nil {
		return capabilities, err
	}
	if evidence.StrictDecode(body, &capabilities) != nil || capabilities.ProtocolVersion != evidence.ProtocolVersion || capabilities.ExtractorVersion != evidence.ExtractorVersion || capabilities.DatabaseID == "" || !evidence.ValidDatasetID(capabilities.DatasetID) || capabilities.Completion != "acceptance" || capabilities.MaxBodyBytes != evidence.MaxBodyBytes || capabilities.MaxEntries != evidence.MaxEntries {
		return capabilities, failure("capabilities", "incompatible_server", nil)
	}
	return capabilities, nil
}

func publishRaw(ctx context.Context, options Options, result *Result) error {
	reportDeliveryProgress(options, result)
	target := options.ServerURL
	local := strings.TrimSpace(target) == ""
	client := deliveryClient(nil)
	var identity string
	var capabilities evidence.Capabilities
	if options.Destination != nil {
		d := options.Destination
		target, local = d.URL, d.Local
		identity = d.Identity
		if identity == "" {
			identity = target
		}
		if d.Client != nil {
			client = deliveryClient(d.Client)
		}
		capabilities = evidence.Capabilities{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: d.DatabaseID, DatasetID: d.DatasetID, Completion: "acceptance", MaxBodyBytes: evidence.MaxBodyBytes, MaxEntries: evidence.MaxEntries}
	} else {
		if local {
			if options.EnsureLocal == nil {
				return failure("delivery", "local_server_unavailable", nil)
			}
			var err error
			target, err = options.EnsureLocal(ctx)
			if err != nil {
				return failure("delivery", "local_server_unavailable", err)
			}
			target, err = endpoint(target)
			if err != nil {
				return err
			}
			identity = target
			if options.LocalClient != nil {
				client = deliveryClient(options.LocalClient)
				target = "http://local"
			}

		} else {
			var err error
			target, err = endpoint(target)
			if err != nil {
				return err
			}
		}
		if identity == "" {
			identity = target
		}
		var err error
		capabilities, err = NegotiateCapabilities(ctx, &Destination{URL: target, Client: client}, options.Token)
		if err != nil {
			return err
		}

	}
	if capabilities.DatasetID == "default" {
		if err := flushLegacy(ctx, options, result, target, identity, capabilities.DatabaseID, client, local); err != nil {
			return err
		}
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
	destination, err := store.ResolveDeliveryDestination(ctx, identity, capabilities.DatabaseID, capabilities.DatasetID, local)
	if err != nil {
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
		route := "/api/v3/ingestion/batches"
		if saved.Batch.ProtocolVersion == evidence.LegacyProtocolVersion {
			route = "/api/v2/ingestion/batches"
		}
		response, err := request(ctx, client, target+route, options.Token, saved.Request)
		if err != nil {
			var rejected *StageError
			if errors.As(err, &rejected) {
				rejected.BatchID = saved.Batch.BatchID
				return rejected
			}
			return &StageError{Stage: "delivery", Code: "acceptance_failed", BatchID: saved.Batch.BatchID, Cause: err}
		}
		if err := store.Ack(ctx, destination, response); err != nil {
			return &StageError{Stage: "receipt", Code: "acknowledgement_rejected", BatchID: saved.Batch.BatchID, Cause: err}
		}
		result.Batches++
		result.Accepted += int64(len(saved.Batch.Entries))
		destination, err = store.ResolveDeliveryDestination(ctx, identity, capabilities.DatabaseID, capabilities.DatasetID, local)
		if err != nil {
			return failure("binding", "server_database_changed", err)
		}
		result.Pending, err = store.Pending(ctx, destination)
		if err != nil {
			return failure("publication", "pending_read", err)
		}
		reportDeliveryProgress(options, result)
	}
}

// Retained protocol-1 requests keep their original semantics/bytes. They are
// delivered as legacy baselines before raw evidence can prove replacement.
func flushLegacy(ctx context.Context, options Options, result *Result, target, identity, databaseID string, client *http.Client, local bool) error {
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
	legacy.Destination = &Destination{URL: target, Identity: identity, DatabaseID: databaseID, DatasetID: "default", Local: local, Client: client}
	if local {
		legacy.ServerURL = ""
		legacy.EnsureLocal = func(context.Context) (string, error) { return target, nil }
	} else {
		legacy.ServerURL = target
	}
	return publishLegacy(ctx, legacy, result)
}
