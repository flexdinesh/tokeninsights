package cli

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
	"io"
	"os"
	"sync"
)

type debugEvent struct {
	stage             string
	accepted, pending int64
	complete          bool
	err               error
}
type debugSyncModel struct {
	ctx      context.Context
	cancel   context.CancelFunc
	store    *syncjob.Store
	job      syncjob.Job
	token    string
	messages chan debugEvent
	workers  *sync.WaitGroup
	event    debugEvent
}

func (m debugSyncModel) Init() tea.Cmd {
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		send := func(event debugEvent) {
			select {
			case m.messages <- event:
			default:
			}
		}
		result, err := syncjob.RunRemote(m.ctx, m.store, m.job, m.token, true, syncjob.Progress{
			Collection: func(event pipeline.SyncProgressEvent) {
				send(debugEvent{stage: "Collecting / parsing " + string(event.Harness)})
			},
			Delivery: func(event collector.DeliveryProgress) {
				send(debugEvent{stage: "Submitting", accepted: event.Accepted, pending: event.Pending})
			},
			Processing: func(pending int64) { send(debugEvent{stage: "Processing accepted receipts", pending: pending}) },
		})
		select {
		case m.messages <- debugEvent{stage: "Complete", accepted: result.Accepted, complete: true, err: err}:
		case <-m.ctx.Done():
		}
	}()
	return m.read()
}
func (m debugSyncModel) read() tea.Cmd {
	return func() tea.Msg {
		select {
		case event := <-m.messages:
			return event
		case <-m.ctx.Done():
			return debugEvent{complete: true, err: m.ctx.Err()}
		}
	}
}
func (m debugSyncModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch event := message.(type) {
	case tea.KeyMsg:
		if event.String() == "q" || event.Type == tea.KeyCtrlC {
			m.cancel()
			return m, tea.Quit
		}
	case debugEvent:
		if event.stage == "Processing accepted receipts" {
			event.accepted = m.event.accepted
		}
		m.event = event
		if event.complete {
			return m, tea.Quit
		}
		return m, m.read()
	}
	return m, nil
}
func (m debugSyncModel) View() string {
	status := m.event.stage
	if status == "" {
		status = "Connecting"
	}
	if m.event.err != nil {
		status = "Failed: " + syncjob.ErrorCode(m.event.err)
	}
	return fmt.Sprintf("Sync · %s\nAccepted: %d   Pending: %d\nq Quit\n", status, m.event.accepted, m.event.pending)
}
func runDebugSync(ctx context.Context, invocation commandInvocation, store *syncjob.Store, job syncjob.Job, token string) error {
	file, ok := invocation.stderr.(*os.File)
	if !ok || !term.IsTerminal(file.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		terminal := newTerminalSyncProgress(invocation.stderr)
		last := int64(-1)
		result, err := syncjob.RunRemote(ctx, store, job, token, true, syncjob.Progress{Collection: terminal.Collection, Delivery: terminal.Delivery, Processing: func(pending int64) {
			if pending != last {
				_, _ = fmt.Fprintf(invocation.stderr, "Receipt processing: %d pending.\n", pending)
				last = pending
			}
		}})
		terminal.Finish(result)
		return err
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	model := debugSyncModel{ctx: run, cancel: cancel, store: store, job: job, token: token, messages: make(chan debugEvent, 32), workers: &sync.WaitGroup{}}
	defer func() { cancel(); model.workers.Wait() }()
	final, err := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(io.Writer(file))).Run()
	if err != nil {
		return err
	}
	if result, ok := final.(debugSyncModel); ok {
		return result.event.err
	}
	return nil
}
