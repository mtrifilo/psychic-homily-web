import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { PostHogProvider } from './PostHogProvider'

// --- Mocks ---
// Lazy posthog API (PSY-1091). The provider now calls enable/disable per
// consent; posthog-js itself only loads inside enableAnalytics, and
// idempotency lives in the lib (tested in lib/posthog.test.ts), not here.
const mockEnable = vi.fn()
const mockDisable = vi.fn()
const mockCapturePageview = vi.fn()

vi.mock('@/lib/posthog', () => ({
  enableAnalytics: () => mockEnable(),
  disableAnalytics: () => mockDisable(),
  capturePageview: (...args: unknown[]) => mockCapturePageview(...args),
}))

const mockUseCookieConsent = vi.fn(() => ({
  canUseAnalytics: false,
  isLoaded: true,
}))

vi.mock('@/lib/context/CookieConsentContext', () => ({
  useCookieConsent: () => mockUseCookieConsent(),
}))

// NOTE: AuthContext is deliberately NOT mocked. PostHogProvider renders above
// the auth hydration boundary, so it must mount with no AuthProvider in
// context — every test here would throw "useAuthContext must be used within an
// AuthProvider" if the identify effect were moved back into the provider.
// Identification lives in PostHogIdentify (see PostHogIdentify.test.tsx).

// Suspense + useSearchParams need a navigation mock.
vi.mock('next/navigation', () => ({
  usePathname: () => '/',
  useSearchParams: () => new URLSearchParams(),
}))

describe('PostHogProvider — consent sync', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
  })

  it('does nothing until consent context is loaded', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: false })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockEnable).not.toHaveBeenCalled()
    expect(mockDisable).not.toHaveBeenCalled()
  })

  it('enables analytics (lazy-loads posthog) when consent is granted', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockEnable).toHaveBeenCalledTimes(1)
    expect(mockDisable).not.toHaveBeenCalled()
  })

  it('does NOT load posthog when consent is withheld', () => {
    // The load-bearing TTI win: no consent → posthog-js never fetched.
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockEnable).not.toHaveBeenCalled()
    expect(mockDisable).toHaveBeenCalledTimes(1)
  })
})

// Consent transitions within a single mount: granted → withdrawn and back.
describe('PostHogProvider — consent transition within a single mount', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
  })

  it('granted → withdrawn (re-render) disables analytics', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    const { rerender } = render(<PostHogProvider>child</PostHogProvider>)
    expect(mockEnable).toHaveBeenCalledTimes(1)

    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    rerender(<PostHogProvider>child</PostHogProvider>)

    expect(mockDisable).toHaveBeenCalledTimes(1)
  })

  it('withdrawn → granted (re-render) enables analytics', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    const { rerender } = render(<PostHogProvider>child</PostHogProvider>)
    expect(mockEnable).not.toHaveBeenCalled()

    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    rerender(<PostHogProvider>child</PostHogProvider>)

    expect(mockEnable).toHaveBeenCalledTimes(1)
  })
})

// Children render through unchanged — provider must never block its subtree.
describe('PostHogProvider — children render', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
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

// Pageview capture: PostHogPageView calls capturePageview ONLY when consent is
// granted (capturePageview itself is a no-op until posthog has lazy-loaded).
describe('PostHogProvider — pageview capture', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('captures a pageview when consent granted', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true, isLoaded: true })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockCapturePageview).toHaveBeenCalled()
  })

  it('does NOT capture a pageview when consent withheld', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false, isLoaded: true })
    render(<PostHogProvider>child</PostHogProvider>)

    expect(mockCapturePageview).not.toHaveBeenCalled()
  })
})
