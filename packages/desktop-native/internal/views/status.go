package views

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// StatusDetails is what the header status shows: its value on the button,
// and in its popover the backend identity, the last ingestion and the
// collector progress.
type StatusDetails struct {
	Value dashboardview.StatusValue
	// InstanceID is the backend identity; empty hides it until the
	// instance loads.
	InstanceID string
	// LastSynced is the last ingestion usage reported, in Unix
	// milliseconds; zero hides it.
	LastSynced int64
	// Timezone is the server timezone LastSynced shows in.
	Timezone string
	// Progress is the collector progress; nil hides it.
	Progress *collectorprogress.Snapshot
}

// statusDetails is the header status of w. Usage, which reports the last
// ingestion, is not read yet, so the popover omits it.
func statusDetails(w *model.Window) StatusDetails {
	d := StatusDetails{Value: w.StatusValue()}
	if instance, ok := w.Instance(); ok {
		d.InstanceID, d.Timezone = instance.InstanceID, instance.Timezone
	}
	if progress, ok := w.Progress(); ok {
		d.Progress = &progress
	}
	return d
}

// hoverCloseDelay is how long the status popover stays after the pointer
// leaves it and its button, as the web's hoverCloseDelayMs, so the pointer
// can cross the gap between them.
const hoverCloseDelay = 150 * time.Millisecond

// Status popover names.
const (
	statusLabelPrefix  = "Backend status: "
	statusDetailsLabel = "Backend status details"
	identityPrefix     = "Backend identity: "
	ingestionPrefix    = "Last ingestion "
)

// statusPopover is the status popover's state, kept by its button: open
// while hovered, or pinned open by a click or the keyboard focus until a
// press outside or Escape.
type statusPopover struct {
	open, pinned bool
	// hovered and panelHovered are the pointer's last frame over the
	// button or the panel; focused the button's keyboard focus.
	hovered, panelHovered, focused bool
	// closeAt is when an unpinned popover closes after the pointer left.
	closeAt time.Time
}

func (p *statusPopover) reveal() { p.open, p.closeAt = true, time.Time{} }

// statusIconStates map the status tone to the kit icon.
var statusIconStates = map[dashboardview.StatusState]kit.StatusState{
	dashboardview.StatusReady:     kit.StatusReady,
	dashboardview.StatusLoading:   kit.StatusLoading,
	dashboardview.StatusAttention: kit.StatusAttention,
}

// DashboardStatus builds the header status, the web's DashboardStatus: an
// icon button named by the status label whose popover shows the details.
// Hovering the button or the popover opens it, and it closes
// hoverCloseDelay after the pointer leaves both; a click or the keyboard
// focus pins it open until a press outside or Escape. Give it a key.
func DashboardStatus(c *ui.Context, d StatusDetails) ui.Element {
	b := kit.ButtonContent(c, kit.ButtonStyle{Variant: kit.Ghost}, func() {
		kit.StatusIcon(c, statusIconStates[d.Value.State])
	})
	b.Size(theme.ControlHeight, theme.ControlHeight).Padding(0).Label(statusLabelPrefix + d.Value.Label)
	p := ui.Local(b, "popover", func() statusPopover { return statusPopover{} })
	now := c.Now()
	if b.Clicked() {
		p.pinned = true
		p.reveal()
	}
	if focused := b.Focused(); focused != p.focused {
		p.focused = focused
		if focused {
			p.pinned = true
			p.reveal()
		}
	}
	if hovered := b.Hovered() || p.panelHovered; hovered != p.hovered {
		p.hovered = hovered
		switch {
		case hovered:
			p.reveal()
		case !p.pinned:
			p.closeAt = now.Add(hoverCloseDelay)
		}
	}
	if !p.closeAt.IsZero() {
		if now.Before(p.closeAt) {
			c.After(p.closeAt.Sub(now))
		} else {
			p.open, p.closeAt = false, time.Time{}
		}
	}
	wasOpen := p.open
	panel := kit.PopoverWith(c, b, &p.open, kit.PopoverOptions{Width: theme.StatusPopoverWidth, End: true}, func() {
		statusPanel(c, d)
	})
	panel.Label(statusDetailsLabel)
	if wasOpen && !p.open {
		// A press outside or Escape closed it, unpinning it.
		p.pinned, p.closeAt = false, time.Time{}
	}
	if hovered := panel.Hovered(); hovered != p.panelHovered {
		p.panelHovered = hovered
		c.Invalidate()
	}
	return b
}

// statusPanel is the popover's content: the status label, the backend
// identity, the last ingestion and collector progress, in subtle label
// text. The web's backend CPU and RAM are omitted (ADR 0032).
func statusPanel(c *ui.Context, d StatusDetails) {
	t := theme.Current(c)
	ui.Column(c).Gap(theme.Unit * 2).TextColor(t.InkSubtle).Children(func() {
		theme.Label(ui.Text(c, d.Value.Label))
		if d.InstanceID != "" {
			theme.Label(ui.Text(c, identityPrefix+d.InstanceID))
		}
		if d.LastSynced != 0 {
			at := theme.Label(ui.Text(c, ingestionPrefix+dashboardview.FormatServerDateTime(d.LastSynced, d.Timezone)))
			kit.Tooltip(c, at, dashboardview.ServerZoneLabel(d.Timezone))
		}
		if d.Progress != nil {
			CollectorProgress(c, *d.Progress)
		}
	})
}
