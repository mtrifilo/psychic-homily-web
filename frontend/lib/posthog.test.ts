import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// Mock the posthog-js module
const mockInit = vi.fn()
const mockOptInCapturing = vi.fn()
const mockOptOutCapturing = vi.fn()
const mockStartSessionRecording = vi.fn()
const mockStopSessionRecording = vi.fn()
const mockReset = vi.fn()

vi.mock('posthog-js', () => ({
  default: {
    init: (...args: unknown[]) => mockInit(...args),
    opt_in_capturing: () => mockOptInCapturing(),
    opt_out_capturing: () => mockOptOutCapturing(),
    startSessionRecording: () => mockStartSessionRecording(),
    stopSessionRecording: () => mockStopSessionRecording(),
    reset: () => mockReset(),
  },
}))

// We need to reimport the module fresh for each test group
// because isInitialized is module-level state
describe('posthog', () => {
  let originalEnv: NodeJS.ProcessEnv

  beforeEach(() => {
    vi.clearAllMocks()
    originalEnv = { ...process.env }
  })

  afterEach(() => {
    process.env = originalEnv
    // Reset module state between tests by re-importing
    vi.resetModules()
  })

  describe('initPostHog', () => {
    it('does not initialize when window is undefined (SSR)', async () => {
      const windowSpy = vi.spyOn(globalThis, 'window', 'get')
      // Simulating SSR where window is undefined.
      windowSpy.mockReturnValue(undefined)

      const { initPostHog } = await import('./posthog')
      initPostHog()

      expect(mockInit).not.toHaveBeenCalled()
      windowSpy.mockRestore()
    })

    it('does not initialize when NEXT_PUBLIC_POSTHOG_KEY is not set', async () => {
      delete process.env.NEXT_PUBLIC_POSTHOG_KEY

      const { initPostHog } = await import('./posthog')
      initPostHog()

      expect(mockInit).not.toHaveBeenCalled()
    })

    it('initializes posthog with correct config when key is set', async () => {
      process.env.NEXT_PUBLIC_POSTHOG_KEY = 'phc_test_key_123'
      process.env.NEXT_PUBLIC_POSTHOG_HOST = 'https://custom.posthog.com'

      const { initPostHog } = await import('./posthog')
      initPostHog()

      expect(mockInit).toHaveBeenCalledWith('phc_test_key_123', {
        api_host: 'https://custom.posthog.com',
        capture_pageview: false,
        capture_pageleave: true,
        opt_out_capturing_by_default: true,
        persistence: 'localStorage',
        session_recording: { maskAllInputs: true },
      })
    })

    it('uses default posthog host when NEXT_PUBLIC_POSTHOG_HOST is not set', async () => {
      process.env.NEXT_PUBLIC_POSTHOG_KEY = 'phc_test_key_123'
      delete process.env.NEXT_PUBLIC_POSTHOG_HOST

      const { initPostHog } = await import('./posthog')
      initPostHog()

      expect(mockInit).toHaveBeenCalledWith(
        'phc_test_key_123',
        expect.objectContaining({
          api_host: 'https://app.posthog.com',
        })
      )
    })

    it('does not initialize twice', async () => {
      process.env.NEXT_PUBLIC_POSTHOG_KEY = 'phc_test_key_123'

      const { initPostHog } = await import('./posthog')
      initPostHog()
      initPostHog()

      expect(mockInit).toHaveBeenCalledTimes(1)
    })
  })

  describe('optInPostHog', () => {
    it('calls opt_in_capturing and startSessionRecording', async () => {
      process.env.NEXT_PUBLIC_POSTHOG_KEY = 'phc_test_key_123'

      const { optInPostHog } = await import('./posthog')
      optInPostHog()

      expect(mockOptInCapturing).toHaveBeenCalledTimes(1)
      expect(mockStartSessionRecording).toHaveBeenCalledTimes(1)
    })

    it('initializes posthog if not already initialized', async () => {
      process.env.NEXT_PUBLIC_POSTHOG_KEY = 'phc_test_key_123'

      const { optInPostHog } = await import('./posthog')
      optInPostHog()

      // Should have called init since it was not initialized
      expect(mockInit).toHaveBeenCalled()
      expect(mockOptInCapturing).toHaveBeenCalled()
    })
  })

  describe('optOutPostHog', () => {
    it('calls opt_out_capturing, stopSessionRecording, and reset', async () => {
      const { optOutPostHog } = await import('./posthog')
      optOutPostHog()

      expect(mockOptOutCapturing).toHaveBeenCalledTimes(1)
      expect(mockStopSessionRecording).toHaveBeenCalledTimes(1)
      expect(mockReset).toHaveBeenCalledTimes(1)
    })
  })

  describe('posthog export', () => {
    it('re-exports the posthog instance', async () => {
      const { posthog } = await import('./posthog')
      expect(posthog).toBeDefined()
      expect(typeof posthog.init).toBe('function')
    })
  })
})
