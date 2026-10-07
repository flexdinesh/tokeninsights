// Package evidence defines the sanitized, source-shaped ingestion contract.
package evidence

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

const (
	ProtocolVersion       = 3
	LegacyProtocolVersion = 2
	ExtractorVersion      = 1
	ProcessorVersion      = 1
	MaxBodyBytes          = publication.MaxBodyBytes
	MaxEntries            = publication.MaxEntries
	MaxStringBytes        = publication.MaxStringBytes
)

type Location struct {
	DirectoryKey     string `json:"directoryKey,omitempty"`
	DirectoryName    string `json:"directoryName,omitempty"`
	RepositoryKey    string `json:"repositoryKey,omitempty"`
	RepositoryName   string `json:"repositoryName,omitempty"`
	RepositorySource string `json:"repositorySource,omitempty"`
}

// Context preserves selected native context records, not inferred fact values.
type Context struct {
	Ordinal     int64           `json:"ordinal"`
	Data        json.RawMessage `json:"data"`
	Diagnostics []string        `json:"diagnostics,omitempty"`
}

type Record struct {
	Harness     string          `json:"harness"`
	Format      string          `json:"format"`
	SourceID    string          `json:"sourceId"`
	Lineage     string          `json:"lineage"`
	Ordinal     int64           `json:"ordinal"`
	Data        json.RawMessage `json:"data"`
	Context     []Context       `json:"context,omitempty"`
	Location    *Location       `json:"location,omitempty"`
	Diagnostics []string        `json:"diagnostics,omitempty"`
}

type Entry struct {
	Sequence int64  `json:"sequence"`
	Record   Record `json:"record"`
}

type Batch struct {
	ProtocolVersion  int     `json:"protocolVersion"`
	ExtractorVersion int     `json:"extractorVersion"`
	DatabaseID       string  `json:"databaseId"`
	DatasetID        string  `json:"datasetId,omitempty"`
	StreamID         string  `json:"streamId"`
	BatchID          string  `json:"batchId"`
	FromSequence     int64   `json:"fromSequence"`
	ToSequence       int64   `json:"toSequence"`
	Entries          []Entry `json:"entries"`
}

type Capabilities struct {
	ProtocolVersion  int    `json:"protocolVersion"`
	ExtractorVersion int    `json:"extractorVersion"`
	DatabaseID       string `json:"databaseId"`
	DatasetID        string `json:"datasetId"`
	Completion       string `json:"completion"`
	MaxBodyBytes     int    `json:"maxBodyBytes"`
	MaxEntries       int    `json:"maxEntries"`
}

type Receipt struct {
	DatabaseID    string `json:"databaseId"`
	DatasetID     string `json:"datasetId"`
	StreamID      string `json:"streamId"`
	BatchID       string `json:"batchId"`
	RequestHash   string `json:"requestHash"`
	FromSequence  int64  `json:"fromSequence"`
	ToSequence    int64  `json:"toSequence"`
	Accepted      int64  `json:"accepted"`
	AcceptedAtMs  int64  `json:"acceptedAtMs"`
	InputRevision int64  `json:"inputRevision"`
}

type Outcome struct {
	EvidenceID    string `json:"evidenceId"`
	Disposition   string `json:"disposition"`
	Code          string `json:"code,omitempty"`
	FactID        string `json:"factId,omitempty"`
	Generation    int64  `json:"generation"`
	InputRevision int64  `json:"inputRevision"`
}

type Status struct {
	Generation    int64     `json:"generation"`
	InputRevision int64     `json:"inputRevision"`
	Pending       int64     `json:"pending"`
	Items         []Outcome `json:"items"`
}

type Response struct {
	Receipt    Receipt `json:"receipt"`
	Processing Status  `json:"processing"`
}

type Stored struct {
	ID     string
	Scope  string
	Record Record
}

type Contribution struct {
	Fact        publication.Fact
	EvidenceIDs []string
}

type Estimate struct {
	EvidenceID string
	Code       string
	Fact       publication.Fact
}

type Projection struct {
	Contributions []Contribution
	Estimates     []Estimate
	Outcomes      []Outcome
}

func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func Tuple(parts ...interface{}) string {
	body, _ := json.Marshal(parts)
	return Hash(body)
}

func RandomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

// ObservationKey never claims a canonical usage identity.
func ObservationKey(record Record) string {
	body, _ := json.Marshal(record)
	return Hash(body)
}

// EffectiveDatasetID preserves the default binding of byte-identical protocol-2 requests.
func (b Batch) EffectiveDatasetID() string {
	if b.ProtocolVersion == LegacyProtocolVersion && b.DatasetID == "" {
		return "default"
	}
	return b.DatasetID
}
