-- DuckDB server data schema, version 1. Legacy server.sql remains read-only import format.
CREATE SCHEMA raw;
CREATE SCHEMA ingestion;
CREATE SCHEMA processing;
CREATE SCHEMA analytics;

CREATE TABLE ingestion.metadata (
 id INTEGER PRIMARY KEY CHECK(id=1), role VARCHAR NOT NULL CHECK(role='server-data'),
 schema_version INTEGER NOT NULL CHECK(schema_version=1), database_id VARCHAR NOT NULL,
 dataset_id VARCHAR NOT NULL CHECK(dataset_id='default'), generation BIGINT NOT NULL,
 target_generation BIGINT NOT NULL,
 input_revision BIGINT NOT NULL DEFAULT 0, revision BIGINT NOT NULL DEFAULT 0,
 last_ingestion_at_ms BIGINT NOT NULL DEFAULT 0, created_at_ms BIGINT NOT NULL
);
CREATE TABLE raw.evidence (
 evidence_id VARCHAR PRIMARY KEY, scope VARCHAR NOT NULL, harness VARCHAR NOT NULL,
 record_json VARCHAR NOT NULL, first_received_at_ms BIGINT NOT NULL
);
CREATE INDEX evidence_scope ON raw.evidence(scope);
CREATE TABLE ingestion.batches (
 stream_id VARCHAR NOT NULL, batch_id VARCHAR NOT NULL, request_hash VARCHAR NOT NULL,
 request_bytes BLOB NOT NULL, receipt_json VARCHAR NOT NULL,
 PRIMARY KEY(stream_id,batch_id)
);
CREATE TABLE ingestion.items (
 stream_id VARCHAR NOT NULL, sequence BIGINT NOT NULL, evidence_id VARCHAR NOT NULL,
 PRIMARY KEY(stream_id,sequence)
);
CREATE TABLE ingestion.batch_items (
 stream_id VARCHAR NOT NULL, batch_id VARCHAR NOT NULL, sequence BIGINT NOT NULL,
 evidence_id VARCHAR NOT NULL, PRIMARY KEY(stream_id,batch_id,sequence)
);
CREATE INDEX batch_evidence ON ingestion.batch_items(evidence_id);
CREATE TABLE ingestion.legacy_receipts (
 stream_id VARCHAR NOT NULL, batch_id VARCHAR NOT NULL, request_hash VARCHAR NOT NULL,
 receipt_json VARCHAR NOT NULL, PRIMARY KEY(stream_id,batch_id)
);
CREATE TABLE processing.scopes (
 scope VARCHAR PRIMARY KEY, revision BIGINT NOT NULL,
 processed_revision BIGINT NOT NULL DEFAULT 0, generation BIGINT NOT NULL DEFAULT 0,
 error_code VARCHAR NOT NULL DEFAULT '',
 attempts BIGINT NOT NULL DEFAULT 0, retry_at_ms BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE processing.dependencies (
 child VARCHAR NOT NULL, parent VARCHAR NOT NULL, PRIMARY KEY(child,parent)
);
CREATE TABLE processing.outcomes (
 evidence_id VARCHAR NOT NULL, disposition VARCHAR NOT NULL,
 code VARCHAR NOT NULL, fact_id VARCHAR NOT NULL,
 generation BIGINT NOT NULL, input_revision BIGINT NOT NULL,
 PRIMARY KEY(generation,evidence_id)
);
CREATE TABLE analytics.generations (
 generation BIGINT PRIMARY KEY, processor_version INTEGER NOT NULL,
 state VARCHAR NOT NULL CHECK(state IN('building','active','retained')),
 created_at_ms BIGINT NOT NULL, activated_at_ms BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE analytics.facts (
 fact_id VARCHAR NOT NULL, scope VARCHAR NOT NULL, harness VARCHAR NOT NULL,
 session_id VARCHAR NOT NULL, session_native_id VARCHAR NOT NULL,
 message_native_id VARCHAR NOT NULL, native_request_id VARCHAR NOT NULL,
 occurred_at_ms BIGINT NOT NULL, provider VARCHAR NOT NULL, provider_source VARCHAR NOT NULL,
 model VARCHAR NOT NULL, usage_scope VARCHAR NOT NULL, quality VARCHAR NOT NULL,
 countable BOOLEAN NOT NULL, input_tokens BIGINT NOT NULL CHECK(input_tokens>=0),
 output_tokens BIGINT NOT NULL CHECK(output_tokens>=0), reasoning_tokens BIGINT NOT NULL CHECK(reasoning_tokens>=0),
 cache_read_tokens BIGINT NOT NULL CHECK(cache_read_tokens>=0), cache_write_tokens BIGINT NOT NULL CHECK(cache_write_tokens>=0),
 total_tokens BIGINT NOT NULL CHECK(total_tokens=input_tokens+output_tokens+reasoning_tokens+cache_read_tokens+cache_write_tokens),
 directory_key VARCHAR NOT NULL, directory_name VARCHAR NOT NULL, repository_key VARCHAR NOT NULL,
 repository_name VARCHAR NOT NULL, repository_source VARCHAR NOT NULL,
 payload_json VARCHAR NOT NULL, generation BIGINT NOT NULL, input_revision BIGINT NOT NULL,
 PRIMARY KEY(generation,fact_id)
);
CREATE TABLE analytics.estimates AS SELECT *,CAST('' AS VARCHAR) AS evidence_id,CAST('' AS VARCHAR) AS reason_code FROM analytics.facts WITH NO DATA;
CREATE UNIQUE INDEX estimate_identity ON analytics.estimates(generation,fact_id);
CREATE TABLE analytics.provenance (
 generation BIGINT NOT NULL, fact_id VARCHAR NOT NULL, evidence_id VARCHAR NOT NULL,
 PRIMARY KEY(generation,fact_id,evidence_id)
);
CREATE TABLE analytics.legacy AS SELECT * FROM analytics.facts WITH NO DATA;
CREATE UNIQUE INDEX legacy_identity ON analytics.legacy(fact_id);
CREATE TABLE analytics.legacy_coverage (
 generation BIGINT NOT NULL, fact_id VARCHAR NOT NULL, payload_hash VARCHAR NOT NULL,
 PRIMARY KEY(generation,fact_id)
);
CREATE VIEW analytics.estimated AS SELECT * FROM analytics.estimates
 WHERE generation=(SELECT generation FROM ingestion.metadata WHERE id=1);
-- Proven identity coverage replaces a baseline contribution without summing it twice.
CREATE VIEW analytics.confirmed AS
 SELECT facts.* FROM analytics.facts facts
 WHERE facts.generation=(SELECT generation FROM ingestion.metadata WHERE id=1)
 AND (NOT EXISTS(SELECT 1 FROM analytics.legacy legacy WHERE legacy.fact_id=facts.fact_id)
 OR EXISTS(SELECT 1 FROM analytics.legacy_coverage coverage WHERE coverage.generation=facts.generation AND coverage.fact_id=facts.fact_id AND coverage.payload_hash=sha256(facts.payload_json)))
 UNION ALL SELECT legacy.* FROM analytics.legacy legacy
 WHERE NOT EXISTS(SELECT 1 FROM analytics.facts facts JOIN analytics.legacy_coverage coverage ON coverage.generation=facts.generation AND coverage.fact_id=facts.fact_id AND coverage.payload_hash=sha256(facts.payload_json) WHERE facts.fact_id=legacy.fact_id AND facts.generation=(SELECT generation FROM ingestion.metadata WHERE id=1));
