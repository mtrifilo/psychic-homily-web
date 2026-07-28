import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { PostHogIdentify } from './PostHogIdentify'

const mockIdentifyUser = vi.fn()
const mockResetAnalytics = vi.fn()

vi.mock('@/lib/posthog', () => ({
  identifyUser: (...args: unknown[]) => mockIdentifyUser(...args),
  resetAnalytics: () => mockResetAnalytics(),
}))

const mockUseCookieConsent = vi.fn(() => ({ canUseAnalytics: false }))

vi.mock('@/lib/context/CookieConsentContext', () => ({
  useCookieConsent: () => mockUseCookieConsent(),
}))

// Widen the user type so individual tests can swap in a real user without TS
// narrowing from the default-null literal (same pattern as Sidebar / TopBar).
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

describe('PostHogIdentify', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false })
    mockUseAuthContext.mockReturnValue({ user: null, isAuthenticated: false })
  })

  it('identifies the user when authenticated AND consent granted', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true })
    mockUseAuthContext.mockReturnValue({
      user: { id: 'u-123', email: 'fan@test.com', is_admin: false },
      isAuthenticated: true,
    })
    render(<PostHogIdentify />)

    expect(mockIdentifyUser).toHaveBeenCalledWith('u-123', {
      email: 'fan@test.com',
      is_admin: false,
    })
    expect(mockResetAnalytics).not.toHaveBeenCalled()
  })

  it('resets analytics when consent granted but user is logged out', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: true })
    mockUseAuthContext.mockReturnValue({ user: null, isAuthenticated: false })
    render(<PostHogIdentify />)

    expect(mockResetAnalytics).toHaveBeenCalledTimes(1)
    expect(mockIdentifyUser).not.toHaveBeenCalled()
  })

  it('does NOT identify or reset when consent is withheld', () => {
    mockUseCookieConsent.mockReturnValue({ canUseAnalytics: false })
    mockUseAuthContext.mockReturnValue({
      user: { id: 'u-123', email: 'fan@test.com', is_admin: false },
      isAuthenticated: true,
    })
    render(<PostHogIdentify />)

    expect(mockIdentifyUser).not.toHaveBeenCalled()
    expect(mockResetAnalytics).not.toHaveBeenCalled()
  })

  it('renders nothing', () => {
    const { container } = render(<PostHogIdentify />)
    expect(container).toBeEmptyDOMElement()
  })
})
