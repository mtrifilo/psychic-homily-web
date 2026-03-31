import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tsconfigPaths from 'vite-tsconfig-paths'

export default defineConfig({
  plugins: [tsconfigPaths(), react()],
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
      include: ['lib/**/*.ts', 'lib/**/*.tsx', 'components/**/*.tsx'],
      exclude: ['**/*.test.{ts,tsx}', '**/types/**'],
    },
  },
})
