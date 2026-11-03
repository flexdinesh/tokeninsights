package collectorprogress

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (r *Registry) ReadHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.NotFound(w, request)
			return
		}
		writeJSON(w, http.StatusOK, r.Snapshot())
	})
}

// ControlHandler must only be mounted inside the instance-verified private
// control transport. Public routes expose ReadHandler alone.
func (r *Registry) ControlHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.NotFound(w, request)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, MaxRequestBytes))
		decoder.DisallowUnknownFields()
		var message Message
		if err := decoder.Decode(&message); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid collector progress"})
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid collector progress"})
			return
		}
		if err := r.Apply(message); err != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(err, ErrMissing):
				status = http.StatusNotFound
			case errors.Is(err, ErrConflict):
				status = http.StatusConflict
			case errors.Is(err, ErrBusy):
				status = http.StatusServiceUnavailable
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, struct {
			Accepted bool `json:"accepted"`
		}{true})
	})
}
