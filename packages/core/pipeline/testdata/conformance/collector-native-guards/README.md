# Native evidence guards

Synthetic, metadata-only Claude Code records. No conversation content or real
identifiers. Native identity is `fixture-session` / `fixture-message` /
`fixture-request`; filenames may guess the same session without proving it.

`claude-weak-and-native.jsonl` contains three filename-derived observations and
one native observation. Expected: four retained raw facts, one canonical and
published fact. Only the native timestamp (`1767225601000`) affects session and
message envelopes. Native input 10 plus inclusive output 10 gives total 20;
reasoning 2 leaves non-reasoning output 8. Weak output 6/7/8 never replaces it.

`claude-equivalent-counters.jsonl` contains six representations of one native
snapshot: omitted optional counters, explicit zero counters, and an explicit
valid total. Expected canonical components: input 10, output 5, reasoning 0,
cache read 0, cache write 0, total 15. These records must never conflict.

`native_guards_test.go` exercises every weak/native permutation, separate syncs,
retained raw normalization, 100 full reparses, copied artifacts, fresh collector
rebuilds, canonical attribution comparisons, and invalid timestamp siblings.
Expected counts and component arithmetic are explicit in tests; they are not
generated from parser output.
