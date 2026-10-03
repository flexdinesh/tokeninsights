// Package app owns serialized mutations and shared, on-demand refresh.
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

const QueueLimit = 16
const completedLimit = 128
const completedTTL = 10 * time.Minute

var ErrClosed = errors.New("service stopping")
var ErrReset = errors.New("reset pending")
var ErrCapacity = errors.New("operation queue full")

type Action struct {
	Kind        string                 `json:"kind"`
	Harnesses   []pipeline.Harness     `json:"harnesses,omitempty"`
	Sources     *pipeline.SourceConfig `json:"sources,omitempty"`
	Normalize   bool                   `json:"normalize,omitempty"`
	FullRefresh bool                   `json:"fullRefresh,omitempty"`
	Now         time.Time              `json:"now"`
}

type Operation struct {
	requestHash string
	ID          string           `json:"id"`
	Kind        string           `json:"kind"`
	State       string           `json:"state"`
	AcceptedAt  int64            `json:"acceptedAt"`
	FinishedAt  int64            `json:"finishedAt,omitempty"`
	JobID       int64            `json:"jobId,omitempty"`
	Summary     pipeline.Summary `json:"summary"`
	Error       string           `json:"error,omitempty"`
	ErrorCode   string           `json:"errorCode,omitempty"`
}

type Status struct {
	Active           *Operation    `json:"active,omitempty"`
	InstanceID       string        `json:"instanceId"`
	DataEpoch        string        `json:"dataEpoch"`
	DataReadiness    string        `json:"dataReadiness"`
	Running          bool          `json:"running"`
	PendingRefresh   bool          `json:"pendingRefresh"`
	CheckRequestedAt int64         `json:"checkRequestedAt"`
	Phase            string        `json:"phase"`
	Error            string        `json:"error"`
	Progress         db.SyncStatus `json:"progress"`
}

type refreshAlias struct {
	id string
	at time.Time
}

type job struct {
	operation Operation
	action    Action
	ordinary  bool
	captured  bool
	cancel    context.CancelFunc
}

type Controller struct {
	aliases        map[string]refreshAlias
	path           string
	sources        *pipeline.SourceConfig
	instance       string
	log            io.Writer
	ctx            context.Context
	cancel         context.CancelFunc
	done           chan struct{}
	wake           chan struct{}
	mu             sync.Mutex
	queue          []*job
	active         *job
	completed      map[string]Operation
	completedOrder []string
	last           Operation
	closed         bool
	reset          bool
	requested      int64
	epoch          string
	readers        int
	readBlocked    bool
	changed        chan struct{}
	runner         func(context.Context, Action, func(pipeline.SyncProgressEvent), func(context.Context) error) (pipeline.Summary, error)
}

func ID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}

func New(ctx context.Context, path string, sources *pipeline.SourceConfig, instance string, log io.Writer) *Controller {
	ctx, cancel := context.WithCancel(ctx)
	c := &Controller{path: path, sources: sources, instance: instance, log: log, ctx: ctx, cancel: cancel, done: make(chan struct{}), wake: make(chan struct{}, 1), changed: make(chan struct{}), epoch: ID(), aliases: make(map[string]refreshAlias), completed: make(map[string]Operation)}
	c.runner = func(ctx context.Context, action Action, progress func(pipeline.SyncProgressEvent), before func(context.Context) error) (pipeline.Summary, error) {
		return Execute(ctx, path, action, progress, before)
	}
	go c.work()
	return c
}

func (c *Controller) notify() { close(c.changed); c.changed = make(chan struct{}) }
func (c *Controller) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Controller) prune() {
	now := time.Now().UnixMilli()
	for id, operation := range c.completed {
		if now-operation.FinishedAt > completedTTL.Milliseconds() {
			delete(c.completed, id)
		}
	}

}

