CREATE TABLE message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  data text NOT NULL
);

CREATE TABLE session_message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  type text NOT NULL,
  seq integer NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  data text NOT NULL
);

INSERT INTO message (id, session_id, time_created, time_updated, data)
VALUES (
  'fixture-oc-message-v1',
  'fixture-oc-session-v1',
  1767225601000,
  1767225602000,
  '{"role":"assistant","providerID":"fixture-provider-alpha","modelID":"fixture-model-alpha","tokens":{"input":101,"output":21,"reasoning":5,"cache":{"read":8,"write":3}},"time":{"created":1767225601000,"completed":1767225602000}}'
);

INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data)
VALUES (
  'fixture-oc-message-v2',
  'fixture-oc-session-v2',
  'assistant',
  1,
  1767312001000,
  1767312002000,
  '{"model":{"providerID":"fixture-provider-beta","id":"fixture-model-beta"},"tokens":{"input":202,"output":32,"reasoning":7,"cache":{"read":13,"write":4}},"time":{"created":1767312001000,"completed":1767312002000}}'
);
