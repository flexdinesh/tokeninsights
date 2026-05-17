export type StreamSample = {
  at: number
  tokens: number
}

export type TokenStorageConfig = {
  dbPath: string
  retentionDays: number
}
