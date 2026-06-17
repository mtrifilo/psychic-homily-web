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

  it('deletes a credential when the user confirms the prompt and removes it from the list', async () => {
    // Use a controlled fetch that returns the initial list, then 200 on DELETE.
    const fetchMock = vi.fn()
      // initial list fetch
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          success: true,
          credentials: [
            {
              id: 7,
              display_name: 'YubiKey',
              created_at: '2025-06-15T10:00:00Z',
              last_used_at: null,
              backup_eligible: false,
              backup_state: false,
            },
          ],
        }),
      } as Response)
      // DELETE response
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ success: true }),
      } as Response)

    vi.spyOn(global, 'fetch').mockImplementation(fetchMock)
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)

    const user = userEvent.setup()
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(screen.getByText('YubiKey')).toBeInTheDocument()
    })

    // Click the destructive ghost icon button on the row.
    const deleteBtn = screen
      .getAllByRole('button')
      .find(btn => btn.className.includes('text-destructive'))
    expect(deleteBtn).toBeDefined()
    await user.click(deleteBtn!)

    // Verify the confirm dialog was shown + the DELETE was issued.
    expect(confirmSpy).toHaveBeenCalledWith(
      'Are you sure you want to remove this passkey?'
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining('/auth/passkey/credentials/7'),
        expect.objectContaining({ method: 'DELETE' })
      )
    })

    // Row removed from UI after success.
    await waitFor(() => {
      expect(screen.queryByText('YubiKey')).not.toBeInTheDocument()
    })

    confirmSpy.mockRestore()
  })

  it('does NOT call DELETE when the user cancels the confirm prompt', async () => {
    const initialListResponse = {
      ok: true,
      json: async () => ({
        success: true,
        credentials: [
          {
            id: 99,
            display_name: 'Phone passkey',
            created_at: '2025-06-15T10:00:00Z',
            last_used_at: null,
            backup_eligible: false,
            backup_state: false,
          },
        ],
      }),
    } as Response

    const fetchMock = vi.fn().mockResolvedValue(initialListResponse)
    vi.spyOn(global, 'fetch').mockImplementation(fetchMock)
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)

    const user = userEvent.setup()
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(screen.getByText('Phone passkey')).toBeInTheDocument()
    })

    const deleteBtn = screen
      .getAllByRole('button')
      .find(btn => btn.className.includes('text-destructive'))
    await user.click(deleteBtn!)

    expect(confirmSpy).toHaveBeenCalled()
    // The credential is still visible — only the initial list fetch ran.
    expect(screen.getByText('Phone passkey')).toBeInTheDocument()
    // Only the initial list fetch happened (no DELETE call).
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).not.toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ method: 'DELETE' })
    )

    confirmSpy.mockRestore()
  })

  it('shows the server message when DELETE returns success=false', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          success: true,
          credentials: [
            {
              id: 5,
              display_name: 'Old key',
              created_at: '2025-06-15T10:00:00Z',
              last_used_at: null,
              backup_eligible: false,
              backup_state: false,
            },
          ],
        }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          success: false,
          message: 'Cannot delete last passkey',
        }),
      } as Response)

    vi.spyOn(global, 'fetch').mockImplementation(fetchMock)
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)

    const user = userEvent.setup()
    renderWithProviders(<PasskeyManagement />)

    await waitFor(() => {
      expect(screen.getByText('Old key')).toBeInTheDocument()
    })

    const deleteBtn = screen
      .getAllByRole('button')
      .find(btn => btn.className.includes('text-destructive'))
    await user.click(deleteBtn!)

    await waitFor(() => {
      expect(
        screen.getByText('Cannot delete last passkey')
      ).toBeInTheDocument()
    })
    // Credential should remain in the list since DELETE failed server-side.
    expect(screen.getByText('Old key')).toBeInTheDocument()

    confirmSpy.mockRestore()
  })

  it('refetches credentials after PasskeyRegisterButton fires onSuccess', async () => {
    // Initial list: empty. After re-fetch: one credential.
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ success: true, credentials: [] }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          success: true,
          credentials: [
            {
              id: 1,
              display_name: 'Newly registered',
              created_at: '2025-06-15T10:00:00Z',
              last_used_at: null,
              backup_eligible: false,
              backup_state: false,
            },
          ],
        }),
      } as Response)

    vi.spyOn(global, 'fetch').mockImplementation(fetchMock)

    const user = userEvent.setup()
    renderWithProviders(<PasskeyManagement />)

    // Empty state visible.
    await waitFor(() => {
      expect(
        screen.getByText(/No passkeys registered yet/)
      ).toBeInTheDocument()
    })

    // The mock PasskeyRegisterButton fires onSuccess on click → triggers refetch.
    await user.click(screen.getByTestId('register-passkey-btn'))

    await waitFor(() => {
      expect(screen.getByText('Newly registered')).toBeInTheDocument()
    })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
