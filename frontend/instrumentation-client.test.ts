import { describe, it, expect, vi, afterEach } from 'vitest'
import * as Sentry from '@sentry/nextjs'

// Sentry.init + replayIntegration both run at module-load time, so the
// mock must be in place before instrumentation-client is imported below.
vi.mock('@sentry/nextjs', () => ({
  init: vi.fn(),
  replayIntegration: vi.fn((options) => ({ name: 'Replay', options })),
  captureRouterTransitionStart: vi.fn(),
}))

// Re-import fresh per test so the module's top-level Sentry.init re-runs
// against the current (stubbed) NODE_ENV.
async function loadInstrumentationClient() {
  vi.resetModules()
  return import('./instrumentation-client')
}

describe('instrumentation-client.ts', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('initializes Sentry once with the DSN and environment from env vars', async () => {
    vi.stubEnv('NEXT_PUBLIC_SENTRY_DSN', 'https://example@o0.ingest.sentry.io/0')
    vi.stubEnv('NODE_ENV', 'production')

    await loadInstrumentationClient()

    expect(Sentry.init).toHaveBeenCalledTimes(1)
    const config = vi.mocked(Sentry.init).mock.calls[0][0]
    expect(config).toMatchObject({
      dsn: 'https://example@o0.ingest.sentry.io/0',
      environment: 'production',
      debug: false,
    })
  })

  it('masks all text + blocks all media in session replay (privacy posture)', async () => {
    await loadInstrumentationClient()

    expect(Sentry.replayIntegration).toHaveBeenCalledWith({
      maskAllText: true,
      blockAllMedia: true,
    })
  })

  it('disables tracing and replay sampling in development', async () => {
    vi.stubEnv('NODE_ENV', 'development')

    await loadInstrumentationClient()

    const config = vi.mocked(Sentry.init).mock.calls[0][0]
    expect(config).toMatchObject({
      tracesSampleRate: 0,
      replaysSessionSampleRate: 0,
      replaysOnErrorSampleRate: 0,
    })
  })

  it('uses production sample rates in production', async () => {
    vi.stubEnv('NODE_ENV', 'production')

    await loadInstrumentationClient()

    const config = vi.mocked(Sentry.init).mock.calls[0][0]
    expect(config).toMatchObject({
      tracesSampleRate: 0.1,
      replaysSessionSampleRate: 0.1,
      replaysOnErrorSampleRate: 1.0,
    })
  })

  it('exports onRouterTransitionStart bound to Sentry.captureRouterTransitionStart', async () => {
    const mod = await loadInstrumentationClient()

    expect(mod.onRouterTransitionStart).toBe(Sentry.captureRouterTransitionStart)
  })
})
