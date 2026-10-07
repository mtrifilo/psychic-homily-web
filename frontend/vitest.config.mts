import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tsconfigPaths from 'vite-tsconfig-paths'

export default defineConfig({
  plugins: [tsconfigPaths(), react()],
  resolve: {
    alias: [
      // App code resolves `next/dynamic` to the App Router loader in both
      // bundlers; the package entry is the Pages Router loader. The App Router
      // loader calls `loading` only while the module is pending and throws a
      // failed import to the nearest error boundary, so tests run on the
      // loader the builds use.
      {
        find: /^next\/dynamic$/,
        replacement: 'next/dist/shared/lib/app-dynamic',
      },
    ],
  },
  test: {
    environment: 'jsdom',
    env: {
      // Ensure API_BASE_URL resolves to a predictable value in tests.
      // MSW handlers in test/mocks/handlers.ts use this same base URL
      // to intercept requests at the network level.
      NEXT_PUBLIC_API_URL: 'http://localhost:8080',
    },
    setupFiles: ['./test/setup.ts'],
    include: ['**/*.test.{ts,tsx}'],
    exclude: ['**/node_modules/**', '**/.next/**'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov'],
      include: ['{lib,components,features,app}/**/*.{ts,tsx}'],
      exclude: ['**/*.test.{ts,tsx}', '**/types/**'],
    },
  },
})
