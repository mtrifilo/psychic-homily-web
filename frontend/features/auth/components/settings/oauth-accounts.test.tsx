import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
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
const mockCaptureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => mockCaptureException(...args),
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
    mockCaptureException.mockReset()
  })

  it('renders card title and description', () => {
    renderWithProviders(<OAuthAccounts />)

    expect(screen.getByText('Connected accounts')).toBeInTheDocument()
    expect(
      screen.getByText('OAuth sign-in methods linked to this account.')
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

  it('shows fallback "Connected" when neither email nor name exists', () => {
    mockOAuthData = {
      accounts: [
        {
          provider: 'google',
          connected_at: '2025-06-15T10:00:00Z',
        },
      ],
    }
    renderWithProviders(<OAuthAccounts />)

    expect(screen.getByText('Connected')).toBeInTheDocument()
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

  it('shows generic error message when unlink error has no message', () => {
    mockUnlinkMutationState = {
      isPending: false,
      isError: true,
      isSuccess: false,
      error: null,
    }
    renderWithProviders(<OAuthAccounts />)

    expect(
      screen.getByText('Failed to disconnect account')
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

  it('shows loading spinner when accounts are loading', () => {
    mockOAuthLoading = true
    renderWithProviders(<OAuthAccounts />)

    // When loading, no Connect or Disconnect button is shown, just a spinner
    expect(screen.queryByRole('button', { name: /Connect/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Disconnect/ })).not.toBeInTheDocument()
  })

  it('shows Google label with email for connected accounts', () => {
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

    // Board J: "Google" label + mono email (no connected_at date row)
    expect(screen.getAllByText('Google').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('user@gmail.com')).toBeInTheDocument()
  })

  it('calls mutateAsync with provider when Disconnect is confirmed', async () => {
    mockOAuthData = {
      accounts: [
        {
          provider: 'google',
          email: 'user@gmail.com',
          connected_at: '2025-06-15T10:00:00Z',
        },
      ],
    }
    mockUnlinkMutateAsync.mockResolvedValueOnce(undefined)

    const user = userEvent.setup()
    renderWithProviders(<OAuthAccounts />)

    // Open dialog
    await user.click(screen.getByRole('button', { name: /Disconnect/ }))

    // Click Disconnect in the dialog
    const dialogDisconnect = screen.getAllByRole('button').find(
      btn => btn.textContent === 'Disconnect' && btn.closest('[role="dialog"]')
    )
    expect(dialogDisconnect).toBeDefined()
    if (dialogDisconnect) {
      await user.click(dialogDisconnect)
    }

    expect(mockUnlinkMutateAsync).toHaveBeenCalledWith('google')
  })

  it('reports to Sentry when unlink fails', async () => {
    mockOAuthData = {
      accounts: [
        {
          provider: 'google',
          email: 'user@gmail.com',
          connected_at: '2025-06-15T10:00:00Z',
        },
      ],
    }
    const unlinkError = new Error('Server error')
    mockUnlinkMutateAsync.mockRejectedValueOnce(unlinkError)

    const user = userEvent.setup()
    renderWithProviders(<OAuthAccounts />)

    // Open dialog
    await user.click(screen.getByRole('button', { name: /Disconnect/ }))

    // Confirm disconnect
    const dialogDisconnect = screen.getAllByRole('button').find(
      btn => btn.textContent === 'Disconnect' && btn.closest('[role="dialog"]')
    )
    if (dialogDisconnect) {
      await user.click(dialogDisconnect)
    }

    expect(mockCaptureException).toHaveBeenCalledWith(
      unlinkError,
      expect.objectContaining({
        level: 'warning',
        tags: { service: 'oauth-accounts' },
        extra: { provider: 'google' },
      })
    )
  })

  it('closes dialog when Cancel is clicked', async () => {
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

    // Open dialog
    await user.click(screen.getByRole('button', { name: /Disconnect/ }))
    expect(screen.getByText('Disconnect Google Account?')).toBeInTheDocument()

    // Click Cancel
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText('Disconnect Google Account?')).not.toBeInTheDocument()
  })

  it('redirects to Google OAuth when Connect is clicked', async () => {
    mockOAuthData = { accounts: [] }

    // Mock window.location
    const originalLocation = window.location
    const mockAssign = vi.fn()
    Object.defineProperty(window, 'location', {
      value: { ...originalLocation, href: '' },
      writable: true,
      configurable: true,
    })
    Object.defineProperty(window.location, 'href', {
      set: mockAssign,
      configurable: true,
    })

    const user = userEvent.setup()
    renderWithProviders(<OAuthAccounts />)

    await user.click(screen.getByRole('button', { name: /Connect/ }))

    expect(mockAssign).toHaveBeenCalledWith(
      expect.stringContaining('/auth/login/google')
    )

    // Restore
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
  })

  it('shows error and success alerts using role="alert"', () => {
    mockOAuthError = new Error('Network error')
    renderWithProviders(<OAuthAccounts />)

    const alerts = screen.getAllByRole('alert')
    expect(alerts.length).toBeGreaterThanOrEqual(1)
  })

  it('shows pending spinner on Disconnect button during unlink', () => {
    mockOAuthData = {
      accounts: [
        {
          provider: 'google',
          email: 'user@gmail.com',
          connected_at: '2025-06-15T10:00:00Z',
        },
      ],
    }
    mockUnlinkMutationState = {
      isPending: true,
      isError: false,
      isSuccess: false,
      error: null,
    }
    renderWithProviders(<OAuthAccounts />)

    // The disconnect button should be disabled during pending
    const disconnectBtn = screen.getByRole('button', { name: /Disconnect/ })
    expect(disconnectBtn).toBeDisabled()
  })

  it('closes the confirmation dialog after a successful unlink', async () => {
    mockOAuthData = {
      accounts: [
        {
          provider: 'google',
          email: 'user@gmail.com',
          connected_at: '2025-06-15T10:00:00Z',
        },
      ],
    }
    mockUnlinkMutateAsync.mockResolvedValueOnce(undefined)

    const user = userEvent.setup()
    renderWithProviders(<OAuthAccounts />)

    await user.click(screen.getByRole('button', { name: /Disconnect/ }))
    expect(screen.getByText('Disconnect Google Account?')).toBeInTheDocument()

    // Confirm disconnect.
    const dialogDisconnect = screen.getAllByRole('button').find(
      btn => btn.textContent === 'Disconnect' && btn.closest('[role="dialog"]')
    )
    if (dialogDisconnect) {
      await user.click(dialogDisconnect)
    }

    // After the awaited mutateAsync resolves, the dialog should close —
    // `setUnlinkProvider(null)` in handleUnlink triggers the close.
    await waitFor(() => {
      expect(
        screen.queryByText('Disconnect Google Account?')
      ).not.toBeInTheDocument()
    })
  })

  it('keeps the confirmation dialog open when unlink rejects', async () => {
    mockOAuthData = {
      accounts: [
        {
          provider: 'google',
          email: 'user@gmail.com',
          connected_at: '2025-06-15T10:00:00Z',
        },
      ],
    }
    mockUnlinkMutateAsync.mockRejectedValueOnce(new Error('Server error'))

    const user = userEvent.setup()
    renderWithProviders(<OAuthAccounts />)

    await user.click(screen.getByRole('button', { name: /Disconnect/ }))

    const dialogDisconnect = screen.getAllByRole('button').find(
      btn => btn.textContent === 'Disconnect' && btn.closest('[role="dialog"]')
    )
    if (dialogDisconnect) {
      await user.click(dialogDisconnect)
    }

    // After Sentry capture, the dialog should stay open so the user can retry.
    await waitFor(() => {
      expect(mockCaptureException).toHaveBeenCalled()
    })
    expect(screen.getByText('Disconnect Google Account?')).toBeInTheDocument()
  })
})
