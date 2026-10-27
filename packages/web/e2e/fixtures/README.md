Synthetic, public TLS certificate/key for isolated browser tests only.
Never use these credentials outside the test HTTPS proxy.

Playwright global setup owns two Go processes and an HTTPS proxy. All bind port 0;
the proxy forwards to the URL printed by the hosted process. A private temporary
home holds databases, the owner socket and generated account credentials. Setup
writes private metadata beneath a unique test-results directory, inherited by
workers, and teardown removes credentials, databases and metadata after joining
the children. Failure screenshots/traces and bounded server logs remain per run.

Default: one Chromium worker. Override deliberately with
`mise exec -- pnpm --filter @tokeninsights/web exec playwright test --workers=2`.
Tests isolate browser contexts; account admission addresses are unique per case.
Do not share `TOKENINSIGHTS_E2E_RUN` between simultaneous invocations.
