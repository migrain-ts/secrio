/// <reference types="vitest/config" />
import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'
import { assertNoMockInProduction, assertProductionBuild } from './src/api/mock-guard.ts'

export default defineConfig(({ command, mode }) => {
  // A build in a dev/test mode or under a non-production NODE_ENV would ship
  // dev behaviour (DEV/MODE gate the mock): refuse it.
  assertProductionBuild(command, mode, process.env.NODE_ENV)
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
