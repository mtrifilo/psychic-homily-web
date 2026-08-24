import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { apiRequest } from '@/lib/api'
import SubmitShowPage from './page'

// --- Mocks ---
//
// The REAL useSendVerificationEmail hook runs here, against a mocked
// apiRequest. Mocking the hook itself would freeze its state at render time, so
// pending/success/error would be presets rather than consequences of the click,
// and the tests would still pass with the click handler deleted.

let mockAuth: {
  isAuthenticated: boolean
  isLoading: boolean
  user: { email: string; email_verified: boolean; is_admin?: boolean } | null
} = {
  isAuthenticated: true,
  isLoading: false,
  user: { email: 'user@example.com', email_verified: false },
}

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuth,
}))

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: vi.fn(),
}))

vi.mock('@/features/shows', () => ({
  AIFormFiller: () => <div data-testid="ai-form-filler" />,
  ShowForm: () => <div data-testid="show-form" />,
}))

const mockApiRequest = vi.mocked(apiRequest)

const resendButton = () =>
  screen.getByRole('button', { name: /send verification email/i })

// --- Tests ---

describe('SubmitShowPage verification gate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuth = {
      isAuthenticated: true,
      isLoading: false,
      user: { email: 'user@example.com', email_verified: false },
    }
  })

  it('blocks an unverified user behind the submission-desk gate', () => {
    renderWithProviders(<SubmitShowPage />)

    expect(
      screen.getByRole('heading', { name: 'One step before you post.' })
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Submission desk · Verification needed/i)
    ).toBeInTheDocument()
    expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()
    expect(resendButton()).toBeInTheDocument()
    // The old spam-hygiene rationale is gone, not merely reworded.
    expect(screen.queryByText(/spam-free/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/real users/i)).not.toBeInTheDocument()
  })

  it('sends the verification email and parks the control on a cooldown', async () => {
    mockApiRequest.mockResolvedValueOnce({
      success: true,
      message: 'Verification email sent. Please check your inbox.',
    })

    renderWithProviders(<SubmitShowPage />)
    await userEvent.click(resendButton())

    expect(mockApiRequest).toHaveBeenCalledTimes(1)
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
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    // Assistive tech hears the state once, not the per-second tick, so the
    // live region carries no second count and the ticking line is hidden.
    expect(screen.getByRole('status')).toHaveTextContent(
      'Verification email sent. Check your inbox.'
    )
    expect(screen.getByRole('status')).not.toHaveTextContent(/\d/)
    expect(screen.getByText(/Resend available in 60s/)).toHaveAttribute(
      'aria-hidden',
      'true'
    )
  })

  // The PSY-1871 rate limiter answers 429 with Retry-After. That is an expected
  // outcome of an impatient second click, so it renders as a wait, never as an
  // error, and never leaks the backend's own wording.
  it('renders a throttled resend as a cooldown rather than an error', async () => {
    const throttled = Object.assign(
      new Error('Rate limit exceeded. Please try again in 60 seconds.'),
      { status: 429, retryAfter: 30 }
    )
    mockApiRequest.mockRejectedValueOnce(throttled)

    renderWithProviders(<SubmitShowPage />)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()

    await userEvent.click(resendButton())

    await waitFor(() => {
      expect(screen.getByText('Resend available in 30s')).toBeInTheDocument()
    })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByText(/Rate limit exceeded/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Check your inbox/i)).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent(
      'Resend is not available yet. Please wait a moment.'
    )
    expect(resendButton()).toBeDisabled()
  })

  // A cookie that expired while the gate sat open is an ordinary event, not a
  // send failure: it gets a way forward and stays out of Sentry.
  it('points an expired session at sign-in rather than a generic failure', async () => {
    mockApiRequest.mockRejectedValueOnce(
      Object.assign(new Error('unauthorized'), { status: 401 })
    )

    renderWithProviders(<SubmitShowPage />)

    await userEvent.click(resendButton())

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Your session has expired.'
      )
    })
    expect(screen.getByRole('link', { name: 'Sign in again' })).toHaveAttribute(
      'href',
      '/auth?returnTo=%2Fshows%2Fsubmit'
    )
    expect(
      screen.queryByText(/We could not send that email just now/)
    ).not.toBeInTheDocument()
  })

  it('shows generic copy on a server failure instead of the backend message', async () => {
    mockApiRequest.mockRejectedValueOnce(
      Object.assign(new Error('Email service is not configured'), { status: 500 })
    )

    renderWithProviders(<SubmitShowPage />)

    await userEvent.click(resendButton())

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        'We could not send that email just now. Please try again in a moment.'
      )
    })
    expect(
      screen.queryByText(/Email service is not configured/i)
    ).not.toBeInTheDocument()
    // No cooldown was started, so a genuine failure stays retryable.
    expect(resendButton()).toBeEnabled()
  })

  it('renders the submission form once the email is verified', () => {
    mockAuth.user = { email: 'user@example.com', email_verified: true }

    renderWithProviders(<SubmitShowPage />)

    expect(screen.getByTestId('show-form')).toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'One step before you post.' })
    ).not.toBeInTheDocument()
  })

  it('lets an admin through without verification', () => {
    mockAuth.user = {
      email: 'admin@example.com',
      email_verified: false,
      is_admin: true,
    }

    renderWithProviders(<SubmitShowPage />)

    expect(screen.getByTestId('show-form')).toBeInTheDocument()
  })
})
