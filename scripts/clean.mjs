import { rm } from "node:fs/promises"

const paths = [
  "packages/plugins/opencode-server/dist",
  "packages/plugins/opencode-server/deploy",
  "packages/plugins/pi/dist",
]

await Promise.all(paths.map((path) => rm(path, { recursive: true, force: true })))
