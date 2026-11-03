package localbackend

import (
	"context"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/collector"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/core/pipeline"
)

// captures retains at most one stronger follow-up. Launches sharing the active
// source never cancel one another; a different source is rejected explicitly.
type captures struct {
	mu      sync.Mutex
	active  *localprotocol.CaptureRequest
	pending bool
	ticket  *captureTicket
	runtime *localruntime.Runtime
	ctx     context.Context
	run     func(context.Context, collector.Options) (collector.Result, error)
}

type captureTicket struct {
	done   chan struct{}
	result localprotocol.CollectionResult
}

func (c *captures) request(request localprotocol.CaptureRequest) (*captureTicket, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != nil {
		if c.active.SourceDir != request.SourceDir {
			return nil, localprotocol.ErrCaptureBusy
		}
		if request.FullRefresh && !c.active.FullRefresh {
			c.pending = true
		}
		return c.ticket, nil
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	c.active = &request
	c.ticket = &captureTicket{done: make(chan struct{})}
	first := c.start(request)
	go c.collect(request, c.ticket, first)
	return c.ticket, nil
}

func (c *captures) start(request localprotocol.CaptureRequest) <-chan localruntime.CollectionResult {
	return c.runtime.StartCollection(c.ctx, pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, Now: time.Now(), FullRefresh: request.FullRefresh, SourceDir: request.SourceDir}, c.run)
}

func (c *captures) collect(request localprotocol.CaptureRequest, ticket *captureTicket, completion <-chan localruntime.CollectionResult) {
	for {
		result := <-completion
		c.mu.Lock()
		if c.pending && c.ctx.Err() == nil {
			c.pending = false
			request.FullRefresh = true
			c.active = &request
			completion = c.start(request)
			c.mu.Unlock()
			continue
		}
		c.active = nil
		ticket.result = localprotocol.CollectionResult{Accepted: result.Result.Accepted, Batches: result.Result.Batches, Pending: result.Result.Pending, PendingKnown: result.Result.PendingKnown, Quarantined: result.Result.Collection.Quarantined, Failed: result.Result.Collection.Failed}
		ticket.result.RawFacts, ticket.result.Skipped = result.Result.Collection.RawFacts, result.Result.Collection.Skipped
		ticket.result.AttemptID = result.AttemptID
		if result.Err != nil {
			ticket.result.ErrorCode = "collection_failed"
		}
		close(ticket.done)
		c.mu.Unlock()
		return
	}
}

func (c *captures) join() {
	c.mu.Lock()
	ticket := c.ticket
	c.mu.Unlock()
	if ticket != nil {
		<-ticket.done
	}
}
