import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import SubmitShowPage from './page'

// --- Mocks ---

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

const mockSendVerificationMutateAsync = vi.fn()
let mockSendVerificationState = {
  isPending: false,
  isError: false,
  isSuccess: false,
  error: null as Error | null,
}

vi.mock('@/features/auth', () => ({
  useSendVerificationEmail: () => ({
    mutateAsync: mockSendVerificationMutateAsync,
    ...mockSendVerificationState,
  }),
}))

vi.mock('@/features/shows', () => ({
  AIFormFiller: () => <div data-testid="ai-form-filler" />,
  ShowForm: () => <div data-testid="show-form" />,
}))

// --- Tests ---

describe('SubmitShowPage verification gate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuth = {
      isAuthenticated: true,
      isLoading: false,
      user: { email: 'user@example.com', email_verified: false },
    }
    mockSendVerificationState = {
      isPending: false,
      isError: false,
      isSuccess: false,
      error: null,
    }
    mockSendVerificationMutateAsync.mockReset()
  })

  it('blocks an unverified user and offers an inline resend', () => {
    renderWithProviders(<SubmitShowPage />)

    expect(screen.getByText('Email Verification Required')).toBeInTheDocument()
    expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /send verification email/i })
    ).toBeInTheDocument()
  })

  it('sends the verification email and confirms it inline', async () => {
    mockSendVerificationMutateAsync.mockResolvedValueOnce(undefined)
    mockSendVerificationState = {
      isPending: false,
      isError: false,
      isSuccess: true,
      error: null,
    }

    renderWithProviders(<SubmitShowPage />)
    await userEvent.click(
      screen.getByRole('button', { name: /send verification email/i })
    )

    expect(mockSendVerificationMutateAsync).toHaveBeenCalledTimes(1)
    await waitFor(() => {
      expect(
        screen.getByText(/verification email sent/i)
      ).toBeInTheDocument()
    })
  })

  it('surfaces a send failure without claiming success', async () => {
    mockSendVerificationMutateAsync.mockRejectedValueOnce(new Error('boom'))
    mockSendVerificationState = {
      isPending: false,
      isError: true,
      isSuccess: false,
      error: new Error('Too many requests. Try again later.'),
    }

    renderWithProviders(<SubmitShowPage />)
    await userEvent.click(
      screen.getByRole('button', { name: /send verification email/i })
    )

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Too many requests. Try again later.'
      )
    })
    expect(screen.queryByText(/verification email sent/i)).not.toBeInTheDocument()
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
