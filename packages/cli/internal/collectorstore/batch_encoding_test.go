package collectorstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func encodingFixture(large bool) (publication.Batch, []publication.Entry) {
	batch := publication.Batch{ProtocolVersion: publication.ProtocolVersion, IdentityVersion: publication.IdentityVersion, SemanticsVersion: publication.SemanticsVersion, DatabaseID: "database<&é", StreamID: "stream", BatchID: "batch", Hostname: "host<&é", FromSequence: 98}
	entries := make([]publication.Entry, publication.MaxEntries+1)
	for i := range entries {
		fact := testFact(fmt.Sprint(i))
		if large {
			html := strings.Repeat("<", publication.MaxStringBytes)
			fact.Session.NativeID = html
			fact.Message.NativeID = fmt.Sprintf("%03d", i) + strings.Repeat("<", publication.MaxStringBytes-3)
			fact.NativeRequestID = html
			fact.Provider = html
			fact.Model = strings.Repeat("é", publication.MaxStringBytes/2)
			fact.Location = &publication.Location{DirectoryKey: html, DirectoryName: html, RepositoryKey: html, RepositoryName: html, RepositorySource: "git"}
			publication.SetIDs(&fact)
		}
		entries[i] = publication.Entry{Sequence: batch.FromSequence + int64(i), Fact: fact}
	}
	return batch, entries
}

func TestLegacyBatchEncodingBoundsAndEscaping(t *testing.T) {
	for _, large := range []bool{false, true} {
		t.Run(fmt.Sprint(large), func(t *testing.T) {
			batch, entries := encodingFixture(large)
			encodedBatch, body, err := encodeBatchPrefix(batch, entries)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := publication.EncodeBatch(encodedBatch)
			if err != nil || !bytes.Equal(expected, body) {
				t.Fatalf("wire encoding differs from standard encoding: %v", err)
			}
			if len(body) > publication.MaxBodyBytes || encodedBatch.ToSequence != batch.FromSequence+int64(len(encodedBatch.Entries))-1 {
				t.Fatal("invalid body size or sequence range")
			}
			if !bytes.Contains(body, []byte(`\u003c`)) || !bytes.Contains(body, []byte("é")) {
				t.Fatal("escaping or multibyte data changed")
			}
			if !large && len(encodedBatch.Entries) != publication.MaxEntries {
				t.Fatal("entry bound changed")
			}
			if large {
				next := encodedBatch
				next.Entries = append(next.Entries, entries[len(next.Entries)])
				next.ToSequence++
				tooLarge, err := json.Marshal(next)
				if err != nil || len(tooLarge) <= publication.MaxBodyBytes {
					t.Fatalf("batch stopped before byte bound: %d %v", len(tooLarge), err)
				}
			}
		})
	}
}

func TestLegacyBatchRejectsGapInvalidAndOversizedRecords(t *testing.T) {
	for _, kind := range []string{"gap", "invalid", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			batch, entries := encodingFixture(false)
			entries = entries[:2]
			switch kind {
			case "gap":
				entries[1].Sequence++
			case "invalid":
				entries[1].Fact.TotalTokens++
			case "oversized":
				entries[0].Fact.Model = strings.Repeat("x", publication.MaxBodyBytes)
			}
			if _, _, err := encodeBatchPrefix(batch, entries); err == nil {
				t.Fatal("invalid record accepted")
			}
		})
	}
}

func TestLegacyByteBoundBatchRetryAndSuffix(t *testing.T) {
	store, _ := newStore(t)
	_, entries := encodingFixture(true)
	tx, err := store.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, entry := range entries {
		if _, err := Record(t.Context(), tx, entry.Fact, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.BindDestination(t.Context(), "destination", "http://remote", "database"); err != nil {
		t.Fatal(err)
	}
	first, err := store.PrepareBatch(t.Context(), "destination", "host<&é", 1)
	if err != nil || first == nil {
		t.Fatal(err)
	}
	if len(first.Batch.Entries) >= publication.MaxEntries {
		t.Fatal("byte bound was not exercised")
	}
	retry, err := store.PrepareBatch(t.Context(), "destination", "changed-host", 2)
	if err != nil || retry == nil || !bytes.Equal(first.Request, retry.Request) || first.RequestHash != retry.RequestHash {
		t.Fatal("byte-bound retry changed saved request", err)
	}
	if err := store.Ack(t.Context(), "destination", receiptFor(first), 3); err != nil {
		t.Fatal(err)
	}
	next, err := store.PrepareBatch(t.Context(), "destination", "host", 4)
	if err != nil || next == nil || next.Batch.FromSequence != first.Batch.ToSequence+1 || next.Batch.Entries[0].Fact.ID != entries[len(first.Batch.Entries)].Fact.ID {
		t.Fatal("byte bound skipped or repeated suffix", err)
	}
}

func BenchmarkLegacyBatchEncoding(b *testing.B) {
	batch, entries := encodingFixture(false)
	entries = entries[:publication.MaxEntries]
	b.Run("growing_prefix", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			candidate := batch
			for _, entry := range entries {
				candidate.Entries = append(candidate.Entries, entry)
				candidate.ToSequence = entry.Sequence
				if _, err := publication.EncodeBatch(candidate); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("encoded_entries", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := encodeBatchPrefix(batch, entries); err != nil {
				b.Fatal(err)
			}
		}
	})
}
