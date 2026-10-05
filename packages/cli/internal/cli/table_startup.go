package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

type startupPhase string

const (
	startupServer        startupPhase = "Starting local server"
	startupCollect       startupPhase = "Checking local sessions"
	startupWaiting       startupPhase = "Waiting for another sync"
	startupPublish       startupPhase = "Publishing usage"
	startupLoad          startupPhase = "Loading saved usage"
	startupPanelWidth                 = 58
	startupMessageBuffer              = 32
)

type startupServerMsg struct{ url string }
type startupDeliveryMsg struct{ progress collector.DeliveryProgress }
type startupLoadMsg struct{}
type startupTickMsg struct{ attempt uint64 }
type startupReadyMsg struct{ data reloadMsg }
type startupFailedMsg struct {
	phase startupPhase
	err   error
}

// Startup owns collection. The dashboard continues to own only REST queries.
type startupModel struct {
	dashboard   interactiveModel
	originalURL string
	phase       startupPhase
	busy        bool
	frame       int
	err         error
	delivery    collector.DeliveryProgress
	messages    chan tea.Msg
	workers     *sync.WaitGroup
	collect     bool
	attempt     uint64
}

func newStartupModel(dashboard interactiveModel) startupModel {
	phase := startupServer
	if dashboard.options.serverURL != "" {
		phase = startupCollect
	}
	return startupModel{dashboard: dashboard, originalURL: dashboard.options.serverURL,
		phase: phase, busy: true, collect: true, attempt: 1, messages: make(chan tea.Msg, startupMessageBuffer), workers: &sync.WaitGroup{}}
}

func (m startupModel) Init() tea.Cmd {
	m.startWorker()
	return tea.Batch(readSyncProgressCmd(m.messages), startupAnimationCmd(m.attempt))
}

func startupAnimationCmd(attempt uint64) tea.Cmd {
	return tea.Tick(syncAnimationInterval, func(time.Time) tea.Msg { return startupTickMsg{attempt: attempt} })
}

func (m startupModel) startWorker() {
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		defer close(m.messages)
		send := func(msg tea.Msg) {
			select {
			case m.messages <- msg:
			case <-m.dashboard.ctx.Done():
			}
		}
		m.run(send)
	}()
}

func (m startupModel) run(send func(tea.Msg)) {
	ctx := m.dashboard.ctx
	options := m.dashboard.options
	if m.originalURL == "" {
		state, err := ensureViewServer(ctx, service.Options{DBPath: options.dbPath})
		if err != nil {
			send(startupFailedMsg{phase: startupServer, err: err})
			return
		}
		if state.Record == nil {
			send(startupFailedMsg{phase: startupServer, err: errors.New("local query server unavailable")})
			return
		}
		options.serverURL = state.Record.URL
	}
	send(startupServerMsg{url: options.serverURL})
	if m.collect {
		result, err := runViewCollector(ctx, collector.Options{
			CollectorDBPath: options.collectorDBPath, ServerDBPath: options.dbPath,
			ServerURL: m.originalURL, Token: options.token,
			SyncOptions: pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, Normalize: true, Now: m.dashboard.now,
				Progress: func(event pipeline.SyncProgressEvent) { send(syncProgressMsg{event: event}) }},
			EnsureLocal:      func(context.Context) (string, error) { return options.serverURL, nil },
			DeliveryProgress: func(progress collector.DeliveryProgress) { send(startupDeliveryMsg{progress: progress}) },
		})
		if err != nil {
			phase := startupCollect
			if result.DeliveryError != nil {
				phase = startupPublish
			}
			send(startupFailedMsg{phase: phase, err: err})
			return
		}
	}
	send(startupLoadMsg{})
	dashboard := m.dashboard
	dashboard.options = options
	data := dashboard.loadDashboard()
	if data.err != nil {
		send(startupFailedMsg{phase: startupLoad, err: data.err})
		return
	}
	send(startupReadyMsg{data: data})
}

func (m startupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.dashboard.width, m.dashboard.height = msg.Width, msg.Height
		return m, nil
	case startupTickMsg:
		if msg.attempt != m.attempt {
			return m, nil
		}
		m.frame++
		if m.busy {
			return m, startupAnimationCmd(m.attempt)
		}
		return m, nil
	case startupServerMsg:
		m.dashboard.options.serverURL = msg.url
		m.phase = startupCollect
	case syncProgressMsg:
		m.dashboard = m.dashboard.withSyncProgress(msg.event)
		m.phase = startupCollect
		if msg.event.Status == pipeline.SyncProgressWaiting {
			m.phase = startupWaiting
		}
	case startupDeliveryMsg:
		m.phase, m.delivery = startupPublish, msg.progress
	case startupLoadMsg:
		m.phase = startupLoad
	case startupFailedMsg:
		m.busy, m.phase, m.err = false, msg.phase, msg.err
		return m, nil
	case startupReadyMsg:
		updated, loaded := m.dashboard.Update(msg.data)
		dashboard := updated.(interactiveModel)
		if dashboard.err != nil {
			m.busy, m.phase, m.err = false, startupLoad, dashboard.err
			return m, nil
		}
		updated, sized := dashboard.Update(tea.WindowSizeMsg{Width: m.dashboard.width, Height: m.dashboard.height})
		return updated, tea.Batch(loaded, sized)
	case tea.KeyMsg:
		if msg.String() == "q" || msg.Type == tea.KeyCtrlC {
			m.dashboard.cancelSync()
			return m, tea.Quit
		}
		if m.busy {
			return m, nil
		}
		switch msg.String() {
		case "r":
			return m.retry(m.phase != startupLoad)
		case "v":
			if m.dashboard.options.serverURL != "" {
				return m.retry(false)
			}
		}
		return m, nil
	default:
		return m, nil
	}
	return m, readSyncProgressCmd(m.messages)
}

