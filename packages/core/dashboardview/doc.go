// Package dashboardview is the pure presentation model shared by native
// dashboard views. It mirrors the web dashboard's TypeScript rules (number,
// date and timezone formatting, calendar ranges, query state, status priority,
// columns, chart and filter rules) without UI toolkit, transport or storage
// dependencies. Shared JSON fixtures in packages/contracts/fixtures/dashboard-view
// pin both implementations to the same observable output (ADR 0032).
package dashboardview