func (c *Controller) RequestRefresh(ctx context.Context) (Operation, error) {
	return c.RequestRefreshID(ctx, ID())
}
func (c *Controller) RequestRefreshID(ctx context.Context, id string) (Operation, error) {
	if err := ctx.Err(); err != nil {
		return Operation{}, err
	}
	if len(id) != 32 {
		return Operation{}, fmt.Errorf("invalid request ID")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return Operation{}, fmt.Errorf("invalid request ID")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, alias := range c.aliases {
		if now.Sub(alias.at) > completedTTL {
			delete(c.aliases, key)
		}
	}
	if alias, ok := c.aliases[id]; ok {
		if o, ok := c.operationLocked(alias.id); ok {
			return o, nil
		}
		return Operation{}, fmt.Errorf("refresh outcome expired")
	}
	o, err := c.requestRefreshLocked()
	if err != nil {
		return o, err
	}
	if len(c.aliases) >= 256 {
		var oldest string
		at := now
		for key, alias := range c.aliases {
			if !alias.at.After(at) {
				oldest, at = key, alias.at
			}
		}
		delete(c.aliases, oldest)
	}
	c.aliases[id] = refreshAlias{id: o.ID, at: now}
	return o, nil
}
func (c *Controller) RefreshRequest(id string) (Operation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	alias, ok := c.aliases[id]
	if !ok || time.Since(alias.at) > completedTTL {
		return Operation{}, false
	}
	return c.operationLocked(alias.id)
}
func (c *Controller) requestRefreshLocked() (Operation, error) {
	if c.closed {
		return Operation{}, ErrClosed
	}
	if c.reset {
		return Operation{}, ErrReset
	}
	c.requested = time.Now().UnixMilli()
	if c.active != nil && c.active.ordinary && !c.active.captured {
		return c.active.operation, nil
	}
	for _, j := range c.queue {
		if j.ordinary {
			return j.operation, nil
		}
	}
	if len(c.queue) >= QueueLimit {
		return Operation{}, ErrCapacity
	}
	j := &job{ordinary: true, action: Action{Kind: "sync", Sources: c.sources, Harnesses: pipeline.SupportedHarnesses, Normalize: true}, operation: Operation{ID: ID(), Kind: "refresh", State: "queued", AcceptedAt: c.requested}}
	c.queue = append(c.queue, j)
	c.signal()
	return j.operation, nil
}

func (c *Controller) Submit(ctx context.Context, id string, action Action) (Operation, error) {
	if err := ctx.Err(); err != nil {
		return Operation{}, err
	}
	if len(id) != 32 {
		return Operation{}, fmt.Errorf("invalid request ID")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return Operation{}, fmt.Errorf("invalid request ID")
	}
	if err := Validate(action); err != nil {
		return Operation{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return Operation{}, ErrClosed
	}
	encoded, err := json.Marshal(action)
	if err != nil {
		return Operation{}, err
	}
	sum := sha256.Sum256(encoded)
	hash := hex.EncodeToString(sum[:])
	reuse := func(o Operation) (Operation, error) {
		if o.requestHash != hash {
			return Operation{}, fmt.Errorf("request ID reused with different action")
		}
		return o, nil
	}
	// IDs are scoped to this instance. Duplicate submissions never run twice.
	if c.active != nil && c.active.operation.ID == id {
		return reuse(c.active.operation)
	}
	for _, j := range c.queue {
		if j.operation.ID == id {
			return reuse(j.operation)
		}
	}
	if old, ok := c.completed[id]; ok {
		return reuse(old)
	}
	if len(c.queue) >= QueueLimit {
		return Operation{}, ErrCapacity
	}
	isReset := action.Kind == "reset-all" || action.Kind == "reset-canonical"
	if isReset {
		if c.reset {
			return Operation{}, ErrReset
		}
		c.reset = true
		kept := c.queue[:0]
		for _, j := range c.queue {
			if j.ordinary {
				c.finish(j, pipeline.Summary{}, context.Canceled)
			} else {
				kept = append(kept, j)
			}
		}
		c.queue = kept
	}
	j := &job{action: action, operation: Operation{requestHash: hash, ID: id, Kind: action.Kind, State: "queued", AcceptedAt: time.Now().UnixMilli()}}
	c.queue = append(c.queue, j)
	c.signal()
	return j.operation, nil
}

func Validate(action Action) error {
	switch action.Kind {
	case "sync", "normalize":
		if err := action.Sources.Validate(); err != nil {
			return err
		}
		if action.Kind == "sync" && len(action.Harnesses) == 0 {
			return fmt.Errorf("sync requires harnesses")
		}
		for _, h := range action.Harnesses {
			if _, ok := pipeline.AdapterFor(h); !ok {
				return fmt.Errorf("unsupported harness")
			}
		}
	case "reset-all", "reset-canonical":
	default:
		return fmt.Errorf("invalid action")
	}
	return nil
}

func (c *Controller) Operation(id string) (Operation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.operationLocked(id)
}
func (c *Controller) operationLocked(id string) (Operation, bool) {
	if c.active != nil && c.active.operation.ID == id {
		return c.active.operation, true
	}
	for _, j := range c.queue {
		if j.operation.ID == id {
			return j.operation, true
		}
	}
	c.prune()
	result, ok := c.completed[id]
	return result, ok
}

func (c *Controller) CancelOperation(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != nil && c.active.operation.ID == id {
		if c.active.ordinary {
			return fmt.Errorf("shared refresh cannot be cancelled")
		}
		c.active.cancel()
		return nil
	}
	for i, j := range c.queue {
		if j.operation.ID == id {
			if j.ordinary {
				return fmt.Errorf("shared refresh cannot be cancelled")
			}
			c.queue = append(c.queue[:i], c.queue[i+1:]...)
			c.finish(j, pipeline.Summary{}, context.Canceled)
			if j.action.Kind == "reset-all" || j.action.Kind == "reset-canonical" {
				c.reset = false
			}
			return nil
		}
	}
	return fmt.Errorf("operation not active")
}

func (c *Controller) finish(j *job, summary pipeline.Summary, err error) {
	j.operation.State = "succeeded"
	if err != nil {
		j.operation.State = "failed"
		j.operation.ErrorCode = "failed"
		j.operation.Error = "Operation failed. Inspect service logs and retry."
		if errors.Is(err, context.Canceled) {
			j.operation.State, j.operation.ErrorCode, j.operation.Error = "cancelled", "cancelled", "Operation cancelled."
		}
		for _, sentinel := range []error{db.ErrRecoveryRequired, db.ErrRebuildPending, db.ErrMetadataUpgradeRequired} {
			if errors.Is(err, sentinel) {
				j.operation.ErrorCode, j.operation.Error = "recovery", "Usage recovery incomplete. Retry with the original sources."
			}
		}
	}
	summary.Errors = nil // Errors can include private source paths and do not cross HTTP.
	j.operation.Summary = summary
	j.operation.FinishedAt = time.Now().UnixMilli()
	c.prune()
	c.completed[j.operation.ID] = j.operation
	c.completedOrder = append(c.completedOrder, j.operation.ID)
	if len(c.completedOrder) > completedLimit {
		delete(c.completed, c.completedOrder[0])
		c.completedOrder = c.completedOrder[1:]
	}
	c.last = j.operation
}

func (c *Controller) work() {
	defer close(c.done)
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.wake:
		}
		for {
			c.mu.Lock()
			if c.closed || len(c.queue) == 0 {
				c.mu.Unlock()
				break
			}
			j := c.queue[0]
			c.queue = c.queue[1:]
			c.active = j
			ctx, cancel := context.WithCancel(c.ctx)
			j.cancel = cancel
			j.operation.State = "running"
			if j.ordinary {
				j.action.Now = time.Now()
			}
			c.mu.Unlock()
			progress := func(e pipeline.SyncProgressEvent) {
				c.mu.Lock()
				defer c.mu.Unlock()
				if e.Status != pipeline.SyncProgressWaiting {
					j.captured = true
				}
				if e.JobID != 0 {
					j.operation.JobID = e.JobID
				}
			}
			summary, err := c.runner(ctx, j.action, progress, c.beforeReset)
			cancel()
			c.mu.Lock()
			c.finish(j, summary, err)
			c.active = nil
			if j.action.Kind == "reset-all" || j.action.Kind == "reset-canonical" {
				c.reset = false
			}
			c.readBlocked = false
			c.notify()
			hasRefresh := false
			for _, queued := range c.queue {
				hasRefresh = hasRefresh || queued.ordinary
			}
			if !hasRefresh {
				c.requested = 0
			}
			c.mu.Unlock()
			if err != nil {
				_, _ = fmt.Fprintf(c.log, "%s: %v\n", j.operation.Kind, err)
			}
		}
	}
}

