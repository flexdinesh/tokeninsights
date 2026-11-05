import { z } from 'zod'

// The native bridge supplies safe display metadata, never a credential-bearing URI.
export const viewerConnectionSchema = z
  .object({
    transport: z.enum(['local', 'ssh', 'tcp']),
    target: z.string().min(1),
    state: z.string().min(1),
    hostname: z.string().optional(),
  })
  .strict()

export type ViewerConnection = z.output<typeof viewerConnectionSchema>
