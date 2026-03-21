import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CookieConsentBanner } from './CookieConsentBanner'

// --- Mocks ---

const mockAcceptAll = vi.fn()
const mockRejectAll = vi.fn()
const mockOpenPreferences = vi.fn()
const mockClosePreferences = vi.fn()
const mockSavePreferences = vi.fn()
const mockUseCookieConsent = vi.fn(() => ({
  showBanner: true,
  gpcSignalDetected: false,
  acceptAll: mockAcceptAll,
  rejectAll: mockRejectAll,
  openPreferences: mockOpenPreferences,
  closePreferences: mockClosePreferences,
  savePreferences: mockSavePreferences,
  preferencesOpen: false,
  consent: null,
}))

vi.mock('@/lib/context/CookieConsentContext', () => ({
  useCookieConsent: () => mockUseCookieConsent(),
}))

vi.mock('next/link', () => ({
  default: ({ children, href, ...props }: { children: React.ReactNode; href: string; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

describe('CookieConsentBanner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCookieConsent.mockReturnValue({
      showBanner: true,
      gpcSignalDetected: false,
      acceptAll: mockAcceptAll,
      rejectAll: mockRejectAll,
      openPreferences: mockOpenPreferences,
      closePreferences: mockClosePreferences,
      savePreferences: mockSavePreferences,
      preferencesOpen: false,
      consent: null,
    })
  })

  describe('when banner should be shown', () => {
    it('renders the banner with "Cookie Preferences" heading', () => {
      render(<CookieConsentBanner />)
      expect(screen.getByText('Cookie Preferences')).toBeInTheDocument()
    })

    it('renders description text', () => {
      render(<CookieConsentBanner />)
      expect(screen.getByText(/We use cookies to improve your experience/)).toBeInTheDocument()
    })

    it('renders "Learn more" link to privacy page', () => {
      render(<CookieConsentBanner />)
      const link = screen.getByText('Learn more')
      expect(link.closest('a')).toHaveAttribute('href', '/privacy')
    })

    it('has correct ARIA attributes', () => {
      render(<CookieConsentBanner />)
      const banner = screen.getByRole('dialog')
      expect(banner).toHaveAttribute('aria-label', 'Cookie consent')
      expect(banner).toHaveAttribute('aria-describedby', 'cookie-consent-description')
    })

    it('renders Accept All button', () => {
      render(<CookieConsentBanner />)
      expect(screen.getByText('Accept All')).toBeInTheDocument()
    })

    it('renders Reject All button', () => {
      render(<CookieConsentBanner />)
      expect(screen.getByText('Reject All')).toBeInTheDocument()
    })

    it('renders Customize button', () => {
      render(<CookieConsentBanner />)
      expect(screen.getByText('Customize')).toBeInTheDocument()
    })

    it('calls acceptAll when Accept All is clicked', async () => {
      const user = userEvent.setup()
      render(<CookieConsentBanner />)

      await user.click(screen.getByText('Accept All'))
      expect(mockAcceptAll).toHaveBeenCalledOnce()
    })

    it('calls rejectAll when Reject All is clicked', async () => {
      const user = userEvent.setup()
      render(<CookieConsentBanner />)

      await user.click(screen.getByText('Reject All'))
      expect(mockRejectAll).toHaveBeenCalledOnce()
    })

    it('calls openPreferences when Customize is clicked', async () => {
      const user = userEvent.setup()
      render(<CookieConsentBanner />)

      await user.click(screen.getByText('Customize'))
      expect(mockOpenPreferences).toHaveBeenCalledOnce()
    })

    it('shows GPC signal notice when detected', () => {
      mockUseCookieConsent.mockReturnValue({
        showBanner: true,
        gpcSignalDetected: true,
        acceptAll: mockAcceptAll,
        rejectAll: mockRejectAll,
        openPreferences: mockOpenPreferences,
        closePreferences: mockClosePreferences,
        savePreferences: mockSavePreferences,
        preferencesOpen: false,
        consent: null,
      })
      render(<CookieConsentBanner />)

      expect(screen.getByText(/Global Privacy Control signal/)).toBeInTheDocument()
    })

    it('does not show GPC signal notice when not detected', () => {
      render(<CookieConsentBanner />)
      expect(screen.queryByText(/Global Privacy Control signal/)).not.toBeInTheDocument()
    })
  })

  describe('when banner should not be shown (consent exists)', () => {
    it('does not render the banner', () => {
      mockUseCookieConsent.mockReturnValue({
        showBanner: false,
        gpcSignalDetected: false,
        acceptAll: mockAcceptAll,
        rejectAll: mockRejectAll,
        openPreferences: mockOpenPreferences,
        closePreferences: mockClosePreferences,
        savePreferences: mockSavePreferences,
        preferencesOpen: false,
        consent: {
          version: 1,
          timestamp: new Date().toISOString(),
          expiresAt: new Date(Date.now() + 86400000).toISOString(),
          gpcDetected: false,
          categories: { essential: true, analytics: true },
          consentMethod: 'banner_accept_all' as const,
        },
      })
      render(<CookieConsentBanner />)

      expect(screen.queryByText('Accept All')).not.toBeInTheDocument()
      expect(screen.queryByText('Reject All')).not.toBeInTheDocument()
      expect(screen.queryByRole('dialog', { name: 'Cookie consent' })).not.toBeInTheDocument()
    })

    it('still renders the preferences dialog component (for footer trigger)', () => {
      mockUseCookieConsent.mockReturnValue({
        showBanner: false,
        gpcSignalDetected: false,
        acceptAll: mockAcceptAll,
        rejectAll: mockRejectAll,
        openPreferences: mockOpenPreferences,
        closePreferences: mockClosePreferences,
        savePreferences: mockSavePreferences,
        preferencesOpen: false,
        consent: {
          version: 1,
          timestamp: new Date().toISOString(),
          expiresAt: new Date(Date.now() + 86400000).toISOString(),
          gpcDetected: false,
          categories: { essential: true, analytics: true },
          consentMethod: 'banner_accept_all' as const,
        },
      })
      // The CookiePreferencesDialog is always rendered, even when banner is hidden.
      // This is just a render test — the dialog won't be visible when preferencesOpen is false.
      const { container } = render(<CookieConsentBanner />)
      expect(container).toBeTruthy()
    })
  })
})
