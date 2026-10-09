package datastore

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

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
