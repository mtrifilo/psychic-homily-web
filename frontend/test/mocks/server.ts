/**
 * MSW Server Setup for Vitest
 *
 * Creates a mock server that intercepts outgoing HTTP requests at the network
 * level. This lets tests exercise the full hook -> apiRequest -> fetch -> response
 * chain without mocking internal modules.
 *
 * Usage in test files:
 *   import { server } from '@/test/mocks/server'
 *   import { http, HttpResponse } from 'msw'
 *
 *   // Override a handler for a specific test:
 *   server.use(
 *     http.get('http://localhost:8080/admin/stats', () => {
 *       return new HttpResponse(null, { status: 403 })
 *     })
 *   )
 *
 * Server lifecycle (start/stop/reset) is managed globally in test/setup.ts,
 * so individual test files don't need beforeAll/afterAll boilerplate.
 */

import { setupServer } from 'msw/node'
import { handlers } from './handlers'

export const server = setupServer(...handlers)
