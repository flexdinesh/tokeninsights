import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../cli/internal/server/static',
    emptyOutDir: true,
  },
  server: { proxy: { '/api': 'http://127.0.0.1:8765' } },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
