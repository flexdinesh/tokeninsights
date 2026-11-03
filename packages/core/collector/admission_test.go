package collector

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBoundedAdmissionErrorsAndRetryAfter(t *testing.T) {
	for _, code := range []string{"user_busy", "rate_limited", "unauthorized", "forbidden", "unavailable"} {
		t.Run(code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = fmt.Fprintf(w, `{"stage":"admission","code":%q}`, code)
			}))
			defer server.Close()
			_, err := request(t.Context(), server.Client(), server.URL, "", []byte("{}"))
			var stage *StageError
			if !errors.As(err, &stage) || stage.Stage != "admission" || stage.Code != code || stage.RetryAfter != 2*time.Second {
				t.Fatal(err)
			}
		})
	}
	for _, value := range []string{"-1", "99999999999999999999999999", "86401", "private-provider-error", ""} {
		if delay := retryDelay(value); delay != 0 {
			t.Fatal(value, delay)
		}
	}
	if delay := retryDelay("86400"); delay != maxRetryAfter {
		t.Fatal(delay)
	}
}
