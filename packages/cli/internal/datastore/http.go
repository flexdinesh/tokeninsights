package datastore

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestionhttp"
)

const IngestionPrefix = "/api/v3/ingestion/"
const RawV2IngestionPrefix = "/api/v2/ingestion/"
const LegacyIngestionPrefix = "/api/v1/ingestion/"
const ProcessingPrefix = "/api/v2/processing/"

func respond(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (e *AdmissionError) IngestionCode() string { return e.Code }
func (s *Store) RawCapabilities(ctx context.Context) (evidence.Capabilities, error) {
	m, err := s.Metadata(ctx)
	if err != nil {
		return evidence.Capabilities{}, err
	}
	return evidence.Capabilities{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: m.DatabaseID, DatasetID: m.DatasetID, Completion: "acceptance", MaxBodyBytes: evidence.MaxBodyBytes, MaxEntries: evidence.MaxEntries}, nil
}
func (s *Store) AcquireAdmission() (func(), bool) {
	select {
	case s.admissions <- struct{}{}:
		return func() { <-s.admissions }, true
	default:
		return nil, false
	}
}
func (s *Store) RawHandler(protocol int) http.Handler {
	prefix := IngestionPrefix
	if protocol == evidence.LegacyProtocolVersion {
		prefix = RawV2IngestionPrefix
	}
	return ingestionhttp.Handler(s, prefix, protocol)
}
func (s *Store) AdminHandler() http.Handler { return ingestionhttp.AdminHandler(s, ProcessingPrefix) }

// Handler retains the private personal compatibility surface. Public servers
// mount raw acceptance and administration independently.
func (s *Store) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(IngestionPrefix, s.RawHandler(evidence.ProtocolVersion))
	if s.Kind() == KindPersonal {
		mux.Handle(RawV2IngestionPrefix, s.RawHandler(evidence.LegacyProtocolVersion))
		s.mountLegacy(mux)
		mux.Handle(ProcessingPrefix, s.AdminHandler())
	}
	return mux
}