func (m startupModel) retry(collect bool) (tea.Model, tea.Cmd) {
	m.busy, m.err, m.collect = true, nil, collect
	m.attempt++
	m.messages = make(chan tea.Msg, startupMessageBuffer)
	if collect {
		m.phase = startupCollect
		m.delivery = collector.DeliveryProgress{}
		m.dashboard.syncProgressRows = initialSyncProgressRows()
	} else {
		m.phase = startupLoad
	}
	return m, m.Init()
}

func (m startupModel) View() string {
	width, height := max(1, m.dashboard.width), max(1, m.dashboard.height)
	contentWidth := max(1, width-2)
	panelWidth := min(startupPanelWidth, contentWidth)
	left := max(1, (width-panelWidth)/2)
	title := "Updating usage"
	if m.err != nil {
		title = "Update needs attention"
	}
	phase := string(m.phase)
	if m.busy {
		phase = syncSpinnerFrame(m.frame) + "  " + phase
	} else {
		phase = m.failureLabel()
	}
	phaseStyle := hintStyle
	if m.err != nil {
		phaseStyle = syncFailStyle
	}
	lines := []string{titleStyle.Render(title), phaseStyle.Render(phase), ""}
	if height < 12 {
		lines = lines[:2]
	}
	if m.err != nil {
		var stage *collector.StageError
		if errors.As(m.err, &stage) {
			reason := syncFailStyle.Render(stage.Code)
			if height < 12 {
				lines[1] = reason
			} else {
				lines = append(lines, reason)
			}
		}
	}
	if m.err == nil && height >= 14 {
		for _, row := range m.dashboard.syncProgressRows {
			lines = append(lines, fmt.Sprintf("%-12s  %s", row.label, m.harnessStatus(row.status)))
		}
		lines = append(lines, "")
	}
	if m.phase == startupPublish && m.delivery.PendingKnown {
		if panelWidth < 45 {
			lines = append(lines, dimensionStyle.Render(fmt.Sprintf("%d batches committed", m.delivery.Batches)), dimensionStyle.Render(fmt.Sprintf("%d pending", m.delivery.Pending)))
		} else {
			lines = append(lines, dimensionStyle.Render(fmt.Sprintf("%d batches committed · %d pending", m.delivery.Batches, m.delivery.Pending)))
		}
	}
	if m.err != nil && height >= 12 {
		if m.dashboard.options.serverURL != "" {
			caption := "Committed usage is safe. Retry or view saved data."
			if panelWidth < 50 {
				caption = "Committed usage is safe."
			}
			lines = append(lines, hintStyle.Render(caption))
		} else {
			lines = append(lines, hintStyle.Render("Retry to connect to your local usage server."))
		}
	} else if m.err == nil && m.phase != startupPublish {
		caption := "All four harnesses · your filters apply after sync"
		if contentWidth < 50 {
			caption = "All four harnesses"
		}
		lines = append(lines, hintStyle.Render(caption))
	}
	if m.err != nil {
		guidance := "Details: tokeninsights sync"
		switch m.phase {
		case startupServer:
			guidance = "tokeninsights service status"
		case startupLoad:
			guidance = "Check server URL and token."
		}
		lines = append(lines, hintStyle.Render(guidance))
	}
	footer := deskControl("q", "Quit", "")
	if m.err != nil {
		footer = deskControl("r", "Retry", "") + "   " + footer
		if m.dashboard.options.serverURL != "" {
			label := "View saved"
			if contentWidth < 40 {
				label = "Saved"
			}
			footer = deskControl("r", "Retry", "") + "   " + deskControl("v", label, "") + "   " + deskControl("q", "Quit", "")
		}
	}
	canvas := make([]string, height)
	canvas[0] = " " + titleStyle.Render("TokenInsights")
	available := max(0, height-4)
	top := 2 + max(0, (available-len(lines))/2)
	for i, line := range lines {
		if top+i >= height-2 {
			break
		}
		canvas[top+i] = strings.Repeat(" ", left) + ansi.Truncate(line, panelWidth, "…")
	}
	if height >= 2 {
		canvas[height-2] = " " + dividerStyle.Render(strings.Repeat("─", contentWidth))
		canvas[height-1] = " " + ansi.Truncate(footer, contentWidth, "…")
	}
	return renderOnAppSurface(strings.Join(canvas, "\n"), width, height)
}

func (m startupModel) harnessStatus(status pipeline.SyncProgressStatus) string {
	switch status {
	case pipeline.SyncProgressDiscovering:
		return syncBusyStyle.Render("Finding sessions")
	case pipeline.SyncProgressSyncing:
		return syncBusyStyle.Render(syncSpinnerFrame(m.frame) + " Reading sessions")
	case pipeline.SyncProgressNormalizing:
		return syncBusyStyle.Render("Normalizing usage")
	case pipeline.SyncProgressSynced:
		return syncOKStyle.Render("Ready")
	case pipeline.SyncProgressSkipped:
		return syncSkipStyle.Render("No new usage")
	case pipeline.SyncProgressFailed:
		return syncFailStyle.Render("Needs attention")
	default:
		return syncSkipStyle.Render("Waiting")
	}
}

func (m startupModel) failureLabel() string {
	switch m.phase {
	case startupServer:
		return "Couldn't start the local server."
	case startupPublish:
		return "Couldn't publish usage."
	case startupLoad:
		return "Couldn't load saved usage."
	default:
		return "Couldn't collect all local usage."
	}
}
