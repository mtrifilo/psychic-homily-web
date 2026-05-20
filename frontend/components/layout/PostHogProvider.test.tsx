import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { PostHogProvider } from './PostHogProvider'

// --- Mocks ---

const mockInitPostHog = vi.fn()
const mockOptInPostHog = vi.fn()
const mockOptOutPostHog = vi.fn()
const mockHasOptedInCapturing = vi.fn(() => false)
const mockCapture = vi.fn()
const mockIdentify = vi.fn()
const mockReset = vi.fn()

vi.mock('@/lib/posthog', () => ({
  initPostHog: () => mockInitPostHog(),
  optInPostHog: () => mockOptInPostHog(),
  optOutPostHog: () => mockOptOutPostHog(),
  posthog: {
    has_opted_in_capturing: () => mockHasOptedInCapturing(),
    capture: (...args: unknown[]) => mockCapture(...args),
    identify: (...args: unknown[]) => mockIdentify(...args),
    reset: () => mockReset(),
  },
}))

const mockUseCookieConsent = vi.fn(() => ({
  canUseAnalytics: false,
  isLoaded: true,
}))

vi.mock('@/lib/context/CookieConsentContext', () => ({
  useCookieConsent: () => mockUseCookieConsent(),
}))

const mockUseAuthContext = vi.fn(() => ({
  user: null,
  isAuthenticated: false,
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

// Suspense + useSearchParams need a navigation mock.
vi.mock('next/navigation', () => ({
  usePathname: () => '/',
  useSearchParams: () => new URLSearchParams(),
}))

describe('PostHogProvider — consent sync', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockHasOptedInCapturing.mockReturnValue(false)
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    mockUseAuthContext.mockReturnValue({ user: null, isAuthenticated: false })
  })

  it('does nothing until consent context is loaded', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: false })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockOptInPostHog).not.toHaveBeenCalled()
    expect(mockOptOutPostHog).not.toHaveBeenCalled()
  })

  it('opts in when consent is granted but PostHog is not yet opted in', () => {
    mockHasOptedInCapturing.mockReturnValue(false)
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockOptInPostHog).toHaveBeenCalledTimes(1)
    expect(mockOptOutPostHog).not.toHaveBeenCalled()
  })

  it('does NOT re-fire opt-in when PostHog is already opted in (aligned)', () => {
    // Latent-bug regression guard: a fresh mount must not re-trigger opt-in
    // when PostHog's persisted state already matches the granted consent.
    mockHasOptedInCapturing.mockReturnValue(true)
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockOptInPostHog).not.toHaveBeenCalled()
    expect(mockOptOutPostHog).not.toHaveBeenCalled()
  })

  it('opts out when consent is withdrawn but PostHog is still opted in', () => {
    mockHasOptedInCapturing.mockReturnValue(true)
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockOptOutPostHog).toHaveBeenCalledTimes(1)
    expect(mockOptInPostHog).not.toHaveBeenCalled()
  })

  it('does NOT re-fire opt-out when PostHog is already opted out (aligned)', () => {
    mockHasOptedInCapturing.mockReturnValue(false)
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockOptInPostHog).not.toHaveBeenCalled()
    expect(mockOptOutPostHog).not.toHaveBeenCalled()
  })

  it('still initializes PostHog on mount regardless of consent', () => {
    render(<PostHogProvider>child</PostHogProvider>)
    expect(mockInitPostHog).toHaveBeenCalledTimes(1)
  })
})
