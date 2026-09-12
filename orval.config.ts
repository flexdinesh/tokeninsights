import { defineConfig } from 'orval'

const target = process.env.TOKENINSIGHTS_API_OUTPUT ?? './packages/web/src/generated/api.ts'

export default defineConfig({
  api: {
    input: {
      target: './docs/openapi.yaml',
    },
    output: {
      clean: true,
      client: 'zod',
      mode: 'single',
      target,
      override: {
        zod: {
          generate: {
            body: true,
            header: true,
            param: true,
            query: true,
            response: true,
          },
          generateCompanionTypes: true,
          generateReusableSchemas: true,
          strict: {
            body: true,
            header: true,
            param: true,
            query: true,
            response: true,
          },
          variant: 'classic',
          version: 4,
        },
      },
    },
  },
})
