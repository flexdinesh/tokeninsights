package localruntime

import (
	"context"
	"database/sql"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
	"time"
)

func (r *Runtime) runJobs(ctx context.Context, dataPath, appPath string) {
	defer close(r.jobsDone)
	ticker := time.NewTicker(syncjob.PollInterval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		r.consumeJob(ctx, dataPath, appPath)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (r *Runtime) consumeJob(ctx context.Context, dataPath, appPath string) {
	owner, held, err := r.Jobs.Acquire()
	if err != nil || held {
		return
	}
	defer func() { _ = owner.Close() }()
	job, err := r.Jobs.NextLocal(ctx, dataPath, appPath)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return
	}
	if err := r.Jobs.Start(ctx, job.ID); err != nil {
		return
	}
	attempt, cancel := context.WithTimeout(ctx, syncjob.Timeout)
	defer cancel()
	err = r.Jobs.Bind(attempt, job.ID, r.Destination.DatabaseID, r.Destination.DatasetID)
	var result collector.Result
	if err == nil {
		result, err = collector.Run(attempt, collector.Options{CollectorDBPath: job.Spec.CollectorPath, ServerDBPath: job.Spec.DataPath, Destination: r.Destination, PublishOnly: job.Spec.PublishOnly,
			AcceptedReceipt: func(receipt evidence.Receipt) error { return r.Jobs.SaveReceipt(attempt, job.ID, receipt) },
			SyncOptions:     pipeline.SyncOptions{Harnesses: job.Spec.Harnesses, SourceDir: job.Spec.SourceDir, FullRefresh: job.Spec.FullRefresh, Normalize: true, Now: time.Now()},
		})
	}
	if err == nil {
		err = r.WaitVisible(attempt)
	}
	_ = r.Jobs.Complete(job.ID, result, err)
}
