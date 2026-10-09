-- DuckDB server data schema, version 3. Current evidence and projections only.
CREATE SCHEMA raw;
CREATE SCHEMA ingestion;
CREATE SCHEMA processing;
CREATE SCHEMA analytics;
CREATE TABLE ingestion.instance (
 id INTEGER PRIMARY KEY CHECK(id=1), role VARCHAR NOT NULL CHECK(role='server-data'),
 schema_version INTEGER NOT NULL CHECK(schema_version=3), database_id VARCHAR NOT NULL,
 server_kind VARCHAR NOT NULL CHECK(server_kind IN('personal','hosted')),
 created_at_ms BIGINT NOT NULL
);
CREATE TABLE ingestion.metadata (
 dataset_id VARCHAR PRIMARY KEY, id INTEGER NOT NULL DEFAULT 1 CHECK(id=1),
 role VARCHAR NOT NULL CHECK(role='server-data'),
 schema_version INTEGER NOT NULL CHECK(schema_version=3), database_id VARCHAR NOT NULL,
 server_kind VARCHAR NOT NULL CHECK(server_kind IN('personal','hosted')),
 generation BIGINT NOT NULL, target_generation BIGINT NOT NULL,
 input_revision BIGINT NOT NULL DEFAULT 0, revision BIGINT NOT NULL DEFAULT 0,
 last_ingestion_at_ms BIGINT NOT NULL DEFAULT 0, created_at_ms BIGINT NOT NULL
);
CREATE TABLE raw.evidence (
 dataset_id VARCHAR NOT NULL, evidence_id VARCHAR NOT NULL, scope VARCHAR NOT NULL, harness VARCHAR NOT NULL,
 record_json VARCHAR NOT NULL, first_received_at_ms BIGINT NOT NULL,
 PRIMARY KEY(dataset_id,evidence_id)
);
CREATE INDEX evidence_scope ON raw.evidence(dataset_id,scope);
CREATE TABLE ingestion.batches (
 dataset_id VARCHAR NOT NULL, stream_id VARCHAR NOT NULL, batch_id VARCHAR NOT NULL, request_hash VARCHAR NOT NULL,
 request_bytes BLOB NOT NULL, receipt_json VARCHAR NOT NULL,
 PRIMARY KEY(dataset_id,stream_id,batch_id)
);
CREATE TABLE ingestion.items (
 dataset_id VARCHAR NOT NULL, stream_id VARCHAR NOT NULL, sequence BIGINT NOT NULL, evidence_id VARCHAR NOT NULL,
 PRIMARY KEY(dataset_id,stream_id,sequence)
);
CREATE TABLE ingestion.batch_items (
 dataset_id VARCHAR NOT NULL, stream_id VARCHAR NOT NULL, batch_id VARCHAR NOT NULL, sequence BIGINT NOT NULL,
 evidence_id VARCHAR NOT NULL, PRIMARY KEY(dataset_id,stream_id,batch_id,sequence)
);
CREATE INDEX batch_evidence ON ingestion.batch_items(dataset_id,evidence_id);
CREATE TABLE processing.scopes (
 dataset_id VARCHAR NOT NULL, scope VARCHAR NOT NULL, revision BIGINT NOT NULL,
 processed_revision BIGINT NOT NULL DEFAULT 0, generation BIGINT NOT NULL DEFAULT 0,
 error_code VARCHAR NOT NULL DEFAULT '',
 attempts BIGINT NOT NULL DEFAULT 0, retry_at_ms BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(dataset_id,scope)
);
CREATE TABLE processing.dependencies (
 dataset_id VARCHAR NOT NULL, child VARCHAR NOT NULL, parent VARCHAR NOT NULL, PRIMARY KEY(dataset_id,child,parent)
);
CREATE TABLE processing.outcomes (
 dataset_id VARCHAR NOT NULL, evidence_id VARCHAR NOT NULL, disposition VARCHAR NOT NULL,
 code VARCHAR NOT NULL, fact_id VARCHAR NOT NULL,
 generation BIGINT NOT NULL, input_revision BIGINT NOT NULL,
 PRIMARY KEY(dataset_id,generation,evidence_id)
);
CREATE TABLE analytics.generations (
 dataset_id VARCHAR NOT NULL, generation BIGINT NOT NULL, processor_version INTEGER NOT NULL,
 state VARCHAR NOT NULL CHECK(state IN('building','active','retained')),
 created_at_ms BIGINT NOT NULL, activated_at_ms BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(dataset_id,generation)
);
CREATE TABLE analytics.facts (
 dataset_id VARCHAR NOT NULL, fact_id VARCHAR NOT NULL, scope VARCHAR NOT NULL, harness VARCHAR NOT NULL,
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
 PRIMARY KEY(dataset_id,generation,fact_id)
);
CREATE TABLE analytics.estimates AS SELECT *,CAST('' AS VARCHAR) AS evidence_id,CAST('' AS VARCHAR) AS reason_code FROM analytics.facts WITH NO DATA;
CREATE UNIQUE INDEX estimate_identity ON analytics.estimates(dataset_id,generation,fact_id);
CREATE TABLE analytics.provenance (
 dataset_id VARCHAR NOT NULL, generation BIGINT NOT NULL, fact_id VARCHAR NOT NULL, evidence_id VARCHAR NOT NULL,
 PRIMARY KEY(dataset_id,generation,fact_id,evidence_id)
);
CREATE VIEW analytics.estimated AS SELECT estimates.* FROM analytics.estimates estimates
 JOIN ingestion.metadata metadata ON metadata.dataset_id=estimates.dataset_id AND metadata.generation=estimates.generation;
CREATE VIEW analytics.confirmed AS
 SELECT facts.* FROM analytics.facts facts
 JOIN ingestion.metadata metadata ON metadata.dataset_id=facts.dataset_id AND metadata.generation=facts.generation;