// ReadPermit spans the whole analytics snapshot. Reset drains these permits
// before changing the epoch or writing; waiting remains context-aware.
func (c *Controller) ReadPermit(ctx context.Context) (string, func(), error) {
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return "", nil, ErrClosed
		}
		if !c.readBlocked {
			c.readers++
			epoch := c.epoch
			c.mu.Unlock()
			var once sync.Once
			return epoch, func() { once.Do(func() { c.mu.Lock(); c.readers--; c.notify(); c.mu.Unlock() }) }, nil
		}
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-changed:
		}
	}
}

func (c *Controller) beforeReset(ctx context.Context) error {
	c.mu.Lock()
	c.readBlocked = true
	c.notify()
	for c.readers > 0 {
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
		c.mu.Lock()
	}
	c.epoch = ID()
	c.mu.Unlock()
	return nil
}

func (c *Controller) Status(ctx context.Context) Status {
	c.mu.Lock()
	s := Status{InstanceID: c.instance, DataEpoch: c.epoch, Phase: "ready", CheckRequestedAt: c.requested}
	if c.active != nil {
		active := c.active.operation
		s.Active = &active
		s.Running = true
		s.Phase = "waiting"
	}
	for _, j := range c.queue {
		s.Running = true
		s.PendingRefresh = s.PendingRefresh || j.ordinary
	}
	jobID := int64(0)
	if c.active != nil {
		jobID = c.active.operation.JobID
	}
	last := c.last
	blocked := c.readBlocked
	c.mu.Unlock()
	s.DataReadiness = Readiness(ctx, c.path)
	if blocked {
		s.DataReadiness = "recovery"
	}
	s.Progress, _ = db.ReadSyncStatus(ctx, c.path)
	if !s.Running || (jobID != 0 && s.Progress.JobID == jobID) {
		s.Phase = s.Progress.Phase
		s.Error = s.Progress.Error
	} else {
		s.Progress.JobID = 0
		s.Progress.Running = false
	}
	if !s.Running && last.Error != "" {
		s.Error = last.Error
		s.Phase = "failed"
		if last.State == "cancelled" {
			s.Phase = "cancelled"
		}
	}
	if s.DataReadiness != "ready" {
		s.Phase = "rebuild_failed"
		if s.Running {
			s.Phase = "rebuilding"
		}
	}
	// Identity is sampled after the database reads. A reset beginning during
	// those reads must be visible to local TUI before/after checks.
	c.mu.Lock()
	s.DataEpoch = c.epoch
	if c.readBlocked {
		s.DataReadiness, s.Phase = "recovery", "rebuilding"
	}
	c.mu.Unlock()
	return s
}

