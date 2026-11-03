package localbackend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestViewerOriginAliasesWildcardAndTunnels(t *testing.T) {
	for _, scenario := range []struct {
		name, bind, host, origin, site string
		allowed                        bool
	}{
		{"advertised wildcard", "0.0.0.0", "127.0.0.1:8765", "", "same-origin", true},
		{"localhost alias", "127.0.0.1", "localhost:8765", "http://localhost:8765", "same-origin", true},
		{"forwarded port", "127.0.0.1", "127.0.0.1:43210", "http://127.0.0.1:43210", "same-origin", true},
		{"broad remote host", "0.0.0.0", "my-host:43210", "http://my-host:43210", "same-origin", true},
		{"foreign origin", "0.0.0.0", "my-host:43210", "http://evil.test", "cross-site", false},
		{"foreign port", "127.0.0.1", "localhost:43210", "http://localhost:8765", "same-site", false},
		{"loopback host guard", "127.0.0.1", "evil.test:8765", "", "same-origin", false},
		{"cross-site no origin", "0.0.0.0", "127.0.0.1:8765", "", "cross-site", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+scenario.host+"/api/local/v1/viewers", nil)
			r.Header.Set("Origin", scenario.origin)
			r.Header.Set("Sec-Fetch-Site", scenario.site)
			if viewerRequestAllowed(r, scenario.bind) != scenario.allowed {
				t.Fatal("viewer origin policy differs")
			}
		})
	}
}

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
