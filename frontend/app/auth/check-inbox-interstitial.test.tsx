import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { apiRequest } from '@/lib/api'
import { CheckInboxInterstitial } from './_components/check-inbox-interstitial'

// --- Mocks ---
//
// The REAL resend control and useSendVerificationEmail hook run here, against a
// mocked apiRequest. Mocking the hook would freeze its state at render time, so
// pending/success/error would be presets rather than consequences of the click,
// and these tests would still pass with the click handler deleted.

vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: vi.fn(),
}))

const mockApiRequest = vi.mocked(apiRequest)

const resendButton = () => screen.getByRole('button', { name: 'Resend email' })

function rateLimitError(retryAfter?: number): Error {
  return Object.assign(new Error('Rate limit exceeded.'), {
    status: 429,
    retryAfter,
  })
}

describe('CheckInboxInterstitial', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('names the address the link was sent to and how long it lasts', () => {
    renderWithProviders(
      <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
    )

    expect(
      screen.getByRole('heading', { name: 'Check your inbox.' })
    ).toBeInTheDocument()
    expect(screen.getByText('listener@example.com')).toBeInTheDocument()
    expect(screen.getByText(/expires in 24 hours/)).toBeInTheDocument()
  })

  // This surface replaces the signup card in place, so it gets none of the
  // App Router's route announcement and the focused submit button unmounts
  // underneath the user. Without moving focus, submitting signup is silent.
  it('takes focus on its heading so the swap is announced', () => {
    renderWithProviders(
      <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
    )

    expect(
      screen.getByRole('heading', { name: 'Check your inbox.' })
    ).toHaveFocus()
  })

  it('states what is open before verifying without promising alerts', () => {
    renderWithProviders(
      <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
    )

    expect(
      screen.getByText(/Browse shows, save what you like, follow artists/)
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Verifying unlocks show submission/)
    ).toBeInTheDocument()
  })

  // returnTo decision (PSY-1900 / PSY-1878): the interstitial always shows, but
  // a signup that started mid-task hands the user back to the task.
  describe('primary CTA', () => {
    it('offers the shows listing when signup did not start from a task', () => {
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      expect(
        screen.getByRole('link', { name: 'Browse upcoming shows' })
      ).toHaveAttribute('href', '/shows')
      expect(
        screen.queryByRole('link', { name: 'Continue where you left off' })
      ).not.toBeInTheDocument()
    })

    it('returns the user to the task they came from', () => {
      renderWithProviders(
        <CheckInboxInterstitial
          email="listener@example.com"
          returnTo="/shows/tigers-jaw-at-the-rebel-lounge"
        />
      )

      expect(
        screen.getByRole('link', { name: 'Continue where you left off' })
      ).toHaveAttribute('href', '/shows/tigers-jaw-at-the-rebel-lounge')
    })
  })

  // PSY-1911: this surface used to carry its own handler and its own 429
  // wording. It now runs the shared control, so these pin the shared voice
  // reaching this surface rather than a second copy of the logic.
  describe('resend', () => {
    it('sends the verification email and parks the control on a cooldown', async () => {
      mockApiRequest.mockResolvedValueOnce({ success: true })
      const user = userEvent.setup()
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      await user.click(resendButton())

      expect(mockApiRequest).toHaveBeenCalledWith(
        expect.stringContaining('/auth/verify-email/send'),
        expect.objectContaining({ method: 'POST' })
      )
      await waitFor(() => {
        expect(
          screen.getByText('Sent · Check your inbox · Resend available in 60s')
        ).toBeInTheDocument()
      })
      expect(resendButton()).toBeDisabled()
      expect(screen.getByRole('status')).toHaveTextContent(
        'Verification email sent. Check your inbox.'
      )
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    })

    // A 429 is the expected outcome of an impatient second click, so it is a
    // wait everywhere, never the red alert this surface used to show.
    it('renders a throttled resend as a cooldown rather than an error', async () => {
      mockApiRequest.mockRejectedValueOnce(rateLimitError(45))
      const user = userEvent.setup()
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      await user.click(resendButton())

      await waitFor(() => {
        expect(screen.getByText('Resend available in 45s')).toBeInTheDocument()
      })
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
      expect(screen.queryByText(/Rate limit exceeded/)).not.toBeInTheDocument()
      expect(screen.queryByText(/lot of resends/)).not.toBeInTheDocument()
      expect(resendButton()).toBeDisabled()
    })

    // The production path: CORS hides Retry-After, so the app does not know the
    // wait and must not quote a second count off its own assumption.
    it('states the wait approximately when the 429 carries no Retry-After', async () => {
      mockApiRequest.mockRejectedValueOnce(rateLimitError())
      const user = userEvent.setup()
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      await user.click(resendButton())

      await waitFor(() => {
        expect(
          screen.getByText('Resend available in about a minute')
        ).toBeInTheDocument()
      })
      expect(screen.queryByText(/\d+s/)).not.toBeInTheDocument()
      expect(resendButton()).toBeDisabled()
    })

    it('shows generic copy on a server failure instead of the backend message', async () => {
      mockApiRequest.mockRejectedValueOnce(
        Object.assign(new Error('Email service is not configured'), {
          status: 500,
        })
      )
      const user = userEvent.setup()
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      await user.click(resendButton())

      await waitFor(() => {
        expect(screen.getByRole('alert')).toHaveTextContent(
          'We could not send that email just now. Please try again in a moment.'
        )
      })
      expect(
        screen.queryByText(/Email service is not configured/)
      ).not.toBeInTheDocument()
      // No cooldown was started, so a genuine failure stays retryable.
      expect(resendButton()).toBeEnabled()
    })
  })

  // There is no `/settings` index route and no change-email endpoint; the
  // account-email fold lives on the profile page's Settings tab.
  it('points the wrong-address escape hatch at the account settings tab', () => {
    renderWithProviders(
      <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
    )

    expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute(
      'href',
      '/profile?tab=settings'
    )
  })
})
