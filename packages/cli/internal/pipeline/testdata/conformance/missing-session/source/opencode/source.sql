CREATE TABLE message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  data text NOT NULL
);

INSERT INTO message (id, session_id, time_created, time_updated, data)
VALUES (
  'm1',
  '',
  1770000000000,
  1770000000000,
  '{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":10,"output":5},"time":{"created":1770000000000}}'
);
