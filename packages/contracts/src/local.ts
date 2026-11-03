import { z } from 'zod'

// Local process contract only. Hosted HTTP shapes remain generated unchanged.
export const localBackendDescriptorSchema = z
  .object({
    url: z.url().refine((value) => {
      const url = new URL(value)
      return (
        url.protocol === 'http:' &&
        ['127.0.0.1', '[::1]', 'localhost'].includes(url.hostname) &&
        !url.username &&
        !url.password &&
        url.port !== '' &&
        url.pathname === '/' &&
        !url.search &&
        !url.hash
      )
    }, 'Expected an HTTP loopback backend origin'),
    instanceId: z.string().min(1),
    protocolVersion: z.literal(2),
  })
  .strict()
export type LocalBackendDescriptor = z.output<typeof localBackendDescriptorSchema>
