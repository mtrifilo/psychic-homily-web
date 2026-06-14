import '@testing-library/jest-dom/vitest'
import { afterAll, afterEach, beforeAll, vi } from 'vitest'
import { cleanup } from '@testing-library/react'
import { server } from './mocks/server'

// Globally mock @sentry/nextjs (PSY-1097). The real package is large and slow to
// transform; when a test pulls it in transitively (e.g. via lib/api.ts) the vite
// module-runner can still be fetching it when the worker pool tears down,
// surfacing as the intermittent "[vitest-worker]: Closing rpc while \"fetch\" was
// pending" CI failure — the module-runner variant of the PSY-945 teardown race
// (that one was an app HTTP fetch; this one is module resolution). Mocking it
// here means the real module is never loaded in any unit test, so that race can't
// fire, and per-file Sentry mocks become unnecessary. App code only calls
// captureException / captureMessage / init / captureRequestError /
// captureRouterTransitionStart / addIntegration; the rest are defensive stubs so
// nothing throws if a consumer reaches for a common Sentry API.
vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
  captureMessage: vi.fn(),
  captureEvent: vi.fn(),
  init: vi.fn(),
  captureRequestError: vi.fn(),
  captureRouterTransitionStart: vi.fn(),
  addIntegration: vi.fn(),
  replayIntegration: vi.fn(() => ({ name: 'Replay' })),
  reactErrorHandler: vi.fn(),
  withScope: vi.fn((cb: (scope: unknown) => unknown) =>
    cb({
      setTag: vi.fn(),
      setExtra: vi.fn(),
      setContext: vi.fn(),
      setLevel: vi.fn(),
    })
  ),
  setUser: vi.fn(),
  setTag: vi.fn(),
  setContext: vi.fn(),
  addBreadcrumb: vi.fn(),
  flush: vi.fn(() => Promise.resolve(true)),
}))

// Start MSW server before all tests, reset handlers after each test,
// and shut down the server when all tests complete.
//
// PSY-945: onUnhandledRequest is 'error' (was 'bypass'). Under 'bypass' a
// request with no MSW handler fell through to the real network. In CI there
// is no backend at localhost:8080, so the fetch stays pending and, if it is
// still in flight when vitest tears down the worker, surfaces as the
// intermittent "[vitest-worker]: Closing rpc while \"fetch\" was pending"
// teardown failure. 'error' fails the offending test loudly at its source
// instead, so a component rendered without stubbing its query-firing children
// can never leak a real request again. vi.mock-based tests are unaffected —
// a mocked module never reaches fetch, so MSW never sees it.
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterAll(() => server.close())

// Cleanup after each test
afterEach(() => {
  server.resetHandlers()
  cleanup()
  vi.clearAllMocks()
})

// Mock window.matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

// Mock IntersectionObserver
class MockIntersectionObserver {
  readonly root: Element | null = null
  readonly rootMargin: string = ''
  readonly thresholds: ReadonlyArray<number> = []

  constructor() {}

  disconnect(): void {}
  observe(): void {}
  unobserve(): void {}
  takeRecords(): IntersectionObserverEntry[] {
    return []
  }
}

Object.defineProperty(window, 'IntersectionObserver', {
  writable: true,
  value: MockIntersectionObserver,
})

// Mock ResizeObserver
class MockResizeObserver {
  constructor() {}

  disconnect(): void {}
  observe(): void {}
  unobserve(): void {}
}

Object.defineProperty(window, 'ResizeObserver', {
  writable: true,
  value: MockResizeObserver,
})

// jsdom doesn't implement these, but Radix popover/listbox primitives
// (Select, etc.) call them when opening. No-op stubs keep popover-based
// component tests working without per-file boilerplate.
Element.prototype.scrollIntoView = vi.fn()
Element.prototype.hasPointerCapture = vi.fn(() => false)
Element.prototype.setPointerCapture = vi.fn()
Element.prototype.releasePointerCapture = vi.fn()
