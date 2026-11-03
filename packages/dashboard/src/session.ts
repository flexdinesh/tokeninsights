import type { QueryClient } from '@tanstack/react-query'

export async function clearAccountQueries(client: QueryClient): Promise<void> {
  await client.cancelQueries()
  client.clear()
}
