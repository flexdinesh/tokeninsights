package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlutil"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

// Bound statement size independently of component size.
const sqlBatchRows = 128

func sqlPlaceholders(count int) string { return strings.TrimSuffix(strings.Repeat("?,", count), ",") }
func writeRows(ctx context.Context, tx *sql.Tx, prefix, suffix string, rows [][]interface{}) error {
	for start := 0; start < len(rows); start += sqlBatchRows {
		end := min(start+sqlBatchRows, len(rows))
		values := make([]string, 0, end-start)
		args := make([]interface{}, 0, len(rows[start])*(end-start))
		for _, row := range rows[start:end] {
			values = append(values, "("+sqlPlaceholders(len(row))+")")
			args = append(args, row...)
		}
		if _, err := tx.ExecContext(ctx, sqlutil.Bind(prefix+strings.Join(values, ",")+suffix), args...); err != nil {
			return err
		}
	}
	return nil
}

type acceptedRecord struct {
	sequence                 int64
	id, scope, harness, body string
	parents                  []string
}

func prepareAcceptance(batch evidence.Batch) ([]acceptedRecord, error) {
	records := make([]acceptedRecord, 0, len(batch.Entries))
	for _, entry := range batch.Entries {
		body, err := json.Marshal(entry.Record)
		if err != nil {
			return nil, err
		}
		records = append(records, acceptedRecord{entry.Sequence, evidence.EvidenceID(entry.Record), evidence.Scope(entry.Record), entry.Record.Harness, string(body), parentScopes(entry.Record)})
	}
	return records, nil
}
func acceptRecords(ctx context.Context, tx *sql.Tx, datasetID string, batch evidence.Batch, records []acceptedRecord, revision, now int64) (bool, error) {
	previous := map[int64]string{}
	rows, err := tx.QueryContext(ctx, sqlutil.Bind("SELECT sequence,evidence_id FROM ingestion_items WHERE dataset_id=? AND stream_id=? AND sequence BETWEEN ? AND ?"), datasetID, batch.StreamID, batch.FromSequence, batch.ToSequence)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var sequence int64
		var id string
		if err := rows.Scan(&sequence, &id); err != nil {
			_ = rows.Close()
			return false, err
		}
		previous[sequence] = id
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return false, err
	}
	args := []interface{}{datasetID}
	for _, record := range records {
		args = append(args, record.id)
	}
	rows, err = tx.QueryContext(ctx, sqlutil.Bind("SELECT evidence_id FROM raw_evidence WHERE dataset_id=? AND evidence_id IN("+sqlPlaceholders(len(records))+")"), args...)
	if err != nil {
		return false, err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return false, err
		}
		existing[id] = true
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return false, err
	}
	var raw, items, mappings, scopes, dependencies [][]interface{}
	changedScopes := map[string]bool{}
	parents := map[[2]string]bool{}
	for _, record := range records {
		if id, found := previous[record.sequence]; found && id != record.id {
			return false, reject("sequence_conflict")
		}
		items = append(items, []interface{}{datasetID, batch.StreamID, record.sequence, record.id})
		mappings = append(mappings, []interface{}{datasetID, batch.StreamID, batch.BatchID, record.sequence, record.id})
		if existing[record.id] {
			continue
		}
		existing[record.id] = true
		raw = append(raw, []interface{}{datasetID, record.id, record.scope, record.harness, record.body, now})
		if !changedScopes[record.scope] {
			changedScopes[record.scope] = true
			scopes = append(scopes, []interface{}{datasetID, record.scope, revision})
		}
		for _, parent := range record.parents {
			key := [2]string{record.scope, parent}
			if !parents[key] {
				parents[key] = true
				dependencies = append(dependencies, []interface{}{datasetID, record.scope, parent})
			}
		}
	}
	if err := writeRows(ctx, tx, "INSERT INTO raw_evidence VALUES", "", raw); err != nil {
		return false, err
	}
	if err := writeRows(ctx, tx, "INSERT INTO processing_scopes(dataset_id,scope,revision) VALUES", " ON CONFLICT(dataset_id,scope) DO UPDATE SET revision=excluded.revision,error_code='',attempts=0,retry_at_ms=0", scopes); err != nil {
		return false, err
	}
	if err := writeRows(ctx, tx, "INSERT INTO processing_dependencies VALUES", " ON CONFLICT DO NOTHING", dependencies); err != nil {
		return false, err
	}
	if err := writeRows(ctx, tx, "INSERT INTO ingestion_items VALUES", " ON CONFLICT DO NOTHING", items); err != nil {
		return false, err
	}
	if err := writeRows(ctx, tx, "INSERT INTO ingestion_batch_items VALUES", "", mappings); err != nil {
		return false, err
	}
	return len(raw) > 0, nil
}

