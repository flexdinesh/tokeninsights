package ingestion

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func NewHandler(core *Core) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/ingestion/capabilities", core.HandleCapabilities)
	mux.HandleFunc("POST /api/v1/ingestion/batches", core.HandleBatches)
	return mux
}

func (c *Core) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	m, err := c.store.Metadata(r.Context())
	if err != nil {
		writeError(w, databaseError(err))
		return
	}
	writeJSON(w, http.StatusOK, publication.NewCapabilities(m.DatabaseID))
}

func (c *Core) HandleBatches(w http.ResponseWriter, r *http.Request) {
	select {
	case c.admission <- struct{}{}:
		defer func() { <-c.admission }()
	default:
		writeError(w, failure(http.StatusServiceUnavailable, "busy", "admission"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, publication.MaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			writeError(w, failure(http.StatusRequestEntityTooLarge, "body_limit", "validation"))
		} else {
			writeError(w, failure(http.StatusBadRequest, "body_read_failed", "validation"))
		}
		return
	}
	receipt, err := c.ingest(r.Context(), body)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

func writeError(w http.ResponseWriter, err error) {
	e, ok := err.(*Error)
	if !ok {
		e = &Error{Status: http.StatusInternalServerError, Code: "transaction_failed", Stage: "database"}
	}
	writeJSON(w, e.Status, publication.ErrorResponse{Code: e.Code, Stage: e.Stage})
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
