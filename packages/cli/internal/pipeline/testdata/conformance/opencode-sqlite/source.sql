CREATE TABLE session (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  slug text NOT NULL,
  directory text NOT NULL,
  title text NOT NULL,
  version text NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL
);

CREATE TABLE message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  data text NOT NULL
);

INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
VALUES ('ses_a', 'proj_a', 'private-slug', '/private/project', 'private title', '1.0.0', 1700000000000, 1700000002000);

INSERT INTO message (id, session_id, time_created, time_updated, data)
VALUES (
  'msg_a',
  'ses_a',
  1700000000100,
  1700000000500,
  '{"id":"assistant_a","role":"assistant","modelID":"claude-sonnet-4","providerID":"anthropic","tokens":{"input":100,"output":50,"reasoning":10,"cache":{"read":20,"write":5}},"time":{"created":1700000000000,"completed":1700000000450}}'
);

INSERT INTO message (id, session_id, time_created, time_updated, data)
VALUES (
  'msg_b',
  'ses_a',
  1700000001000,
  1700000001300,
  '{"role":"assistant","tokens":{"input":7,"output":8,"reasoning":0,"cache":{"read":0,"write":2}},"time":{"created":1700000001000,"completed":1700000001300}}'
);

INSERT INTO message (id, session_id, time_created, time_updated, data)
VALUES (
  'msg_user',
  'ses_a',
  1700000002000,
  1700000002000,
  '{"role":"user","tokens":{"input":999,"output":999},"time":{"created":1700000002000}}'
);
