import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import * as Sentry from '@sentry/nextjs'

// Replay is attached LAZILY (PSY-1091): Sentry.init runs at module-load, but the
// replay integration lives in ./instrumentation-replay, dynamic-import()ed after
// interactivity (production only). Mock that module so the lazy path is testable.
// Hoisted stable spy so it survives vi.resetModules() — the dynamic import in
// instrumentation-client and the assertions below share one instance.
const { attachReplay } = vi.hoisted(() => ({ attachReplay: vi.fn() }))
vi.mock('./instrumentation-replay', () => ({ attachReplay }))

// Re-import fresh per test so the module's top-level Sentry.init re-runs against
// the current (stubbed) NODE_ENV.
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

    // Replay is attached via a dynamic import after idle, never via the eager
    // init `integrations` array — that's what keeps it out of the initial bundle.
    const config = vi.mocked(Sentry.init).mock.calls[0][0]
    expect(config.integrations).toBeUndefined()
  })

  it('lazily attaches session replay in production after idle', async () => {
    vi.stubEnv('NODE_ENV', 'production')

    await loadInstrumentationClient()
    await vi.waitFor(() => expect(attachReplay).toHaveBeenCalled())
  })

  it('does NOT load session replay in development', async () => {
    vi.stubEnv('NODE_ENV', 'development')

    await loadInstrumentationClient()
    await Promise.resolve()

    expect(attachReplay).not.toHaveBeenCalled()
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
