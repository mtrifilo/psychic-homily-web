import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as Sentry from '@sentry/nextjs'
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

  describe('resend', () => {
    it('confirms a send in its own words and parks the control on a cooldown', async () => {
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
        expect(screen.getByRole('status')).toHaveTextContent(
          'Sent again. Give it a minute to arrive.'
        )
      })
      // The shared line carries only the wait; the confirmation above says the
      // rest, so "Sent · Check your inbox" would say it twice.
      expect(screen.getByText('Resend available in 60s')).toHaveAttribute(
        'aria-hidden',
        'true'
      )
      expect(screen.queryByText(/Check your inbox ·/)).not.toBeInTheDocument()
      expect(resendButton()).toBeDisabled()
      // Announced in the same words it shows, once: the live region speaks
      // the sentence and the visible copy of it stays out of the a11y tree.
      expect(screen.getByRole('status')).toHaveTextContent(
        'Sent again. Give it a minute to arrive.'
      )
      expect(
        screen.getByText('Sent again. Give it a minute to arrive.', {
          selector: 'p[aria-hidden="true"]',
        })
      ).toBeInTheDocument()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    })

    it('takes the confirmation down when a later attempt fails', async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true })
      try {
        mockApiRequest
          .mockResolvedValueOnce({ success: true })
          .mockRejectedValueOnce(
            Object.assign(new Error('boom'), { status: 500 })
          )
        const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
        renderWithProviders(
          <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
        )

        await user.click(resendButton())
        await waitFor(() => expect(resendButton()).toBeDisabled())
        act(() => {
          vi.advanceTimersByTime(60_000)
        })
        await user.click(resendButton())

        await waitFor(() => {
          expect(screen.getByRole('alert')).toHaveTextContent(
            'We could not send that email just now.'
          )
        })
        expect(screen.queryByText(/Sent again/, { selector: 'p[aria-hidden="true"]' })).not.toBeInTheDocument()
      } finally {
        vi.useRealTimers()
      }
    })

    it('keeps its own in-flight label while a send is pending', async () => {
      let resolveSend: (value: { success: boolean }) => void = () => undefined
      mockApiRequest.mockReturnValueOnce(
        new Promise(resolve => {
          resolveSend = resolve
        })
      )
      const user = userEvent.setup()
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      await user.click(resendButton())

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Sending...' })).toBeDisabled()
      })

      resolveSend({ success: true })
      await waitFor(() => {
        expect(screen.getByRole('status')).toHaveTextContent(
          'Sent again. Give it a minute to arrive.'
        )
      })
    })

    // A 429 is the expected outcome of an impatient second click, so it is the
    // same parked wait here as on every other resend surface, never an alert.
    it('renders a throttled resend as a cooldown from Retry-After', async () => {
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
      expect(screen.queryByText(/Sent again/)).not.toBeInTheDocument()
      expect(resendButton()).toBeDisabled()
      expect(screen.getByRole('status')).toHaveTextContent(
        'Resend is not available yet. Please wait a moment.'
      )
    })

    it('falls back to the standard cooldown when the 429 carries no Retry-After', async () => {
      mockApiRequest.mockRejectedValueOnce(rateLimitError())
      const user = userEvent.setup()
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      await user.click(resendButton())

      await waitFor(() => {
        expect(screen.getByText('Resend available in 60s')).toBeInTheDocument()
      })
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
      expect(resendButton()).toBeDisabled()
    })

    it('points a dead session at sign-in, back to where the reader was headed', async () => {
      mockApiRequest.mockRejectedValueOnce(
        Object.assign(new Error('unauthorized'), { status: 401 })
      )
      const user = userEvent.setup()
      renderWithProviders(
        <CheckInboxInterstitial
          email="listener@example.com"
          returnTo="/shows/tigers-jaw-at-the-rebel-lounge"
        />
      )

      await user.click(resendButton())

      await waitFor(() => {
        expect(screen.getByRole('alert')).toHaveTextContent(
          'Your session has expired. Sign in again to send the email.'
        )
      })
      expect(screen.getByRole('link', { name: 'Sign in again' })).toHaveAttribute(
        'href',
        '/auth?returnTo=%2Fshows%2Ftigers-jaw-at-the-rebel-lounge'
      )
      expect(Sentry.captureException).not.toHaveBeenCalled()
    })

    // Verified in another tab or on another device while this card sat open.
    it('says the address is already verified rather than inviting a retry', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Email is already verified',
        error_code: 'ALREADY_VERIFIED',
      })
      const user = userEvent.setup()
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      await user.click(resendButton())

      await waitFor(() => {
        expect(screen.getByRole('alert')).toHaveTextContent(
          'Email is already verified'
        )
      })
      expect(screen.queryByText(/try again/i)).not.toBeInTheDocument()
      expect(Sentry.captureException).not.toHaveBeenCalled()
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
      expect(Sentry.captureException).toHaveBeenCalledWith(
        expect.any(Error),
        expect.objectContaining({
          tags: { service: 'auth_check_inbox', error_type: 'verification_email' },
        })
      )
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
