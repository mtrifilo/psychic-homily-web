import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import * as Sentry from '@sentry/nextjs'

// The replay integration is now attached LAZILY (PSY-1091) — Sentry.init runs at
// module-load, but replayIntegration is fetched via lazyLoadIntegration after
// interactivity (production only), so the mocks cover that path too.
const replayIntegrationFn = vi.fn((options) => ({ name: 'Replay', options }))

vi.mock('@sentry/nextjs', () => ({
  init: vi.fn(),
  lazyLoadIntegration: vi.fn(() => Promise.resolve(replayIntegrationFn)),
  addIntegration: vi.fn(),
  captureRouterTransitionStart: vi.fn(),
}))

// Re-import fresh per test so the module's top-level Sentry.init re-runs
// against the current (stubbed) NODE_ENV.
async function loadInstrumentationClient() {
  vi.resetModules()
  return import('./instrumentation-client')
}

describe('instrumentation-client.ts', () => {
  beforeEach(() => {
    // Run the idle-deferred replay attach synchronously so it's observable.
    vi.stubGlobal('requestIdleCallback', (cb: () => void) => {
      cb()
      return 0
    })
  })

  afterEach(() => {
    vi.unstubAllEnvs()
    vi.unstubAllGlobals()
    vi.clearAllMocks()
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

  it('does NOT eagerly include replay in the init integrations (lazy-attached)', async () => {
    vi.stubEnv('NODE_ENV', 'production')

    await loadInstrumentationClient()

    // Replay is attached via addIntegration after idle, never via the eager
    // init `integrations` array — that's what keeps it out of the initial bundle.
    const config = vi.mocked(Sentry.init).mock.calls[0][0]
    expect(config.integrations).toBeUndefined()
  })

  it('lazily attaches masked session replay in production after idle', async () => {
    vi.stubEnv('NODE_ENV', 'production')

    await loadInstrumentationClient()
    await vi.waitFor(() => expect(Sentry.addIntegration).toHaveBeenCalled())

    expect(Sentry.lazyLoadIntegration).toHaveBeenCalledWith('replayIntegration')
    // Privacy posture preserved on the lazily-loaded integration.
    expect(replayIntegrationFn).toHaveBeenCalledWith({
      maskAllText: true,
      blockAllMedia: true,
    })
  })

  it('does NOT load session replay in development', async () => {
    vi.stubEnv('NODE_ENV', 'development')

    await loadInstrumentationClient()
    await Promise.resolve()

    expect(Sentry.lazyLoadIntegration).not.toHaveBeenCalled()
    expect(Sentry.addIntegration).not.toHaveBeenCalled()
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
