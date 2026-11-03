-- SQLite token data schema, version 1. Current evidence and projections only.
CREATE TABLE ingestion_instance (
 id INTEGER PRIMARY KEY CHECK(id=1), role TEXT NOT NULL CHECK(role='server-data'),
 schema_version INTEGER NOT NULL CHECK(schema_version=1), database_id TEXT NOT NULL,
 server_kind TEXT NOT NULL CHECK(server_kind IN('personal','hosted')),
 created_at_ms INTEGER NOT NULL
) STRICT;
CREATE TABLE ingestion_metadata (
 dataset_id TEXT PRIMARY KEY, id INTEGER NOT NULL DEFAULT 1 CHECK(id=1),
 role TEXT NOT NULL CHECK(role='server-data'),
 schema_version INTEGER NOT NULL CHECK(schema_version=1), database_id TEXT NOT NULL,
 server_kind TEXT NOT NULL CHECK(server_kind IN('personal','hosted')),
 generation INTEGER NOT NULL, target_generation INTEGER NOT NULL,
 input_revision INTEGER NOT NULL DEFAULT 0, revision INTEGER NOT NULL DEFAULT 0,
 last_ingestion_at_ms INTEGER NOT NULL DEFAULT 0, created_at_ms INTEGER NOT NULL
) STRICT;
CREATE TABLE raw_evidence (
 dataset_id TEXT NOT NULL, evidence_id TEXT NOT NULL, scope TEXT NOT NULL, harness TEXT NOT NULL,
 record_json TEXT NOT NULL, first_received_at_ms INTEGER NOT NULL,
 PRIMARY KEY(dataset_id,evidence_id)
) STRICT;
CREATE INDEX evidence_scope ON raw_evidence(dataset_id,scope);
CREATE TABLE ingestion_batches (
 dataset_id TEXT NOT NULL, stream_id TEXT NOT NULL, batch_id TEXT NOT NULL, request_hash TEXT NOT NULL,
 request_bytes BLOB NOT NULL, receipt_json TEXT NOT NULL,
 PRIMARY KEY(dataset_id,stream_id,batch_id)
) STRICT;
CREATE TABLE ingestion_items (
 dataset_id TEXT NOT NULL, stream_id TEXT NOT NULL, sequence INTEGER NOT NULL, evidence_id TEXT NOT NULL,
 PRIMARY KEY(dataset_id,stream_id,sequence)
) STRICT;
CREATE TABLE ingestion_batch_items (
 dataset_id TEXT NOT NULL, stream_id TEXT NOT NULL, batch_id TEXT NOT NULL, sequence INTEGER NOT NULL,
 evidence_id TEXT NOT NULL, PRIMARY KEY(dataset_id,stream_id,batch_id,sequence)
) STRICT;
CREATE INDEX batch_evidence ON ingestion_batch_items(dataset_id,evidence_id);
CREATE TABLE processing_scopes (
 dataset_id TEXT NOT NULL, scope TEXT NOT NULL, revision INTEGER NOT NULL,
 processed_revision INTEGER NOT NULL DEFAULT 0, generation INTEGER NOT NULL DEFAULT 0,
 error_code TEXT NOT NULL DEFAULT '',
 attempts INTEGER NOT NULL DEFAULT 0, retry_at_ms INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(dataset_id,scope)
) STRICT;
CREATE TABLE processing_dependencies (
 dataset_id TEXT NOT NULL, child TEXT NOT NULL, parent TEXT NOT NULL, PRIMARY KEY(dataset_id,child,parent)
) STRICT;
CREATE TABLE processing_outcomes (
 dataset_id TEXT NOT NULL, evidence_id TEXT NOT NULL, disposition TEXT NOT NULL,
 code TEXT NOT NULL, fact_id TEXT NOT NULL,
 generation INTEGER NOT NULL, input_revision INTEGER NOT NULL,
 PRIMARY KEY(dataset_id,generation,evidence_id)
) STRICT;
CREATE TABLE analytics_generations (
 dataset_id TEXT NOT NULL, generation INTEGER NOT NULL, processor_version INTEGER NOT NULL,
 state TEXT NOT NULL CHECK(state IN('building','active','retained')),
 created_at_ms INTEGER NOT NULL, activated_at_ms INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(dataset_id,generation)
) STRICT;
CREATE TABLE analytics_facts (
 dataset_id TEXT NOT NULL, fact_id TEXT NOT NULL, scope TEXT NOT NULL, harness TEXT NOT NULL,
 session_id TEXT NOT NULL, session_native_id TEXT NOT NULL,
 message_native_id TEXT NOT NULL, native_request_id TEXT NOT NULL,
 occurred_at_ms INTEGER NOT NULL, provider TEXT NOT NULL, provider_source TEXT NOT NULL,
 model TEXT NOT NULL, usage_scope TEXT NOT NULL, quality TEXT NOT NULL,
 countable INTEGER NOT NULL CHECK(countable IN(0,1)), input_tokens INTEGER NOT NULL CHECK(input_tokens>=0),
 output_tokens INTEGER NOT NULL CHECK(output_tokens>=0), reasoning_tokens INTEGER NOT NULL CHECK(reasoning_tokens>=0),
 cache_read_tokens INTEGER NOT NULL CHECK(cache_read_tokens>=0), cache_write_tokens INTEGER NOT NULL CHECK(cache_write_tokens>=0),
 total_tokens INTEGER NOT NULL CHECK(total_tokens=input_tokens+output_tokens+reasoning_tokens+cache_read_tokens+cache_write_tokens),
 directory_key TEXT NOT NULL, directory_name TEXT NOT NULL, repository_key TEXT NOT NULL,
 repository_name TEXT NOT NULL, repository_source TEXT NOT NULL,
 payload_json TEXT NOT NULL, generation INTEGER NOT NULL, input_revision INTEGER NOT NULL,
 PRIMARY KEY(dataset_id,generation,fact_id)
) STRICT;
CREATE TABLE analytics_estimates (
 dataset_id TEXT NOT NULL, fact_id TEXT NOT NULL, scope TEXT NOT NULL, harness TEXT NOT NULL,
 session_id TEXT NOT NULL, session_native_id TEXT NOT NULL,
 message_native_id TEXT NOT NULL, native_request_id TEXT NOT NULL,
 occurred_at_ms INTEGER NOT NULL, provider TEXT NOT NULL, provider_source TEXT NOT NULL,
 model TEXT NOT NULL, usage_scope TEXT NOT NULL, quality TEXT NOT NULL,
 countable INTEGER NOT NULL CHECK(countable IN(0,1)), input_tokens INTEGER NOT NULL CHECK(input_tokens>=0),
 output_tokens INTEGER NOT NULL CHECK(output_tokens>=0), reasoning_tokens INTEGER NOT NULL CHECK(reasoning_tokens>=0),
 cache_read_tokens INTEGER NOT NULL CHECK(cache_read_tokens>=0), cache_write_tokens INTEGER NOT NULL CHECK(cache_write_tokens>=0),
 total_tokens INTEGER NOT NULL CHECK(total_tokens=input_tokens+output_tokens+reasoning_tokens+cache_read_tokens+cache_write_tokens),
 directory_key TEXT NOT NULL, directory_name TEXT NOT NULL, repository_key TEXT NOT NULL,
 repository_name TEXT NOT NULL, repository_source TEXT NOT NULL,
 payload_json TEXT NOT NULL, generation INTEGER NOT NULL, input_revision INTEGER NOT NULL,
 evidence_id TEXT NOT NULL, reason_code TEXT NOT NULL,
 PRIMARY KEY(dataset_id,generation,fact_id)
) STRICT;
CREATE TABLE analytics_provenance (
 dataset_id TEXT NOT NULL, generation INTEGER NOT NULL, fact_id TEXT NOT NULL, evidence_id TEXT NOT NULL,
 PRIMARY KEY(dataset_id,generation,fact_id,evidence_id)
) STRICT;
CREATE VIEW analytics_estimated AS SELECT estimates.* FROM analytics_estimates estimates
 JOIN ingestion_metadata metadata ON metadata.dataset_id=estimates.dataset_id AND metadata.generation=estimates.generation;
CREATE VIEW analytics_confirmed AS
 SELECT facts.* FROM analytics_facts facts
 JOIN ingestion_metadata metadata ON metadata.dataset_id=facts.dataset_id AND metadata.generation=facts.generation;

CREATE INDEX dependency_parent ON processing_dependencies(dataset_id,parent);
CREATE INDEX pending_scopes ON processing_scopes(dataset_id,generation,retry_at_ms,scope);
CREATE INDEX facts_scope ON analytics_facts(dataset_id,generation,scope);
CREATE INDEX facts_time ON analytics_facts(dataset_id,generation,occurred_at_ms);
CREATE INDEX estimates_scope ON analytics_estimates(dataset_id,generation,scope);
CREATE INDEX estimates_time ON analytics_estimates(dataset_id,generation,occurred_at_ms);
CREATE INDEX provenance_evidence ON analytics_provenance(dataset_id,generation,evidence_id);
PRAGMA application_id = 1414091604;
PRAGMA user_version = 1;
