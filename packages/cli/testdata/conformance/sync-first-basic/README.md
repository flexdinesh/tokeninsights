# Public Development Fixture

This compact fixture mirrors only the durable record shapes needed by the OpenCode, Pi, Codex, and Claude Code adapters. The shapes were derived from local harness sources; every retained identifier, provider, model, timestamp, and token count is synthetic.

Allowed source data is limited to session/message identity, record type, required timestamps, provider/model labels, token counters, and structural fields required by the parsers. The fixture intentionally excludes prompts, responses, thinking, tool calls and results, request headers, secrets, costs, working directories, repository metadata, user names, and raw provider payloads.

Never commit raw harness databases or transcripts. Add coverage by constructing synthetic records here and updating the language-neutral expected JSON outputs.
