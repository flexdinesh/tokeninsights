package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

type captureRequest struct {
	full   bool
	source string
}
type refreshSession struct {
	Session
	calls chan captureRequest
}

func (s refreshSession) Collect(_ context.Context, full bool, source string) (localprotocol.CollectionResult, error) {
	s.calls <- captureRequest{full: full, source: source}
	return localprotocol.CollectionResult{AttemptID: "explicit-refresh"}, nil
}

func TestExplicitRefreshUsesDefaultCaptureWhileReloadOnlyQueries(t *testing.T) {
	model := savedRefreshModel(t)
	requests := make(chan captureRequest, 2)
	model.options.local = refreshSession{Session: model.options.local, calls: requests}
	model.options.fullRefresh, model.options.sourceDir = true, "/startup-only"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if command == nil {
		t.Fatal("reload did not query saved data")
	}
	model = updated.(interactiveModel)
	select {
	case <-requests:
		t.Fatal("reload captured")
	default:
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	model = updated.(interactiveModel)
	if command == nil || !model.refresh.collectionPending {
		t.Fatal("explicit Refresh did not request capture")
	}
	request := <-requests
	if request.full || request.source != "" {
		t.Fatal("Refresh reused startup source or full-refresh override", request)
	}
	model = finishCollectionForTest(t, model)
	if model.refresh.result == nil || model.refresh.result.Result.AttemptID != "explicit-refresh" {
		t.Fatal("Refresh lost completion")
	}
}
