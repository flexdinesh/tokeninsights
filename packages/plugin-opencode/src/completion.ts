// V2 completion routing only. No transcript, location, or native session ID
// crosses this boundary: sync discovers its configured durable sources.
function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function isCompletion(event: unknown): boolean {
  return (
    record(event) &&
    event.type === 'session.status' &&
    record(event.data) &&
    typeof event.data.sessionID === 'string' &&
    event.data.sessionID.length > 0 &&
    record(event.data.status) &&
    event.data.status.type === 'idle'
  )
}
