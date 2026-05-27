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

// Widen the user type so individual tests can swap in a real user
// without TS narrowing from the default-null literal (same pattern as
// Sidebar / TopBar test files).
type MockAuthContextValue = {
  user: { id: string; email: string; is_admin?: boolean } | null
  isAuthenticated: boolean
}
const mockUseAuthContext = vi.fn<() => MockAuthContextValue>(() => ({
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

// PSY-728 reference: the prev-consent comparison must read PostHog's own
// persisted state (`posthog.has_opted_in_capturing()`) — NOT a per-mount
// ref that would flip every page load and re-fire opt-in / session
// recording for users already opted in. The tests above pin the post-fix
// behavior. This block adds explicit transition tests (granted → withdrawn,
// withdrawn → granted) within a single mounted instance, which the
// original ref-based code mishandled.
describe('PostHogProvider — consent transition within a single mount', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockHasOptedInCapturing.mockReturnValue(false)
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    mockUseAuthContext.mockReturnValue({ user: null, isAuthenticated: false })
  })

  it('granted → withdrawn (re-render) triggers exactly one opt-out call', () => {
    // Start opted in (mocked PostHog state agrees with consent).
    mockHasOptedInCapturing.mockReturnValue(true)
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    const { rerender } = render(<PostHogProvider>child</PostHogProvider>)

    expect(mockOptOutPostHog).not.toHaveBeenCalled()

    // User withdraws consent. After the user clicked "Reject all",
    // PostHog's persisted state is still opted in until we opt out.
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    rerender(<PostHogProvider>child</PostHogProvider>)

    expect(mockOptOutPostHog).toHaveBeenCalledTimes(1)
    expect(mockOptInPostHog).not.toHaveBeenCalled()
  })

  it('withdrawn → granted (re-render) triggers exactly one opt-in call', () => {
    mockHasOptedInCapturing.mockReturnValue(false)
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    const { rerender } = render(<PostHogProvider>child</PostHogProvider>)

    expect(mockOptInPostHog).not.toHaveBeenCalled()

    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    rerender(<PostHogProvider>child</PostHogProvider>)

    expect(mockOptInPostHog).toHaveBeenCalledTimes(1)
    expect(mockOptOutPostHog).not.toHaveBeenCalled()
  })
})

// User identification side-effect: PostHog should identify on auth, reset
// on logout, and do nothing while consent is withheld. Distinct from
// opt-in/out wiring since identify() and reset() are independent calls.
describe('PostHogProvider — user identification', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockHasOptedInCapturing.mockReturnValue(false)
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    mockUseAuthContext.mockReturnValue({ user: null, isAuthenticated: false })
  })

  it('identifies the user when authenticated AND consent granted', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    mockUseAuthContext.mockReturnValue({
      user: { id: 'u-123', email: 'fan@test.com', is_admin: false },
      isAuthenticated: true,
    })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockIdentify).toHaveBeenCalledWith('u-123', {
      email: 'fan@test.com',
      is_admin: false,
    })
    expect(mockReset).not.toHaveBeenCalled()
  })

  it('resets PostHog when consent granted but user is logged out', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    mockUseAuthContext.mockReturnValue({ user: null, isAuthenticated: false })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockReset).toHaveBeenCalledTimes(1)
    expect(mockIdentify).not.toHaveBeenCalled()
  })

  it('does NOT identify or reset when consent is withheld', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    mockUseAuthContext.mockReturnValue({
      user: { id: 'u-123', email: 'fan@test.com', is_admin: false },
      isAuthenticated: true,
    })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockIdentify).not.toHaveBeenCalled()
    expect(mockReset).not.toHaveBeenCalled()
  })
})

// Children render through unchanged — provider must never block its
// subtree on consent state, network state, or anything else.
describe('PostHogProvider — children render', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockHasOptedInCapturing.mockReturnValue(false)
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    mockUseAuthContext.mockReturnValue({ user: null, isAuthenticated: false })
  })

  it('renders children when consent loaded + analytics off', () => {
    const { getByText } = render(
      <PostHogProvider>
        <div>child-content</div>
      </PostHogProvider>
    )
    expect(getByText('child-content')).toBeInTheDocument()
  })

  it('renders children when consent loaded + analytics on', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    const { getByText } = render(
      <PostHogProvider>
        <div>child-content</div>
      </PostHogProvider>
    )
    expect(getByText('child-content')).toBeInTheDocument()
  })

  it('renders children while consent context is still loading', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: false })
    const { getByText } = render(
      <PostHogProvider>
        <div>child-content</div>
      </PostHogProvider>
    )
    expect(getByText('child-content')).toBeInTheDocument()
  })
})

// Pageview capture: the inner PostHogPageView component should call
// posthog.capture('$pageview', …) ONLY when canUseAnalytics is true.
describe('PostHogProvider — pageview capture', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockHasOptedInCapturing.mockReturnValue(false)
    mockUseAuthContext.mockReturnValue({ user: null, isAuthenticated: false })
  })

  it('captures $pageview when consent granted', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockCapture).toHaveBeenCalled()
    const pageviewCall = mockCapture.mock.calls.find(c => c[0] === '$pageview')
    expect(pageviewCall).toBeDefined()
  })

  it('does NOT capture $pageview when consent withheld', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    render(<PostHogProvider>child</PostHogProvider>)

    const pageviewCall = mockCapture.mock.calls.find(c => c[0] === '$pageview')
    expect(pageviewCall).toBeUndefined()
  })
})
