package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/flexdinesh/tokeninsights/packages/core/dbpath"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/core/rawcollectorstore"
	"github.com/flexdinesh/tokeninsights/packages/core/writerlock"
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
	release, err := writerlock.Acquire(ctx, path)
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
	// Serialize one destination, without holding the capture lock over transport.
	path, err := dbpath.Canonical(options.CollectorDBPath)
	if err != nil {
		return failure("publication", "collector_database", err)
	}
	lockIdentity := identity
	if local {
		lockIdentity = "local"
	}
	releaseDelivery, err := writerlock.Acquire(ctx, path+".delivery-"+evidence.Hash([]byte(lockIdentity+"\x00"+d.DatabaseID+"\x00"+d.DatasetID)))
	if err != nil {
		return failure("publication", "collector_busy", err)
	}
	defer releaseDelivery()
	release, err := writerlock.Acquire(ctx, path)
	if err != nil {
		return failure("publication", "collector_busy", err)
	}
	store, err := rawcollectorstore.Open(ctx, path)
	release()
	if err != nil {
		return failure("publication", "collector_database", err)
	}
	defer func() { _ = store.Close() }()
	// Deferred after Close so the lookahead is cancelled and joined first.
	var lookahead *draftLookahead
	defer func() {
		if lookahead != nil {
			lookahead.wait()
		}
	}()
	draftCtx, cancelDraft := context.WithCancel(ctx)
	defer cancelDraft()
	var destination string
	var draft *rawcollectorstore.Draft
	for {
		var saved *rawcollectorstore.SavedBatch
		err := withPublicationLock(ctx, path, func() error {
			var err error
			destination, err = store.ResolveDestination(ctx, identity, capabilities.DatabaseID, capabilities.DatasetID)
			if err != nil {
				return failure("binding", "server_database_changed", err)
			}
			result.Pending, err = store.Pending(ctx, destination)
			if err != nil {
				return failure("publication", "pending_read", err)
			}
			result.PendingKnown = true
			saved, err = store.PrepareDraft(ctx, destination, draft)
			if err != nil {
				return failure("publication", "prepare_batch", err)
			}
			return nil
		})
		if err != nil {
			return err
		}
		reportDeliveryProgress(options, result)
		if saved == nil {
			return nil
		}
		// Encode the next batch while this one is accepted. Nothing is written
		// until this batch is acknowledged; preparation verifies the draft.
		lookahead = startDraft(draftCtx, store, destination, saved.Batch.ToSequence)
		response, err := d.Transport.Submit(ctx, saved.Batch.ProtocolVersion, saved.Request)
		if err != nil {
			var rejected *StageError
			if errors.As(err, &rejected) {
				rejected.BatchID = saved.Batch.BatchID
				return rejected
			}
			return &StageError{Stage: "delivery", Code: "acceptance_failed", BatchID: saved.Batch.BatchID, Cause: err}
		}
		if err := withPublicationLock(ctx, path, func() error {
			return store.AckPrepared(ctx, destination, saved, response)
		}); err != nil {
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
		draft = lookahead.wait()
		lookahead = nil
	}
}

// draftLookahead encodes the next batch concurrently with acceptance. A failed
// or stale draft is discarded; preparation then reads and reports normally.
type draftLookahead struct {
	done  chan struct{}
	draft *rawcollectorstore.Draft
}

func startDraft(ctx context.Context, store *rawcollectorstore.Store, destination string, cursor int64) *draftLookahead {
	l := &draftLookahead{done: make(chan struct{})}
	go func() {
		defer close(l.done)
		draft, err := store.Draft(ctx, destination, cursor)
		if err == nil {
			l.draft = draft
		}
	}()
	return l
}

func (l *draftLookahead) wait() *rawcollectorstore.Draft {
	<-l.done
	return l.draft
}

// Each journal operation holds a short capture lock, never across transport.
func withPublicationLock(ctx context.Context, path string, operation func() error) error {
	release, err := writerlock.Acquire(ctx, path)
	if err != nil {
		return failure("publication", "collector_busy", err)
	}
	defer release()
	return operation()
}
