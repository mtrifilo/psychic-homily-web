import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// Mock posthog-js (resolved via the lazy dynamic import in the lib).
// has_opted_in_capturing is stateful so the opt-in idempotency / PSY-728
// re-opt-in guard can be exercised.
let optedInState = false
const mockInit = vi.fn()
const mockOptInCapturing = vi.fn(() => {
  optedInState = true
})
const mockOptOutCapturing = vi.fn(() => {
  optedInState = false
})
const mockHasOptedIn = vi.fn(() => optedInState)
const mockStartSessionRecording = vi.fn()
const mockStopSessionRecording = vi.fn()
const mockReset = vi.fn()
const mockCapture = vi.fn()
const mockIdentify = vi.fn()

vi.mock('posthog-js', () => ({
  default: {
    init: (...args: unknown[]) => mockInit(...args),
    opt_in_capturing: () => mockOptInCapturing(),
    opt_out_capturing: () => mockOptOutCapturing(),
    has_opted_in_capturing: () => mockHasOptedIn(),
    startSessionRecording: () => mockStartSessionRecording(),
    stopSessionRecording: () => mockStopSessionRecording(),
    reset: () => mockReset(),
    capture: (...args: unknown[]) => mockCapture(...args),
    identify: (...args: unknown[]) => mockIdentify(...args),
  },
}))

describe('posthog (lazy, consent-gated)', () => {
  let originalEnv: NodeJS.ProcessEnv

  beforeEach(() => {
    vi.clearAllMocks()
    optedInState = false
    originalEnv = { ...process.env }
    process.env.NEXT_PUBLIC_POSTHOG_KEY = 'phc_test_key_123'
  })

  afterEach(() => {
    process.env = originalEnv
    vi.resetModules()
  })

  describe('enableAnalytics', () => {
    it('does not load/init on the server (no window)', async () => {
      const windowSpy = vi.spyOn(globalThis, 'window', 'get')
      windowSpy.mockReturnValue(undefined as unknown as Window & typeof globalThis)

      const { enableAnalytics } = await import('./posthog')
      await enableAnalytics()

      expect(mockInit).not.toHaveBeenCalled()
      expect(mockOptInCapturing).not.toHaveBeenCalled()
      windowSpy.mockRestore()
    })

    it('does not load/init when NEXT_PUBLIC_POSTHOG_KEY is not set', async () => {
      delete process.env.NEXT_PUBLIC_POSTHOG_KEY

      const { enableAnalytics } = await import('./posthog')
      await enableAnalytics()

      expect(mockInit).not.toHaveBeenCalled()
    })

    it('lazy-inits with the correct config, opts in, records, and captures the landing pageview', async () => {
      process.env.NEXT_PUBLIC_POSTHOG_HOST = 'https://custom.posthog.com'

      const { enableAnalytics } = await import('./posthog')
      await enableAnalytics()

      expect(mockInit).toHaveBeenCalledWith('phc_test_key_123', {
        api_host: 'https://custom.posthog.com',
        capture_pageview: false,
        capture_pageleave: true,
        opt_out_capturing_by_default: true,
        persistence: 'localStorage',
        session_recording: { maskAllInputs: true },
      })
      expect(mockOptInCapturing).toHaveBeenCalledTimes(1)
      expect(mockStartSessionRecording).toHaveBeenCalledTimes(1)
      // Landing pageview recovered (the effect fired before posthog loaded).
      const pageview = mockCapture.mock.calls.find(c => c[0] === '$pageview')
      expect(pageview).toBeDefined()
    })

    it('uses the default host when NEXT_PUBLIC_POSTHOG_HOST is not set', async () => {
      delete process.env.NEXT_PUBLIC_POSTHOG_HOST

      const { enableAnalytics } = await import('./posthog')
      await enableAnalytics()

      expect(mockInit).toHaveBeenCalledWith(
        'phc_test_key_123',
        expect.objectContaining({ api_host: 'https://app.posthog.com' })
      )
    })

    it('is idempotent — init + opt-in run once across repeated calls', async () => {
      const { enableAnalytics } = await import('./posthog')
      await enableAnalytics()
      await enableAnalytics()

      expect(mockInit).toHaveBeenCalledTimes(1)
      expect(mockOptInCapturing).toHaveBeenCalledTimes(1)
    })

    it('does NOT re-opt-in when posthog is already opted in (PSY-728 parity across page loads)', async () => {
      optedInState = true // simulate a returning visitor persisted as opted-in

      const { enableAnalytics } = await import('./posthog')
      await enableAnalytics()

      expect(mockOptInCapturing).not.toHaveBeenCalled()
      expect(mockStartSessionRecording).not.toHaveBeenCalled()
    })

    it('aborts opt-in when consent is withdrawn while posthog is still loading (privacy race)', async () => {
      const { enableAnalytics, disableAnalytics } = await import('./posthog')

      const pending = enableAnalytics() // import in flight, desired=true
      disableAnalytics() // consent withdrawn before the import resolves → desired=false
      await pending

      expect(mockOptInCapturing).not.toHaveBeenCalled()
      expect(mockStartSessionRecording).not.toHaveBeenCalled()
    })
  })

  describe('disableAnalytics', () => {
    it('is a no-op when analytics was never enabled (posthog not loaded)', async () => {
      const { disableAnalytics } = await import('./posthog')
      disableAnalytics()

      expect(mockOptOutCapturing).not.toHaveBeenCalled()
      expect(mockStopSessionRecording).not.toHaveBeenCalled()
      expect(mockReset).not.toHaveBeenCalled()
    })

    it('opts out, stops recording, and resets after analytics was enabled', async () => {
      const { enableAnalytics, disableAnalytics } = await import('./posthog')
      await enableAnalytics()
      disableAnalytics()

      expect(mockOptOutCapturing).toHaveBeenCalledTimes(1)
      expect(mockStopSessionRecording).toHaveBeenCalledTimes(1)
      expect(mockReset).toHaveBeenCalledTimes(1)
    })
  })

  describe('capturePageview / identifyUser / resetAnalytics', () => {
    it('are no-ops until posthog has loaded (pre-consent)', async () => {
      const { capturePageview, identifyUser, resetAnalytics } = await import(
        './posthog'
      )
      capturePageview('https://example.com/explore')
      identifyUser('u-1', { email: 'a@b.com', is_admin: false })
      resetAnalytics()

      expect(mockCapture).not.toHaveBeenCalled()
      expect(mockIdentify).not.toHaveBeenCalled()
      expect(mockReset).not.toHaveBeenCalled()
    })

    it('capture + identify work after analytics is enabled', async () => {
      const { enableAnalytics, capturePageview, identifyUser } = await import(
        './posthog'
      )
      await enableAnalytics()
      capturePageview('https://example.com/explore')
      identifyUser('u-1', { email: 'a@b.com', is_admin: true })

      expect(mockCapture).toHaveBeenCalledWith('$pageview', {
        $current_url: 'https://example.com/explore',
      })
      expect(mockIdentify).toHaveBeenCalledWith('u-1', {
        email: 'a@b.com',
        is_admin: true,
      })
    })
  })
})
