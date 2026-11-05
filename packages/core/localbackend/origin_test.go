package localbackend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type deadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
	set      bool
	cancel   context.CancelFunc
}

func (w *deadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.set = true
	w.deadline = deadline
	return nil
}
func (w *deadlineWriter) Flush() { w.ResponseRecorder.Flush(); w.cancel() }

func TestViewerStreamRemovesOnlyItsWriteDeadlineAndReleasesLease(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	writer := &deadlineWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, deadline: time.Now().Add(time.Second)}
	b := &backend{viewers: newViewers()}
	b.stream(writer, httptest.NewRequest(http.MethodGet, "/api/local/v1/viewers", nil).WithContext(ctx))
	if !writer.set || !writer.deadline.IsZero() || writer.Code != http.StatusOK || writer.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatal("stream retained ordinary HTTP deadline", writer.set, writer.deadline, writer.Code)
	}
	if b.viewers.state.viewers != 0 {
		t.Fatal("disconnected stream retained viewer")
	}
}
