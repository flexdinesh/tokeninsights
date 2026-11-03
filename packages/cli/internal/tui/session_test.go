package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

func TestTUIProductionSessionAttachesWithoutPreparingOwnedStorage(t *testing.T) {
	root := t.TempDir()
	settings := config.LocalSettings{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite"), AppDBPath: filepath.Join(root, "app.sqlite"), Host: "127.0.0.1"}
	backendContext, cancelBackend := context.WithCancel(t.Context())
	ready, finished := make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- localbackend.Serve(backendContext, localprotocol.Settings{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath, AppDBPath: settings.AppDBPath}, io.Discard,
			func(localprotocol.Descriptor) error { close(ready); return nil })
	}()
	defer func() {
		cancelBackend()
		select {
		case err := <-finished:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("backend did not stop")
		}
	}()
	select {
	case <-ready:
	case err := <-finished:
		finished <- err
		t.Fatal("backend startup failed", err)
	case <-time.After(10 * time.Second):
		t.Fatal("backend not ready")
	}
	other, err := localclient.Connect(t.Context(), localclient.Options{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath, AppDBPath: settings.AppDBPath, Host: settings.Host})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()

	for _, collect := range []bool{false, true} {
		t.Run(fmt.Sprint(collect), func(t *testing.T) {
			restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
				if model.refresh.collectionPending != collect || (model.collectionDone != nil) != collect {
					t.Fatal("wrong startup capture policy")
				}
				usage := model.loadDashboard()
				if usage.err != nil || usage.instanceID != other.InstanceID {
					t.Fatal("viewer did not attach to compatible backend", usage.err)
				}
				if collect {
					model = finishCollectionForTest(t, model)
					if model.refresh.result == nil || model.refresh.result.Err != nil || model.refresh.result.Result.ErrorCode != "" || model.refresh.result.Result.AttemptID == "" {
						t.Fatal("shared collection result unavailable", model.refresh.result)
					}
					model = loadRefreshStatusForTest(t, model)
					model = loadDashboardForTest(t, model)
					model.width = 120
					if !strings.Contains(model.refreshLine(), "no new usage") {
						t.Fatal("empty shared capture did not finish through displayed publication", model.refreshLine())
					}
				}
				status := model.loadSharedSync()
				if status.err != nil || status.instanceID != other.InstanceID || status.refreshStatus == nil {
					t.Fatal("shared progress unavailable", status.err)
				}
				return model, nil
			})
			defer restore()
			err := Run(Command{Context: t.Context(), Args: []string{"--sync=" + strconv.FormatBool(collect), "--source-dir", root}, Resolve: func(config.Overrides) (config.LocalSettings, error) { return settings, nil },
				Prepare: func(config.LocalSettings) error { t.Fatal("attached viewer inspected owned storage"); return nil }, Now: time.Now(), Stdout: io.Discard, Stderr: io.Discard})
			if err != nil {
				t.Fatal(err)
			}
		})
	}

	if _, err := other.Query.Instance(t.Context()); err != nil {
		t.Fatal("TUI close disrupted another viewer", err)
	}
	select {
	case <-other.Done():
		t.Fatal("TUI close disconnected another viewer")
	default:
	}
}

type progressFailureSession struct{ Session }

func (progressFailureSession) Progress(context.Context) (collectorprogress.DetailSnapshot, error) {
	return collectorprogress.DetailSnapshot{}, errors.New("private progress endpoint?token=secret")
}

func TestTUIProgressFailurePreservesSavedDataAndPrivacy(t *testing.T) {
	options := localViewOptions(t, true)
	options.local = progressFailureSession{options.local}
	model := newInteractiveModel(t.Context(), options, time.Now(), "fixture")
	defer model.cancelSync()
	model.width, model.height = 120, 35
	model = loadDashboardForTest(t, model)
	updated, _ := model.Update(model.loadSharedSync())
	model = updated.(interactiveModel)
	if len(model.rows) != 1 || model.rows[0].totalValue != 100 || model.refreshBusy() {
		t.Fatal("progress failure changed saved usage")
	}
	view := model.View()
	if !strings.Contains(view, "Refresh status unavailable") || strings.Contains(view, "secret") || strings.Contains(view, "private progress") {
		t.Fatal("unsafe or missing failure status", view)
	}
}

func TestTUIBackendDisconnectCancelsViewerRequests(t *testing.T) {
	model := savedRefreshModel(t)
	updated, command := model.Update(sessionDisconnectedMsg{})
	model = updated.(interactiveModel)
	if command == nil || !errors.Is(model.err, errSessionDisconnected) || !errors.Is(model.ctx.Err(), context.Canceled) {
		t.Fatal("disconnect left viewer running", model.err)
	}
}

func TestTUICollectionTransportFailureCannotReuseOldAcceptance(t *testing.T) {
	model := savedRefreshModel(t)
	updated, _ := model.Update(refreshStatusMessage(0, 1, 1, 1, 1))
	model = updated.(interactiveModel)
	model.refresh.result = &collectionResult{Err: errors.New("private collection endpoint?token=secret")}
	model.refresh.loaded, model.refresh.displayedToken = true, model.refresh.token
	if !strings.Contains(model.refreshLine(), "Refresh incomplete") || model.refreshBusy() || strings.Contains(model.View(), "secret") {
		t.Fatal("failed request reused previous acceptance", model.refreshLine())
	}
}

func TestTUIParentCancellationEndsProgramWithoutBackendFailure(t *testing.T) {
	model := savedRefreshModel(t)
	model.cancelSync()
	updated, command := model.Update(sessionDisconnectedMsg{})
	model = updated.(interactiveModel)
	if command == nil || !errors.Is(model.err, context.Canceled) {
		t.Fatal("parent cancellation reported backend failure", model.err)
	}
}
