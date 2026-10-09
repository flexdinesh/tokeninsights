package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"

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
func NegotiateCapabilities(ctx context.Context, destination *Destination) (evidence.Capabilities, error) {
	var capabilities evidence.Capabilities
	if destination == nil || destination.Transport == nil {
		return capabilities, failure("configuration", "invalid_destination", nil)
	}
	var err error
	capabilities, err = destination.Transport.Capabilities(ctx)
	if err != nil {
		return capabilities, err
	}
	if capabilities.ProtocolVersion != evidence.ProtocolVersion || capabilities.ExtractorVersion != evidence.ExtractorVersion || capabilities.DatabaseID == "" || !evidence.ValidDatasetID(capabilities.DatasetID) || capabilities.Completion != "acceptance" || capabilities.MaxBodyBytes != evidence.MaxBodyBytes || capabilities.MaxEntries != evidence.MaxEntries {
		return capabilities, failure("capabilities", "incompatible_server", nil)
	}
	return capabilities, nil
}

func publishRaw(ctx context.Context, options Options, result *Result) error {
	reportDeliveryProgress(options, result)

	d := options.Destination
	if d == nil || d.Transport == nil {
		return failure("configuration", "invalid_destination", nil)
	}
	identity := d.Identity
	local := d.Local
	capabilities := evidence.Capabilities{DatabaseID: d.DatabaseID, DatasetID: d.DatasetID}
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
	destination, err := store.ResolveDestination(ctx, identity, capabilities.DatabaseID, capabilities.DatasetID, local)
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
		response, err := d.Transport.Submit(ctx, saved.Batch.ProtocolVersion, saved.Request)
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
		if options.AcceptedReceipt != nil {
			var accepted evidence.Response
			if err := evidence.StrictDecode(response, &accepted); err != nil {
				return failure("receipt", "invalid_receipt", err)
			}
			if err := options.AcceptedReceipt(accepted.Receipt); err != nil {
				return failure("receipt", "receipt_observer_failed", err)
			}
		}
		result.Batches++
		result.Accepted += int64(len(saved.Batch.Entries))
		destination, err = store.ResolveDestination(ctx, identity, capabilities.DatabaseID, capabilities.DatasetID, local)
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
