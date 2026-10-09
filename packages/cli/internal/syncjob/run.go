package syncjob

import (
	"context"
	"errors"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/clientworkflow"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

const retryAttempts = 3
const retryInterval = time.Second
const finishTimeout = 5 * time.Second

type Progress struct {
	Collection func(pipeline.SyncProgressEvent)
	Delivery   func(collector.DeliveryProgress)
	Processing func(int64)
}

func ErrorCode(err error) string {
	var stage *collector.StageError
	if errors.As(err, &stage) {
		return stage.Code
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "sync_failed"
}
func (s *Store) Complete(id string, result collector.Result, err error) error {
	state, code := "accepted", ""
	if err != nil {
		state, code = "failed", ErrorCode(err)
		if errors.Is(err, context.Canceled) {
			state = "interrupted"
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), finishTimeout)
	defer cancel()
	return s.Finish(ctx, id, state, code, result.Accepted)
}
func WaitForOwner(ctx context.Context, s *Store) (func(), error) {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		file, held, err := s.Acquire()
		if err != nil {
			return nil, err
		}
		if !held {
			return func() { _ = file.Close() }, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// RunRemote is finite. It replays retained immutable batches on retry, without
// recollecting or changing the authenticated destination binding.
func RunRemote(ctx context.Context, store *Store, job Job, token string, debug bool, progress Progress) (collector.Result, error) {
	if job.Spec.Mode != config.Distributed || job.Spec.Credential != Fingerprint(token) {
		return collector.Result{}, errors.New("sync_configuration_changed")
	}
	release, err := WaitForOwner(ctx, store)
	if err != nil {
		return collector.Result{}, err
	}
	defer release()

	for {
		current, err := store.Get(ctx, job.ID)
		if err != nil {
			return collector.Result{}, err
		}
		if current.Terminal() {
			result := collector.Result{Accepted: current.Accepted}
			if current.State != "accepted" {
				return result, errors.New(current.Error)
			}
			if debug || job.Spec.Debug {
				settings := config.Defaults()
				settings.Mode = config.Distributed
				settings.ServerURL = job.Spec.URL
				settings.ServerToken = token
				session, err := clientworkflow.Resolve(ctx, settings)
				if err != nil {
					return result, err
				}
				if err := session.Require(serverfeatures.Read); err != nil {
					return result, err
				}
				return result, waitReceipts(ctx, collector.HTTPDelivery{URL: job.Spec.URL, Token: token}, current.Receipts, progress.Processing)
			}
			return result, nil
		}
		next, err := store.NextRemote(ctx, job)
		if err != nil {
			_ = store.Complete(job.ID, collector.Result{}, err)
			return collector.Result{}, err
		}
		result, err := runRemoteClaim(ctx, store, next, token, next.Spec.Debug || (debug && next.ID == job.ID), progress)
		if next.ID == job.ID {
			return result, err
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
	}
}
func runRemoteClaim(ctx context.Context, store *Store, job Job, token string, debug bool, progress Progress) (collector.Result, error) {
	if err := store.Start(ctx, job.ID); err != nil {
		return collector.Result{}, err
	}
	settings := config.Defaults()
	settings.Mode = config.Distributed
	settings.ServerURL = job.Spec.URL
	settings.ServerToken = token
	session, err := clientworkflow.Resolve(ctx, settings)
	if err == nil {
		err = session.VerifyIngestion(ctx)
	}
	if err == nil && debug {
		err = session.Require(serverfeatures.Read)
	}
	if err == nil {
		err = store.Bind(ctx, job.ID, session.Destination.DatabaseID, session.Destination.DatasetID)
	}
	if err != nil {
		_ = store.Complete(job.ID, collector.Result{}, err)
		return collector.Result{}, err
	}
	var receipts []evidence.Receipt
	options := collector.Options{CollectorDBPath: job.Spec.CollectorPath, ServerDBPath: job.Spec.DataPath, Destination: session.Destination, PublishOnly: job.Spec.PublishOnly,
		SyncOptions: pipeline.SyncOptions{Harnesses: job.Spec.Harnesses, SourceDir: job.Spec.SourceDir, FullRefresh: job.Spec.FullRefresh, Now: time.Now(), Progress: progress.Collection}, DeliveryProgress: progress.Delivery,
		AcceptedReceipt: func(receipt evidence.Receipt) error {
			receipts = append(receipts, receipt)
			return store.SaveReceipt(ctx, job.ID, receipt)
		},
	}
	var total collector.Result
	for attempt := 0; attempt < retryAttempts; attempt++ {
		result, runErr := collector.Run(ctx, options)
		if attempt == 0 {
			total.Collection = result.Collection
		}
		total.Accepted += result.Accepted
		total.Batches += result.Batches
		total.Pending, total.PendingKnown = result.Pending, result.PendingKnown
		total.CollectionError, total.DeliveryError = result.CollectionError, result.DeliveryError
		err = runErr
		if err == nil {
			break
		}
		var stage *collector.StageError
		if !errors.As(err, &stage) || !retryable(stage.Code) || result.CollectionError != nil || attempt+1 == retryAttempts {
			break
		}
		delay := retryInterval * time.Duration(attempt+1)
		if stage.RetryAfter > delay {
			delay = stage.RetryAfter
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			err = ctx.Err()
		case <-timer.C:
		}
		if ctx.Err() != nil {
			break
		}
		options.PublishOnly = true
	}
	if err == nil && debug {
		err = waitReceipts(ctx, collector.HTTPDelivery{URL: session.URL, Token: token}, receipts, progress.Processing)
	}
	return total, errors.Join(err, store.Complete(job.ID, total, err))
}
func retryable(code string) bool {
	switch code {
	case "transport_failed", "response_failed", "acceptance_failed", "http_429", "http_500", "http_502", "http_503", "http_504", "busy", "admission_full", "user_busy", "rate_limited", "unavailable":
		return true
	}
	return false
}
func waitReceipts(ctx context.Context, delivery collector.Delivery, receipts []evidence.Receipt, progress func(int64)) error {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		var pending int64
		for _, receipt := range receipts {
			response, err := delivery.Receipt(ctx, receipt.StreamID, receipt.BatchID)
			if err != nil {
				return err
			}
			if response.Receipt != receipt {
				return errors.New("receipt_binding_changed")
			}
			pending += response.Processing.Pending
		}
		if progress != nil {
			progress(pending)
		}
		if pending == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
