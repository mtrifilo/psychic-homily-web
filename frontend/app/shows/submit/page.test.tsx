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

  it('blocks an unverified user and offers an inline resend', () => {
    renderWithProviders(<SubmitShowPage />)

    expect(screen.getByText('Email Verification Required')).toBeInTheDocument()
    expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()
    expect(resendButton()).toBeInTheDocument()
  })

  it('sends the verification email and confirms it inline', async () => {
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
        screen.getByText('Verification email sent! Check your inbox.')
      ).toBeInTheDocument()
    })
  })

  // A throttled resend (the PSY-1871 rate limiter returns 429) must show the
  // backend's message rather than a success latch.
  it('surfaces a send failure without claiming success', async () => {
    mockApiRequest.mockResolvedValueOnce({
      success: false,
      message: 'Rate limit exceeded. Please try again in 60 seconds.',
      error_code: 'SERVICE_UNAVAILABLE',
    })

    renderWithProviders(<SubmitShowPage />)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()

    await userEvent.click(resendButton())

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Rate limit exceeded. Please try again in 60 seconds.'
      )
    })
    expect(
      screen.queryByText(/verification email sent/i)
    ).not.toBeInTheDocument()
    // The button stays available so a throttled user can retry after waiting.
    expect(resendButton()).toBeInTheDocument()
  })

  it('renders the submission form once the email is verified', () => {
    mockAuth.user = { email: 'user@example.com', email_verified: true }

    renderWithProviders(<SubmitShowPage />)

    expect(screen.getByTestId('show-form')).toBeInTheDocument()
    expect(
      screen.queryByText('Email Verification Required')
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
