import { rm } from "node:fs/promises"

const paths = [
  "packages/cli/tokeninsights-cli",
]

await Promise.all(paths.map((path) => rm(path, { recursive: true, force: true })))
