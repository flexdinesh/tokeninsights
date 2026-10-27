import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath } from 'node:url'

const testWorkers = process.env.TOKENINSIGHTS_VITEST_WORKERS ?? '2'
if (!/^[1-9]\d*$/.test(testWorkers) || !Number.isSafeInteger(Number(testWorkers))) {
  throw new Error('TOKENINSIGHTS_VITEST_WORKERS must be a positive integer')
}

export default defineConfig(({ command, mode }) => {
  const apiURL = process.env.TOKENINSIGHTS_DEV_API_URL
  if (command === 'serve' && mode !== 'mock' && mode !== 'test' && !apiURL) {
    throw new Error(
      'Set TOKENINSIGHTS_DEV_API_URL to the URL printed by mise run dev:server, or use mise run dev',
    )
  }
  return {
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
    },
    server: {
      host: '127.0.0.1',
      port: 0,
      strictPort: true,
      proxy: apiURL ? { '/api': { target: apiURL, changeOrigin: true } } : undefined,
    },
    test: {
      maxWorkers: Number(testWorkers),
      environment: 'jsdom',
      setupFiles: ['./src/test-setup.ts'],
      include: ['src/**/*.test.{ts,tsx}'],
    },
  }
})
