import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { OAuthAccounts } from './oauth-accounts'

// --- Mocks ---

let mockOAuthData: {
  accounts?: {
    provider: string
    email?: string
    name?: string
    avatar_url?: string
    connected_at: string
  }[]
} | undefined = undefined

let mockOAuthLoading = false
let mockOAuthError: Error | null = null

const mockUnlinkMutateAsync = vi.fn()
let mockUnlinkMutationState = {
  isPending: false,
  isError: false,
  isSuccess: false,
  error: null as Error | null,
}

vi.mock('@/features/auth', () => ({
  useOAuthAccounts: () => ({
    data: mockOAuthData,
    isLoading: mockOAuthLoading,
    error: mockOAuthError,
  }),
  useUnlinkOAuthAccount: () => ({
    mutateAsync: mockUnlinkMutateAsync,
    ...mockUnlinkMutationState,
  }),
}))

// Mock Sentry to avoid import errors
vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
}))

// --- Tests ---

describe('OAuthAccounts', () => {
  beforeEach(() => {
    mockOAuthData = undefined
    mockOAuthLoading = false
    mockOAuthError = null
    mockUnlinkMutateAsync.mockReset()
    mockUnlinkMutationState = {
      isPending: false,
      isError: false,
      isSuccess: false,
      error: null,
    }
  })

  it('renders card title and description', () => {
    renderWithProviders(<OAuthAccounts />)

    expect(screen.getByText('Connected Accounts')).toBeInTheDocument()
    expect(
      screen.getByText('Manage your connected sign-in methods')
    ).toBeInTheDocument()
  })

  it('shows Google as "Not connected" when no Google account is linked', () => {
    mockOAuthData = { accounts: [] }
    renderWithProviders(<OAuthAccounts />)

    expect(screen.getByText('Google')).toBeInTheDocument()
    expect(screen.getByText('Not connected')).toBeInTheDocument()
  })

  it('shows Connect button when no Google account is linked', () => {
    mockOAuthData = { accounts: [] }
    renderWithProviders(<OAuthAccounts />)

    expect(screen.getByRole('button', { name: /Connect/ })).toBeInTheDocument()
  })

  it('shows connected Google account email and Disconnect button', () => {
    mockOAuthData = {
      accounts: [
        {
          provider: 'google',
          email: 'user@gmail.com',
          connected_at: '2025-06-15T10:00:00Z',
        },
      ],
    }
    renderWithProviders(<OAuthAccounts />)

    expect(screen.getByText('user@gmail.com')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Disconnect/ })
    ).toBeInTheDocument()
  })

  it('shows connected Google account name when no email', () => {
    mockOAuthData = {
      accounts: [
        {
          provider: 'google',
          name: 'Test User',
          connected_at: '2025-06-15T10:00:00Z',
        },
      ],
    }
    renderWithProviders(<OAuthAccounts />)

    expect(screen.getByText('Test User')).toBeInTheDocument()
  })

  it('shows fallback "Google Account" when neither email nor name exists', () => {
    mockOAuthData = {
      accounts: [
        {
          provider: 'google',
          connected_at: '2025-06-15T10:00:00Z',
        },
      ],
    }
    renderWithProviders(<OAuthAccounts />)

    expect(screen.getByText('Google Account')).toBeInTheDocument()
  })

  it('shows error alert when fetching accounts fails', () => {
    mockOAuthError = new Error('Network error')
    renderWithProviders(<OAuthAccounts />)

    expect(
      screen.getByText('Failed to load connected accounts')
    ).toBeInTheDocument()
  })

  it('opens disconnect confirmation dialog when Disconnect is clicked', async () => {
    mockOAuthData = {
      accounts: [
        {
          provider: 'google',
          email: 'user@gmail.com',
          connected_at: '2025-06-15T10:00:00Z',
        },
      ],
    }
    const user = userEvent.setup()
    renderWithProviders(<OAuthAccounts />)

    await user.click(screen.getByRole('button', { name: /Disconnect/ }))

    expect(
      screen.getByText('Disconnect Google Account?')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        /You will no longer be able to sign in with this Google account/
      )
    ).toBeInTheDocument()
  })

  it('shows unlink error message when unlink mutation fails', () => {
    mockUnlinkMutationState = {
      isPending: false,
      isError: true,
      isSuccess: false,
      error: new Error('Cannot unlink last auth method'),
    }
    renderWithProviders(<OAuthAccounts />)

    expect(
      screen.getByText('Cannot unlink last auth method')
    ).toBeInTheDocument()
  })

  it('shows success message after successful unlink', () => {
    mockUnlinkMutationState = {
      isPending: false,
      isError: false,
      isSuccess: true,
      error: null,
    }
    renderWithProviders(<OAuthAccounts />)

    expect(
      screen.getByText('Account disconnected successfully')
    ).toBeInTheDocument()
  })
})
