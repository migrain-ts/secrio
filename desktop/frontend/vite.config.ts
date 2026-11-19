/// <reference types="vitest/config" />
import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'
import { assertNoMockInProduction } from './src/api/mock-guard.ts'

export default defineConfig(({ mode }) => {
  // .env files and process.env (VITE_ prefix): refuse a mock production build.
  assertNoMockInProduction(mode, loadEnv(mode, process.cwd(), 'VITE_'))
  return {
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
    },
    test: {
      environment: 'jsdom',
      setupFiles: ['./src/test/setup.ts'],
    },
  }
})
