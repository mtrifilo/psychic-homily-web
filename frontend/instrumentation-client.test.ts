import { describe, it, expect, vi } from 'vitest'
import * as Sentry from '@sentry/nextjs'

// Sentry.init + replayIntegration both run at module-load time, so the
// mock must be in place before instrumentation-client is imported below.
vi.mock('@sentry/nextjs', () => ({
  init: vi.fn(),
  replayIntegration: vi.fn((options) => ({ name: 'Replay', options })),
  captureRouterTransitionStart: vi.fn(),
}))

describe('instrumentation-client.ts', () => {
  // A single test because the module's Sentry.init / replayIntegration calls
  // are one-time module-load side effects; the global afterEach clears mock
  // call records, so a second `it` would see zero recorded calls.
  it('loads and masks all text + blocks all media in session replay (privacy posture)', async () => {
    await import('./instrumentation-client')

    expect(Sentry.init).toHaveBeenCalledTimes(1)
    expect(Sentry.replayIntegration).toHaveBeenCalledWith({
      maskAllText: true,
      blockAllMedia: true,
    })
  })
})