func factArguments(datasetID, table string, fact publication.Fact, generation, revision int64, evidenceID, reason string) ([]interface{}, error) {
	if err := publication.ValidateFact(fact); err != nil {
		return nil, err
	}
	body, err := json.Marshal(fact)
	if err != nil {
		return nil, err
	}
	message := ""
	if fact.Message != nil {
		message = fact.Message.NativeID
	}
	location := publication.Location{}
	if fact.Location != nil {
		location = *fact.Location
	}
	args := []interface{}{datasetID, fact.ID, evidence.SessionScope(fact.Harness, fact.Session.NativeID), fact.Harness, fact.Session.ID, fact.Session.NativeID, message, fact.NativeRequestID, fact.OccurredAtMs, fact.Provider, fact.ProviderSource, fact.Model, fact.UsageScope, fact.Quality, fact.Countable, fact.InputTokens, fact.OutputTokens, fact.ReasoningTokens, fact.CacheReadTokens, fact.CacheWriteTokens, fact.TotalTokens, location.DirectoryKey, location.DirectoryName, location.RepositoryKey, location.RepositoryName, location.RepositorySource, string(body), generation, revision}
	if table == "analytics_estimates" {
		args = append(args, evidenceID, reason)
	}
	return args, nil
}

type projectionRows struct {
	facts, estimates, provenance, outcomes, scopes [][]interface{}
}

func prepareProjection(work dataengine.Work, projection evidence.Projection) (projectionRows, error) {
	rows := projectionRows{}
	for _, contribution := range projection.Contributions {
		args, err := factArguments(work.DatasetID, "analytics_facts", contribution.Fact, work.Generation, work.Revision, "", "")
		if err != nil {
			return rows, err
		}
		rows.facts = append(rows.facts, args)
		seen := make(map[string]bool, len(contribution.EvidenceIDs))
		for _, id := range contribution.EvidenceIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			rows.provenance = append(rows.provenance, []interface{}{work.DatasetID, work.Generation, contribution.Fact.ID, id})
		}
	}
	for _, estimate := range projection.Estimates {
		args, err := factArguments(work.DatasetID, "analytics_estimates", estimate.Fact, work.Generation, work.Revision, estimate.EvidenceID, estimate.Code)
		if err != nil {
			return rows, err
		}
		rows.estimates = append(rows.estimates, args)
	}
	for _, outcome := range projection.Outcomes {
		rows.outcomes = append(rows.outcomes, []interface{}{work.DatasetID, outcome.EvidenceID, outcome.Disposition, outcome.Code, outcome.FactID, work.Generation, work.Revision})
	}
	for scope, revision := range work.Scopes {
		rows.scopes = append(rows.scopes, []interface{}{work.DatasetID, scope, revision, work.Generation})
	}
	return rows, nil
}
func publishRows(ctx context.Context, tx *sql.Tx, work dataengine.Work, rows projectionRows) error {
	scopes := make([]interface{}, 0, len(work.Scopes))
	for scope := range work.Scopes {
		scopes = append(scopes, scope)
	}
	for start := 0; start < len(scopes); start += sqlBatchRows {
		end := min(start+sqlBatchRows, len(scopes))
		args := []interface{}{work.DatasetID, work.Generation}
		args = append(args, scopes[start:end]...)
		filter := " WHERE dataset_id=? AND generation=? AND scope IN(" + sqlPlaceholders(end-start) + ")"
		provenanceArgs := []interface{}{work.DatasetID, work.Generation}
		provenanceArgs = append(provenanceArgs, args...)
		if _, err := tx.ExecContext(ctx, sqlutil.Bind("DELETE FROM analytics_provenance WHERE dataset_id=? AND generation=? AND fact_id IN(SELECT fact_id FROM analytics_facts"+filter+")"), provenanceArgs...); err != nil {
			return err
		}
		for _, table := range []string{"analytics_facts", "analytics_estimates"} {
			if _, err := tx.ExecContext(ctx, sqlutil.Bind("DELETE FROM "+table+filter), args...); err != nil {
				return err
			}
		}
	}
	if err := writeRows(ctx, tx, "INSERT INTO analytics_facts VALUES", "", rows.facts); err != nil {
		return fmt.Errorf("insert projected contribution: %w", err)
	}
	if err := writeRows(ctx, tx, "INSERT INTO analytics_estimates VALUES", "", rows.estimates); err != nil {
		return fmt.Errorf("insert projected contribution: %w", err)
	}
	// Old edges were deleted above; preparation deduplicates each fact's new edges.
	if err := writeRows(ctx, tx, "INSERT INTO analytics_provenance VALUES", "", rows.provenance); err != nil {
		return err
	}
	if err := writeRows(ctx, tx, "INSERT INTO processing_outcomes VALUES", " ON CONFLICT(dataset_id,generation,evidence_id) DO UPDATE SET disposition=excluded.disposition,code=excluded.code,fact_id=excluded.fact_id,input_revision=excluded.input_revision", rows.outcomes); err != nil {
		return err
	}
	statement, err := tx.PrepareContext(ctx, sqlutil.Bind("UPDATE processing_scopes SET processed_revision=?,generation=?,error_code='',attempts=0,retry_at_ms=0 WHERE dataset_id=? AND scope=?"))
	if err != nil {
		return err
	}
	defer func() { _ = statement.Close() }()
	for _, scope := range rows.scopes {
		if _, err := statement.ExecContext(ctx, scope[2], scope[3], scope[0], scope[1]); err != nil {
			return err
		}
	}
	return nil
}
