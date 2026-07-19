import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { APITokenManagement } from './api-token-management'

// --- Mocks ---


let mockTokensData: {
  tokens: {
    id: number
    description: string | null
    scope: string
    created_at: string
    expires_at: string
    last_used_at: string | null
    is_expired: boolean
  }[]
} | undefined = undefined

let mockTokensLoading = false
let mockTokensError: Error | null = null

const mockCreateMutateAsync = vi.fn()
let mockCreateMutationState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

const mockRevokeMutateAsync = vi.fn()
let mockRevokeMutationState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

vi.mock('@/features/auth', () => ({
  useAPITokens: () => ({
    data: mockTokensData,
    isLoading: mockTokensLoading,
    error: mockTokensError,
  }),
  useCreateAPIToken: () => ({
    mutateAsync: mockCreateMutateAsync,
    ...mockCreateMutationState,
  }),
  useRevokeAPIToken: () => ({
    mutateAsync: mockRevokeMutateAsync,
    ...mockRevokeMutationState,
  }),
}))

// --- Tests ---

describe('APITokenManagement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockTokensData = undefined
    mockTokensLoading = false
    mockTokensError = null
    mockCreateMutateAsync.mockReset()
    mockRevokeMutateAsync.mockReset()
    mockCreateMutationState = { isPending: false, isError: false, error: null }
    mockRevokeMutationState = { isPending: false, isError: false, error: null }
  })

  it('renders card title and description', () => {
    renderWithProviders(<APITokenManagement />)

    expect(screen.getByText('API tokens')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Long-lived tokens for the local discovery app and other admin tools'
      )
    ).toBeInTheDocument()
  })

  it('shows Generate new token button', () => {
    renderWithProviders(<APITokenManagement />)

    expect(
      screen.getByRole('button', { name: /Generate new token/ })
    ).toBeInTheDocument()
  })

  it('shows loading state', () => {
    mockTokensLoading = true
    renderWithProviders(<APITokenManagement />)

    // The loading spinner should be present (no text to check, just no error or empty state)
    expect(screen.queryByText('No API tokens yet')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Failed to load tokens')
    ).not.toBeInTheDocument()
  })

  it('shows error state when tokens fail to load', () => {
    mockTokensError = new Error('Server error')
    renderWithProviders(<APITokenManagement />)

    expect(
      screen.getByText('Failed to load tokens. Please try again.')
    ).toBeInTheDocument()
  })

  it('shows empty state when no tokens exist', () => {
    mockTokensData = { tokens: [] }
    renderWithProviders(<APITokenManagement />)

    expect(screen.getByText('No API tokens yet')).toBeInTheDocument()
    expect(
      screen.getByText('Create a token to use with the local discovery app')
    ).toBeInTheDocument()
  })

  it('renders token rows with descriptions', () => {
    mockTokensData = {
      tokens: [
        {
          id: 1,
          description: 'Discovery App',
          scope: 'admin',
          created_at: '2025-06-01T10:00:00Z',
          expires_at: '2025-09-01T10:00:00Z',
          last_used_at: '2025-07-15T14:30:00Z',
          is_expired: false,
        },
        {
          id: 2,
          description: null,
          scope: 'admin',
          created_at: '2025-05-01T10:00:00Z',
          expires_at: '2025-08-01T10:00:00Z',
          last_used_at: null,
          is_expired: true,
        },
      ],
    }
    renderWithProviders(<APITokenManagement />)

    expect(screen.getByText('Discovery App')).toBeInTheDocument()
    expect(screen.getByText('Unnamed token')).toBeInTheDocument()
    expect(screen.getByText('Expired')).toBeInTheDocument()
  })

  it('shows Active badge for non-expired tokens', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2025-06-15T00:00:00Z'))

    mockTokensData = {
      tokens: [
        {
          id: 1,
          description: 'My Token',
          scope: 'admin',
          created_at: '2025-06-01T10:00:00Z',
          expires_at: '2025-12-01T10:00:00Z',
          last_used_at: null,
          is_expired: false,
        },
      ],
    }
    renderWithProviders(<APITokenManagement />)

    expect(screen.getByText('Active')).toBeInTheDocument()

    vi.useRealTimers()
  })

  it('shows Token Usage info section', () => {
    mockTokensData = { tokens: [] }
    renderWithProviders(<APITokenManagement />)

    expect(screen.getByText('Token Usage')).toBeInTheDocument()
    expect(
      screen.getByText(/Use these tokens with the local discovery app/)
    ).toBeInTheDocument()
  })

  it('opens create token dialog when Generate new token is clicked', async () => {
    mockTokensData = { tokens: [] }
    const user = userEvent.setup()
    renderWithProviders(<APITokenManagement />)

    await user.click(screen.getByRole('button', { name: /Generate new token/ }))

    expect(screen.getByText('Create API Token')).toBeInTheDocument()
    expect(screen.getByLabelText('Description (optional)')).toBeInTheDocument()
    expect(screen.getByLabelText('Expiration (days)')).toBeInTheDocument()
  })

  it('shows create mutation error', () => {
    mockTokensData = { tokens: [] }
    mockCreateMutationState = {
      isPending: false,
      isError: true,
      error: new Error('Rate limit exceeded'),
    }
    renderWithProviders(<APITokenManagement />)

    expect(screen.getByText('Rate limit exceeded')).toBeInTheDocument()
  })

  it('shows revoke mutation error', () => {
    mockTokensData = { tokens: [] }
    mockRevokeMutationState = {
      isPending: false,
      isError: true,
      error: new Error('Token not found'),
    }
    renderWithProviders(<APITokenManagement />)

    expect(screen.getByText('Token not found')).toBeInTheDocument()
  })

  it('opens revoke confirmation dialog when delete button is clicked on a token', async () => {
    mockTokensData = {
      tokens: [
        {
          id: 1,
          description: 'Test Token',
          scope: 'admin',
          created_at: '2025-06-01T10:00:00Z',
          expires_at: '2025-12-01T10:00:00Z',
          last_used_at: null,
          is_expired: false,
        },
      ],
    }
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.setSystemTime(new Date('2025-06-15T00:00:00Z'))

    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    renderWithProviders(<APITokenManagement />)

    // Find the delete/revoke button on the token row (the ghost icon button)
    const revokeButtons = screen.getAllByRole('button')
    // The delete button is the one that's not "Generate new token"
    const deleteBtn = revokeButtons.find(
      btn =>
        btn.textContent !== 'Generate new token' &&
        !btn.textContent?.includes('Create')
    )
    expect(deleteBtn).toBeDefined()

    await user.click(deleteBtn!)

    await waitFor(() => {
      expect(screen.getByText('Revoke API Token')).toBeInTheDocument()
    })
    expect(
      screen.getByText(
        /Are you sure you want to revoke this token/
      )
    ).toBeInTheDocument()

    vi.useRealTimers()
  })

  it('calls revokeToken mutation when Revoke Token is confirmed in dialog', async () => {
    mockTokensData = {
      tokens: [
        {
          id: 42,
          description: 'Confirm-revoke token',
          scope: 'admin',
          created_at: '2025-06-01T10:00:00Z',
          expires_at: '2025-12-01T10:00:00Z',
          last_used_at: null,
          is_expired: false,
        },
      ],
    }
    mockRevokeMutateAsync.mockResolvedValueOnce(undefined)

    const user = userEvent.setup()
    renderWithProviders(<APITokenManagement />)

    // Open revoke dialog via the trash icon button on the row.
    const allButtons = screen.getAllByRole('button')
    const deleteBtn = allButtons.find(
      btn =>
        btn.textContent !== 'Generate new token' &&
        !btn.textContent?.includes('Create')
    )
    await user.click(deleteBtn!)

    // Click the destructive confirm button inside the dialog.
    await waitFor(() => {
      expect(screen.getByText('Revoke API Token')).toBeInTheDocument()
    })
    const confirmBtn = screen.getByRole('button', { name: 'Revoke Token' })
    await user.click(confirmBtn)

    expect(mockRevokeMutateAsync).toHaveBeenCalledWith(42)
  })

  it('calls createToken with description + parsed expiration_days, then shows the issued token', async () => {
    mockTokensData = { tokens: [] }
    mockCreateMutateAsync.mockResolvedValueOnce({
      token: 'ph_test_token_abcdef123',
    })

    const user = userEvent.setup()
    renderWithProviders(<APITokenManagement />)

    // Open create dialog.
    await user.click(screen.getByRole('button', { name: /Generate new token/ }))

    // Fill description, override expiration_days.
    await user.type(
      screen.getByLabelText('Description (optional)'),
      'Discovery laptop'
    )
    const expirationInput = screen.getByLabelText('Expiration (days)')
    await user.clear(expirationInput)
    await user.type(expirationInput, '30')

    // Submit (trigger is "Generate new token"; dialog submit is still "Create Token" —
    // confirm button inside the dialog footer). Pick the dialog one.
    const submitBtn = screen.getAllByRole('button', { name: /Create Token/ })
      .find(btn => btn.closest('[role="dialog"]'))
    expect(submitBtn).toBeDefined()
    await user.click(submitBtn!)

    expect(mockCreateMutateAsync).toHaveBeenCalledWith({
      description: 'Discovery laptop',
      expiration_days: 30,
    })

    // The issued-token state should render the new token + a Copy button.
    await waitFor(() => {
      expect(screen.getByText('Token Created')).toBeInTheDocument()
    })
    expect(screen.getByText('ph_test_token_abcdef123')).toBeInTheDocument()
  })

  it('omits empty description from create payload and defaults expiration_days to 90', async () => {
    mockTokensData = { tokens: [] }
    mockCreateMutateAsync.mockResolvedValueOnce({ token: 'ph_default' })

    const user = userEvent.setup()
    renderWithProviders(<APITokenManagement />)

    await user.click(screen.getByRole('button', { name: /Generate new token/ }))

    // Don't touch the description (empty) or expiration_days (default 90).
    const submitBtn = screen.getAllByRole('button', { name: /Create Token/ })
      .find(btn => btn.closest('[role="dialog"]'))
    await user.click(submitBtn!)

    expect(mockCreateMutateAsync).toHaveBeenCalledWith({
      description: undefined,
      expiration_days: 90,
    })
  })

  it('copies the issued token to the clipboard and shows "Copied!" feedback', async () => {
    mockTokensData = { tokens: [] }
    mockCreateMutateAsync.mockResolvedValueOnce({
      token: 'ph_copyable_token',
    })

    // userEvent v14 installs a clipboard stub on navigator.clipboard at
    // setup() time. Spying after setup() ensures the test sees the call.
    const user = userEvent.setup()
    const writeTextSpy = vi
      .spyOn(navigator.clipboard, 'writeText')
      .mockResolvedValue(undefined)

    renderWithProviders(<APITokenManagement />)

    // Issue the token first.
    await user.click(screen.getByRole('button', { name: /Generate new token/ }))
    const submitBtn = screen.getAllByRole('button', { name: /Create Token/ })
      .find(btn => btn.closest('[role="dialog"]'))
    await user.click(submitBtn!)

    // Wait for the issued-token panel to render.
    await waitFor(() => {
      expect(screen.getByText('ph_copyable_token')).toBeInTheDocument()
    })

    // The Copy icon button is the sibling button to the <code> element
    // inside the issued-token panel.
    const tokenCodeEl = screen.getByText('ph_copyable_token')
    const copyBtn = tokenCodeEl.parentElement?.querySelector('button')
    expect(copyBtn).toBeTruthy()
    await user.click(copyBtn!)

    await waitFor(() => {
      expect(writeTextSpy).toHaveBeenCalledWith('ph_copyable_token')
    })
    expect(
      screen.getByText('Token copied to clipboard!')
    ).toBeInTheDocument()

    writeTextSpy.mockRestore()
  })

  it('disables the in-dialog Create Token submit button while the mutation is pending', async () => {
    mockTokensData = { tokens: [] }
    mockCreateMutationState = {
      isPending: true,
      isError: false,
      error: null,
    }
    const user = userEvent.setup()
    renderWithProviders(<APITokenManagement />)

    // Open the dialog so the in-dialog submit button is mounted; the
    // `mutation.isPending` flag drives `disabled` on that button.
    await user.click(screen.getByRole('button', { name: /Generate new token/ }))

    const submitBtn = screen
      .getAllByRole('button', { name: /Create Token/ })
      .find(btn => btn.closest('[role="dialog"]'))
    expect(submitBtn).toBeDefined()
    expect(submitBtn).toBeDisabled()
  })
})