func Readiness(ctx context.Context, path string) string {
	state, err := db.InspectCompatibility(ctx, path)
	if err != nil || !state.Exists {
		return "unavailable"
	}
	if state.ResetRequired {
		return "recovery"
	}
	if state.RebuildPending {
		return "rebuild"
	}
	if state.MigrationRequired {
		return "metadata"
	}
	return "ready"
}

func (c *Controller) Close(ctx context.Context) error {
	c.mu.Lock()
	c.closed = true
	c.cancel()
	for _, j := range c.queue {
		c.finish(j, pipeline.Summary{}, context.Canceled)
	}
	c.queue = nil
	c.notify()
	c.mu.Unlock()
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func Execute(ctx context.Context, path string, action Action, progress func(pipeline.SyncProgressEvent), before func(context.Context) error) (pipeline.Summary, error) {
	switch action.Kind {
	case "sync":
		return pipeline.Sync(ctx, pipeline.SyncOptions{DBPath: path, Harnesses: action.Harnesses, Sources: action.Sources, SourceDir: action.Sources.Override.Identity, Normalize: action.Normalize, FullRefresh: action.FullRefresh, Now: action.Now, Progress: progress, BeforeReset: before})
	case "normalize":
		return pipeline.Normalize(ctx, pipeline.NormalizeOptions{DBPath: path, Harnesses: action.Harnesses, Sources: action.Sources, Now: action.Now, Progress: progress, BeforeReset: before})
	case "reset-all":
		if before != nil {
			if err := before(ctx); err != nil {
				return pipeline.Summary{}, err
			}
		}
		return pipeline.Summary{}, db.ResetAllContext(ctx, path)
	case "reset-canonical":
		if before != nil {
			if err := before(ctx); err != nil {
				return pipeline.Summary{}, err
			}
		}
		release, err := db.AcquireWriterLock(ctx, path)
		if err != nil {
			return pipeline.Summary{}, err
		}
		defer release()
		reader, err := db.Open(path)
		if err != nil {
			return pipeline.Summary{}, err
		}
		if err := reader.Close(); err != nil {
			return pipeline.Summary{}, err
		}
		database, err := db.OpenWritable(path)
		if err != nil {
			return pipeline.Summary{}, err
		}
		defer func() { _ = database.Close() }()
		return pipeline.Summary{}, db.ResetCanonical(ctx, database)
	default:
		return pipeline.Summary{}, fmt.Errorf("invalid action")
	}
}
