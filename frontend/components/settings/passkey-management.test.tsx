import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { PasskeyManagement } from './passkey-management'

// --- Mocks ---

let mockSupportsWebAuthn = true

vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: () => mockSupportsWebAuthn,
}))

vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
}))

vi.mock('@/features/auth', () => ({
  PasskeyRegisterButton: ({ onSuccess, onError }: { onSuccess: () => void; onError: (err: string) => void }) => (
    <button onClick={() => onSuccess()} data-testid="register-passkey-btn">
      Add Passkey
    </button>
  ),
}))

let mockFetchResponse: {
  ok: boolean
  json: () => Promise<unknown>
} = {
  ok: true,
  json: async () => ({ success: true, credentials: [] }),
}

// --- Tests ---

describe('PasskeyManagement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSupportsWebAuthn = true
    mockFetchResponse = {
      ok: true,
      json: async () => ({ success: true, credentials: [] }),
    }
    vi.spyOn(global, 'fetch').mockImplementation(async () =>
      mockFetchResponse as Response
    )
  })

  it('shows "browser does not support passkeys" when WebAuthn not supported', () => {
    mockSupportsWebAuthn = false
    renderWithProviders(<PasskeyManagement />)

    expect(screen.getByText('Passkeys')).toBeInTheDocument()
    expect(
      screen.getByText('Your browser does not support passkeys.')
    ).toBeInTheDocument()
  })

  it('renders card title and description when WebAuthn is supported', async () => {
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(screen.getByText('Passkeys')).toBeInTheDocument()
    })
    expect(
      screen.getByText(/Passkeys let you sign in securely/)
    ).toBeInTheDocument()
  })

  it('shows empty state when no credentials exist', async () => {
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(
        screen.getByText(/No passkeys registered yet/)
      ).toBeInTheDocument()
    })
  })

  it('renders credentials list when passkeys exist', async () => {
    mockFetchResponse = {
      ok: true,
      json: async () => ({
        success: true,
        credentials: [
          {
            id: 1,
            display_name: 'MacBook Pro',
            created_at: '2025-06-15T10:00:00Z',
            last_used_at: '2025-07-01T14:30:00Z',
            backup_eligible: true,
            backup_state: true,
          },
          {
            id: 2,
            display_name: 'iPhone',
            created_at: '2025-08-01T10:00:00Z',
            last_used_at: null,
            backup_eligible: false,
            backup_state: false,
          },
        ],
      }),
    }
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(screen.getByText('MacBook Pro')).toBeInTheDocument()
    })
    expect(screen.getByText('iPhone')).toBeInTheDocument()
    expect(screen.getByText('Synced')).toBeInTheDocument()
  })

  it('shows "Unnamed passkey" for credentials without display_name', async () => {
    mockFetchResponse = {
      ok: true,
      json: async () => ({
        success: true,
        credentials: [
          {
            id: 1,
            display_name: '',
            created_at: '2025-06-15T10:00:00Z',
            last_used_at: null,
            backup_eligible: false,
            backup_state: false,
          },
        ],
      }),
    }
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(screen.getByText('Unnamed passkey')).toBeInTheDocument()
    })
  })

  it('shows error when fetch fails', async () => {
    mockFetchResponse = {
      ok: true,
      json: async () => ({ success: false, message: 'Unauthorized' }),
    }
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(screen.getByText('Unauthorized')).toBeInTheDocument()
    })
  })

  it('shows generic error when fetch throws', async () => {
    vi.spyOn(global, 'fetch').mockRejectedValue(new Error('Network error'))
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(screen.getByText('Failed to load passkeys')).toBeInTheDocument()
    })
  })

  it('renders delete button for each credential', async () => {
    mockFetchResponse = {
      ok: true,
      json: async () => ({
        success: true,
        credentials: [
          {
            id: 1,
            display_name: 'MacBook Pro',
            created_at: '2025-06-15T10:00:00Z',
            last_used_at: null,
            backup_eligible: false,
            backup_state: false,
          },
        ],
      }),
    }
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(screen.getByText('MacBook Pro')).toBeInTheDocument()
    })

    // Delete button should exist (ghost variant with trash icon)
    const deleteButtons = screen.getAllByRole('button')
    const deleteBtn = deleteButtons.find(
      btn => !btn.textContent?.includes('Add')
    )
    expect(deleteBtn).toBeDefined()
  })

  it('renders the passkey register button', async () => {
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(screen.getByTestId('register-passkey-btn')).toBeInTheDocument()
    })
  })
})
