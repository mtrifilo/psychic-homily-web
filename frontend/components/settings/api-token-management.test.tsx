import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { APITokenManagement } from './api-token-management'

// --- Mocks ---

vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
}))

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

    expect(screen.getByText('API Tokens')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Long-lived tokens for the local discovery app and other admin tools'
      )
    ).toBeInTheDocument()
  })

  it('shows Create Token button', () => {
    renderWithProviders(<APITokenManagement />)

    expect(
      screen.getByRole('button', { name: /Create Token/ })
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

  it('opens create token dialog when Create Token is clicked', async () => {
    mockTokensData = { tokens: [] }
    const user = userEvent.setup()
    renderWithProviders(<APITokenManagement />)

    await user.click(screen.getByRole('button', { name: /Create Token/ }))

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
    // The delete button is the one that's not "Create Token"
    const deleteBtn = revokeButtons.find(
      btn =>
        btn.textContent !== 'Create Token' &&
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
})
