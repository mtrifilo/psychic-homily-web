import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ApiError } from '@/lib/api'
import { CheckInboxInterstitial } from './_components/check-inbox-interstitial'

// --- Mocks ---

const mockResendMutate = vi.fn()
let mockResendState: {
  isPending: boolean
  isSuccess: boolean
  isError: boolean
  error: Error | null
} = { isPending: false, isSuccess: false, isError: false, error: null }

vi.mock('@/features/auth', () => ({
  useSendVerificationEmail: () => ({
    mutate: mockResendMutate,
    ...mockResendState,
  }),
}))

function rateLimitError(retryAfter?: number): ApiError {
  const error: ApiError = new Error('rate limit exceeded')
  error.status = 429
  error.retryAfter = retryAfter
  return error
}

describe('CheckInboxInterstitial', () => {
  beforeEach(() => {
    mockResendMutate.mockReset()
    mockResendState = {
      isPending: false,
      isSuccess: false,
      isError: false,
      error: null,
    }
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
    it('fires the resend mutation', async () => {
      const user = userEvent.setup()
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      await user.click(screen.getByRole('button', { name: 'Resend email' }))

      expect(mockResendMutate).toHaveBeenCalledTimes(1)
    })

    it('confirms a successful resend', () => {
      mockResendState = {
        isPending: false,
        isSuccess: true,
        isError: false,
        error: null,
      }
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      // Announced, not just rendered: the failure branch is a live region, so
      // the success branch has to be one too.
      expect(screen.getByRole('status')).toHaveTextContent('Sent again.')
    })

    it('disables the button while a resend is in flight', () => {
      mockResendState = {
        isPending: true,
        isSuccess: false,
        isError: false,
        error: null,
      }
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      expect(screen.getByRole('button', { name: /Sending/ })).toBeDisabled()
    })

    // The resend endpoint is rate-limited per IP (PSY-1871). Retry-After is the
    // only part of a 429 the user can act on, so it has to reach the copy.
    it('turns a 429 into the wait the Retry-After header asked for', () => {
      mockResendState = {
        isPending: false,
        isSuccess: false,
        isError: true,
        error: rateLimitError(45),
      }
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      expect(screen.getByRole('alert')).toHaveTextContent(
        'That is a lot of resends. Try again in 45s.'
      )
    })

    it('falls back to a generic wait when the 429 carries no Retry-After', () => {
      mockResendState = {
        isPending: false,
        isSuccess: false,
        isError: true,
        error: rateLimitError(),
      }
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      expect(screen.getByRole('alert')).toHaveTextContent(
        'That is a lot of resends. Try again in a minute.'
      )
    })

    it('surfaces a non-rate-limit failure message', () => {
      mockResendState = {
        isPending: false,
        isSuccess: false,
        isError: true,
        error: new Error('Failed to send verification email'),
      }
      renderWithProviders(
        <CheckInboxInterstitial email="listener@example.com" returnTo="/" />
      )

      expect(screen.getByRole('alert')).toHaveTextContent(
        'Failed to send verification email'
      )
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
