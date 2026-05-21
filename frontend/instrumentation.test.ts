import { describe, it, expect, vi } from 'vitest'
import * as Sentry from '@sentry/nextjs'

// instrumentation.ts binds onRequestError = Sentry.captureRequestError at
// module-load time, so the mock must be in place before the import below.
vi.mock('@sentry/nextjs', () => ({
  captureRequestError: vi.fn(),
}))

describe('instrumentation.ts', () => {
  it('exports a callable register function', async () => {
    const mod = await import('./instrumentation')

    expect(typeof mod.register).toBe('function')
  })

  it('register resolves without throwing when no runtime is set', async () => {
    const mod = await import('./instrumentation')

    // NEXT_RUNTIME is unset in the test environment, so register should take
    // neither the nodejs nor edge branch and resolve without importing a
    // server/edge Sentry config.
    await expect(mod.register()).resolves.toBeUndefined()
  })

  it('exports onRequestError bound to Sentry.captureRequestError', async () => {
    const mod = await import('./instrumentation')

    expect(mod.onRequestError).toBe(Sentry.captureRequestError)
  })
})
