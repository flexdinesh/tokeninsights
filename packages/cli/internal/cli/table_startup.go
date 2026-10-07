package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/clientworkflow"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

type startupPhase string

const (
	startupServer        startupPhase = "Starting local server"
	startupCollect       startupPhase = "Checking local sessions"
	startupWaiting       startupPhase = "Waiting for another sync"
	startupPublish       startupPhase = "Submitting usage"
	startupLoad          startupPhase = "Loading saved usage"
	startupPanelWidth                 = 58
	startupMessageBuffer              = 32
)

type startupServerMsg struct{ url, datasetID string }
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
	settings := m.dashboard.localSettings
	if settings.ServerKind == "" {
		settings.ServerKind = serverfeatures.Personal
	}
	settings.ServerURL, settings.ServerToken = m.originalURL, options.token
	session, err := clientworkflow.Resolve(ctx, settings, func(ctx context.Context) (service.State, error) {
		return ensureViewConfiguredLocal(ctx, options.dbPath, settings)
	})
	if err != nil {
		send(startupFailedMsg{phase: startupServer, err: err})
		return
	}
	if err := session.Require(serverfeatures.Read, serverfeatures.TerminalDashboard, serverfeatures.Usage, serverfeatures.Facets); err != nil {
		send(startupFailedMsg{phase: startupServer, err: err})
		return
	}
	if m.collect {
		if err := session.VerifyIngestion(ctx); err != nil {
			send(startupFailedMsg{phase: startupServer, err: err})
			return
		}
	}
	options.serverURL, options.datasetID = session.URL, session.Descriptor.DatasetId
	send(startupServerMsg{url: options.serverURL, datasetID: options.datasetID})
	if m.collect {
		observer := session.Observe(ctx)
		result, err := runViewCollector(ctx, collector.Options{
			CollectorDBPath: options.collectorDBPath, ServerDBPath: options.dbPath,
			ServerURL: m.originalURL, Token: options.token,
			Destination: session.Destination,
			SyncOptions: pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, Normalize: true, Now: m.dashboard.now,
				Progress: func(event pipeline.SyncProgressEvent) {
					observer.Collection(event)
					send(syncProgressMsg{event: event})
				}},
			EnsureLocal: func(context.Context) (string, error) { return options.serverURL, nil },
			DeliveryProgress: func(progress collector.DeliveryProgress) {
				observer.Delivery(progress)
				send(startupDeliveryMsg{progress: progress})
			},
		})
		observer.Finish(result, err)
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
		m.dashboard.options.datasetID = msg.datasetID
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
		if code := m.failureCode(); code != "" {
			reason := syncFailStyle.Render(code)
			if height < 12 {
				lines[1] = reason
			} else {
				lines = append(lines, reason)
			}
		}
	}
	if m.err == nil && height >= 14 {
		for _, row := range m.dashboard.syncProgressRows {
			lines = append(lines, fmt.Sprintf("%-12s  %s", row.label, m.harnessRowStatus(row)))
		}
		lines = append(lines, "")
	}
	if m.phase == startupPublish && m.delivery.PendingKnown {
		progress := fmt.Sprintf("%d entries accepted · %d entries pending", m.delivery.Accepted, m.delivery.Pending)
		if ansi.StringWidth(progress) > panelWidth {
			lines = append(lines, dimensionStyle.Render(fmt.Sprintf("%d entries accepted", m.delivery.Accepted)), dimensionStyle.Render(fmt.Sprintf("%d entries pending", m.delivery.Pending)))
		} else {
			lines = append(lines, dimensionStyle.Render(progress))
		}
	}
	if count := m.quarantinedFiles(); count > 0 && height >= 12 {
		lines = append(lines, syncFailStyle.Render(fmt.Sprintf("%d files quarantined; usage incomplete.", count)))
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
		if m.quarantinedFiles() > 0 {
			guidance = "Retry files: tokeninsights sync --full-refresh"
		}
		switch m.phase {
		case startupServer:
			guidance = "tokeninsights service status"
		case startupLoad:
			guidance = m.loadFailureGuidance()
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

func (m startupModel) failureCode() string {
	var stage *collector.StageError
	if errors.As(m.err, &stage) {
		return stage.Code
	}
	if m.phase != startupLoad {
		return ""
	}
	var status *queryclient.StatusError
	if errors.As(m.err, &status) {
		return fmt.Sprintf("http_%d", status.StatusCode)
	}
	switch {
	case errors.Is(m.err, queryclient.ErrSnapshotChanged):
		return "snapshot_changed"
	case errors.Is(m.err, queryclient.ErrUnavailable):
		return "analytics_unavailable"
	case errors.Is(m.err, context.DeadlineExceeded):
		return "query_timeout"
	default:
		return "query_failed"
	}
}

func (m startupModel) loadFailureGuidance() string {
	var status *queryclient.StatusError
	if errors.As(m.err, &status) {
		switch status.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "Check server URL and token."
		case http.StatusServiceUnavailable:
			return "Server read failed; check service logs."
		}
	}
	if errors.Is(m.err, queryclient.ErrSnapshotChanged) {
		return "Usage changed during loading; retry."
	}
	return "Check server status and retry."
}

func (m startupModel) quarantinedFiles() int {
	count := 0
	for _, row := range m.dashboard.syncProgressRows {
		count += row.quarantined
	}
	return count
}

func (m startupModel) harnessRowStatus(row syncProgressRow) string {
	if row.quarantined > 0 {
		return syncFailStyle.Render(fmt.Sprintf("%d quarantined", row.quarantined))
	}
	return m.harnessStatus(row.status)
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
