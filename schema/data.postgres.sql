CREATE SCHEMA tokeninsights_data;
SET LOCAL search_path TO tokeninsights_data;
-- PostgreSQL token schema, version 1. Paired with account schema version 1.
CREATE TABLE ingestion_instance (
 id BIGINT PRIMARY KEY CHECK(id=1), role TEXT COLLATE "C" NOT NULL CHECK(role='server-data'),
 schema_version BIGINT NOT NULL CHECK(schema_version=1), database_id TEXT COLLATE "C" NOT NULL,
 server_kind TEXT COLLATE "C" NOT NULL CHECK(server_kind IN('personal','hosted')),
 created_at_ms BIGINT NOT NULL, application_instance_id TEXT COLLATE "C" NOT NULL
);
CREATE TABLE ingestion_metadata (
 dataset_id TEXT COLLATE "C" PRIMARY KEY, id BIGINT NOT NULL DEFAULT 1 CHECK(id=1),
 role TEXT COLLATE "C" NOT NULL CHECK(role='server-data'),
 schema_version BIGINT NOT NULL CHECK(schema_version=1), database_id TEXT COLLATE "C" NOT NULL,
 server_kind TEXT COLLATE "C" NOT NULL CHECK(server_kind IN('personal','hosted')),
 generation BIGINT NOT NULL, target_generation BIGINT NOT NULL,
 input_revision BIGINT NOT NULL DEFAULT 0, revision BIGINT NOT NULL DEFAULT 0,
 last_ingestion_at_ms BIGINT NOT NULL DEFAULT 0, created_at_ms BIGINT NOT NULL
);
CREATE TABLE raw_evidence (
 dataset_id TEXT COLLATE "C" NOT NULL, evidence_id TEXT COLLATE "C" NOT NULL, scope TEXT COLLATE "C" NOT NULL, harness TEXT COLLATE "C" NOT NULL,
 record_json TEXT COLLATE "C" NOT NULL, first_received_at_ms BIGINT NOT NULL,
 PRIMARY KEY(dataset_id,evidence_id)
);
CREATE INDEX evidence_scope ON raw_evidence(dataset_id,scope);
CREATE TABLE ingestion_batches (
 dataset_id TEXT COLLATE "C" NOT NULL, stream_id TEXT COLLATE "C" NOT NULL, batch_id TEXT COLLATE "C" NOT NULL, request_hash TEXT COLLATE "C" NOT NULL,
 request_bytes BYTEA NOT NULL, receipt_json TEXT COLLATE "C" NOT NULL,
 PRIMARY KEY(dataset_id,stream_id,batch_id)
);
CREATE TABLE ingestion_items (
 dataset_id TEXT COLLATE "C" NOT NULL, stream_id TEXT COLLATE "C" NOT NULL, sequence BIGINT NOT NULL, evidence_id TEXT COLLATE "C" NOT NULL,
 PRIMARY KEY(dataset_id,stream_id,sequence)
);
CREATE TABLE ingestion_batch_items (
 dataset_id TEXT COLLATE "C" NOT NULL, stream_id TEXT COLLATE "C" NOT NULL, batch_id TEXT COLLATE "C" NOT NULL, sequence BIGINT NOT NULL,
 evidence_id TEXT COLLATE "C" NOT NULL, PRIMARY KEY(dataset_id,stream_id,batch_id,sequence)
);
CREATE INDEX batch_evidence ON ingestion_batch_items(dataset_id,evidence_id);
CREATE TABLE processing_scopes (
 dataset_id TEXT COLLATE "C" NOT NULL, scope TEXT COLLATE "C" NOT NULL, revision BIGINT NOT NULL,
 processed_revision BIGINT NOT NULL DEFAULT 0, generation BIGINT NOT NULL DEFAULT 0,
 error_code TEXT COLLATE "C" NOT NULL DEFAULT '',
 attempts BIGINT NOT NULL DEFAULT 0, retry_at_ms BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(dataset_id,scope)
);
CREATE TABLE processing_dependencies (
 dataset_id TEXT COLLATE "C" NOT NULL, child TEXT COLLATE "C" NOT NULL, parent TEXT COLLATE "C" NOT NULL, PRIMARY KEY(dataset_id,child,parent)
);
CREATE TABLE processing_outcomes (
 dataset_id TEXT COLLATE "C" NOT NULL, evidence_id TEXT COLLATE "C" NOT NULL, disposition TEXT COLLATE "C" NOT NULL,
 code TEXT COLLATE "C" NOT NULL, fact_id TEXT COLLATE "C" NOT NULL,
 generation BIGINT NOT NULL, input_revision BIGINT NOT NULL,
 PRIMARY KEY(dataset_id,generation,evidence_id)
);
CREATE TABLE analytics_generations (
 dataset_id TEXT COLLATE "C" NOT NULL, generation BIGINT NOT NULL, processor_version BIGINT NOT NULL,
 state TEXT COLLATE "C" NOT NULL CHECK(state IN('building','active','retained')),
 created_at_ms BIGINT NOT NULL, activated_at_ms BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(dataset_id,generation)
);
CREATE TABLE analytics_facts (
 dataset_id TEXT COLLATE "C" NOT NULL, fact_id TEXT COLLATE "C" NOT NULL, scope TEXT COLLATE "C" NOT NULL, harness TEXT COLLATE "C" NOT NULL,
 session_id TEXT COLLATE "C" NOT NULL, session_native_id TEXT COLLATE "C" NOT NULL,
 message_native_id TEXT COLLATE "C" NOT NULL, native_request_id TEXT COLLATE "C" NOT NULL,
 occurred_at_ms BIGINT NOT NULL, provider TEXT COLLATE "C" NOT NULL, provider_source TEXT COLLATE "C" NOT NULL,
 model TEXT COLLATE "C" NOT NULL, usage_scope TEXT COLLATE "C" NOT NULL, quality TEXT COLLATE "C" NOT NULL,
 countable BOOLEAN NOT NULL, input_tokens BIGINT NOT NULL CHECK(input_tokens>=0),
 output_tokens BIGINT NOT NULL CHECK(output_tokens>=0), reasoning_tokens BIGINT NOT NULL CHECK(reasoning_tokens>=0),
 cache_read_tokens BIGINT NOT NULL CHECK(cache_read_tokens>=0), cache_write_tokens BIGINT NOT NULL CHECK(cache_write_tokens>=0),
 total_tokens BIGINT NOT NULL CHECK(total_tokens=input_tokens+output_tokens+reasoning_tokens+cache_read_tokens+cache_write_tokens),
 directory_key TEXT COLLATE "C" NOT NULL, directory_name TEXT COLLATE "C" NOT NULL, repository_key TEXT COLLATE "C" NOT NULL,
 repository_name TEXT COLLATE "C" NOT NULL, repository_source TEXT COLLATE "C" NOT NULL,
 payload_json TEXT COLLATE "C" NOT NULL, generation BIGINT NOT NULL, input_revision BIGINT NOT NULL,
 PRIMARY KEY(dataset_id,generation,fact_id)
);
CREATE TABLE analytics_estimates (
 dataset_id TEXT COLLATE "C" NOT NULL, fact_id TEXT COLLATE "C" NOT NULL, scope TEXT COLLATE "C" NOT NULL, harness TEXT COLLATE "C" NOT NULL,
 session_id TEXT COLLATE "C" NOT NULL, session_native_id TEXT COLLATE "C" NOT NULL,
 message_native_id TEXT COLLATE "C" NOT NULL, native_request_id TEXT COLLATE "C" NOT NULL,
 occurred_at_ms BIGINT NOT NULL, provider TEXT COLLATE "C" NOT NULL, provider_source TEXT COLLATE "C" NOT NULL,
 model TEXT COLLATE "C" NOT NULL, usage_scope TEXT COLLATE "C" NOT NULL, quality TEXT COLLATE "C" NOT NULL,
 countable BOOLEAN NOT NULL, input_tokens BIGINT NOT NULL CHECK(input_tokens>=0),
 output_tokens BIGINT NOT NULL CHECK(output_tokens>=0), reasoning_tokens BIGINT NOT NULL CHECK(reasoning_tokens>=0),
 cache_read_tokens BIGINT NOT NULL CHECK(cache_read_tokens>=0), cache_write_tokens BIGINT NOT NULL CHECK(cache_write_tokens>=0),
 total_tokens BIGINT NOT NULL CHECK(total_tokens=input_tokens+output_tokens+reasoning_tokens+cache_read_tokens+cache_write_tokens),
 directory_key TEXT COLLATE "C" NOT NULL, directory_name TEXT COLLATE "C" NOT NULL, repository_key TEXT COLLATE "C" NOT NULL,
 repository_name TEXT COLLATE "C" NOT NULL, repository_source TEXT COLLATE "C" NOT NULL,
 payload_json TEXT COLLATE "C" NOT NULL, generation BIGINT NOT NULL, input_revision BIGINT NOT NULL,
 evidence_id TEXT COLLATE "C" NOT NULL, reason_code TEXT COLLATE "C" NOT NULL,
 PRIMARY KEY(dataset_id,generation,fact_id)
);
CREATE TABLE analytics_provenance (
 dataset_id TEXT COLLATE "C" NOT NULL, generation BIGINT NOT NULL, fact_id TEXT COLLATE "C" NOT NULL, evidence_id TEXT COLLATE "C" NOT NULL,
 PRIMARY KEY(dataset_id,generation,fact_id,evidence_id)
);
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
