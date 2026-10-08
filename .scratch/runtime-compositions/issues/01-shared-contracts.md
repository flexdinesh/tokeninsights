# Shared transport, query and lifetime foundations

Status: complete
Dependencies: none

Extract collector Delivery with capabilities, exact-byte submission, receipt lookup
and retained legacy negotiation. Default HTTP behavior stays compatible; add direct
calls to the same dataset-bound acceptance. Direct admission/limits and errors must
match HTTP. No httptest/loopback/custom RoundTripper in production direct paths.

Introduce analytics.Repository and DuckDB implementation. REST and direct readers
share filter validation, result mapping and snapshot-aware pagination. Keep SQL in
the adapter and identity/counter semantics unchanged. Remove replaced wrappers.

Add localruntime with canonical path validation, lifecycle admission lock, lifetime
lock, owned store/worker and direct delivery/query handles. Open no listener.
Close cancels and joins before closing storage/releasing ownership. Read visibility
and generation status in a single consistent snapshot.

Verification: existing collector tests plus native golden direct replay/rebuild,
lost reply, HTTP/direct receipt/error parity, all-tab query/facet parity, pending
work/generation restart and single ownership/reopen. Full format/lint/test/build.

Implementation: complete. Root format/lint/full tests/build, schema/API checks,
focused race tests and native JS-absent smoke passed. See PRD validation record.

## Unresolved questions

None.
