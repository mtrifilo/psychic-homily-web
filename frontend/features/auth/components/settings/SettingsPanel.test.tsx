import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { SettingsPanel } from './SettingsPanel'

// --- Mocks ---

let mockUser: {
  email: string
  email_verified: boolean
  is_admin?: boolean
} | null = null

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    user: mockUser,
  }),
}))

const mockSendVerificationMutateAsync = vi.fn()
let mockSendVerificationState = {
  isPending: false,
  isError: false,
  isSuccess: false,
  error: null as Error | null,
}

const mockExportMutateAsync = vi.fn()
let mockExportState = {
  isPending: false,
  isError: false,
  isSuccess: false,
  error: null as Error | null,
}

const mockGenerateCLITokenMutateAsync = vi.fn()
let mockGenerateCLITokenState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

vi.mock('@/features/auth', () => ({
  useSendVerificationEmail: () => ({
    mutateAsync: mockSendVerificationMutateAsync,
    ...mockSendVerificationState,
  }),
  useExportData: () => ({
    mutateAsync: mockExportMutateAsync,
    ...mockExportState,
  }),
  useGenerateCLIToken: () => ({
    mutateAsync: mockGenerateCLITokenMutateAsync,
    ...mockGenerateCLITokenState,
  }),
  useProfile: () => ({ data: null as unknown }),
}))

vi.mock('./change-password', () => ({
  ChangePassword: () => <div data-testid="change-password">ChangePassword</div>,
}))

vi.mock('./delete-account-dialog', () => ({
  DeleteAccountDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="delete-dialog">DeleteAccountDialog</div> : null,
}))

vi.mock('./oauth-accounts', () => ({
  OAuthAccounts: () => <div data-testid="oauth-accounts">OAuthAccounts</div>,
}))

vi.mock('./passkey-management', () => ({
  PasskeyManagement: () => (
    <div data-testid="passkey-management">PasskeyManagement</div>
  ),
}))

vi.mock('./api-token-management', () => ({
  APITokenManagement: () => (
    <div data-testid="api-token-management">APITokenManagement</div>
  ),
}))

vi.mock('./favorite-cities', () => ({
  FavoriteCitiesSettings: () => (
    <div data-testid="favorite-cities">FavoriteCitiesSettings</div>
  ),
}))

vi.mock('./notification-settings', () => ({
  NotificationSettings: () => (
    <div data-testid="notification-settings">NotificationSettings</div>
  ),
}))

vi.mock('./reply-permission-settings', () => ({
  ReplyPermissionSettings: () => (
    <div data-testid="reply-permission-settings">ReplyPermissionSettings</div>
  ),
}))

vi.mock('@/features/collections', () => ({
  CalendarFeedSection: ({ variant }: { variant?: string }) => (
    <div data-testid="calendar-feed-section" data-variant={variant}>
      CalendarFeedSection
    </div>
  ),
}))

// --- Tests ---

describe('SettingsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUser = {
      email: 'user@example.com',
      email_verified: false,
    }
    mockSendVerificationState = {
      isPending: false,
      isError: false,
      isSuccess: false,
      error: null,
    }
    mockExportState = {
      isPending: false,
      isError: false,
      isSuccess: false,
      error: null,
    }
    mockGenerateCLITokenState = {
      isPending: false,
      isError: false,
      error: null,
    }
    mockSendVerificationMutateAsync.mockReset()
    mockExportMutateAsync.mockReset()
    mockGenerateCLITokenMutateAsync.mockReset()
  })

  it('renders FavoriteCitiesSettings component', () => {
    renderWithProviders(<SettingsPanel />)
    expect(screen.getByTestId('favorite-cities')).toBeInTheDocument()
  })

  it('renders NotificationSettings component', () => {
    renderWithProviders(<SettingsPanel />)
    expect(screen.getByTestId('notification-settings')).toBeInTheDocument()
  })

  it('renders ReplyPermissionSettings component', () => {
    renderWithProviders(<SettingsPanel />)
    expect(screen.getByTestId('reply-permission-settings')).toBeInTheDocument()
  })

  it('renders CalendarFeedSection with settings variant', () => {
    renderWithProviders(<SettingsPanel />)
    const feed = screen.getByTestId('calendar-feed-section')
    expect(feed).toBeInTheDocument()
    expect(feed).toHaveAttribute('data-variant', 'settings')
  })

  it('renders OAuthAccounts component', () => {
    renderWithProviders(<SettingsPanel />)
    expect(screen.getByTestId('oauth-accounts')).toBeInTheDocument()
  })

  it('renders PasskeyManagement component', () => {
    renderWithProviders(<SettingsPanel />)
    expect(screen.getByTestId('passkey-management')).toBeInTheDocument()
  })

  it('renders ChangePassword component', () => {
    renderWithProviders(<SettingsPanel />)
    expect(screen.getByTestId('change-password')).toBeInTheDocument()
  })

  // --- Account (email + verification fold) ---

  it('renders Account card with board J copy', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Account')).toBeInTheDocument()
    expect(
      screen.getByText('Your sign-in email. Verification unlocks contributions.')
    ).toBeInTheDocument()
  })

  it('shows user email', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('user@example.com')).toBeInTheDocument()
  })

  it('shows "Not verified" badge when email is not verified', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Not verified')).toBeInTheDocument()
  })

  it('shows "Verified" badge when email is verified', () => {
    mockUser = { email: 'user@example.com', email_verified: true }
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Verified')).toBeInTheDocument()
  })

  it('shows "Verified" badge when user is admin (even without email_verified)', () => {
    mockUser = {
      email: 'admin@example.com',
      email_verified: false,
      is_admin: true,
    }
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Verified')).toBeInTheDocument()
  })

  it('shows Resend verification button for unverified users', () => {
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.getByRole('button', { name: /Resend verification/ })
    ).toBeInTheDocument()
  })

  it('does not show Resend verification button for verified users', () => {
    mockUser = { email: 'user@example.com', email_verified: true }
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.queryByRole('button', { name: /Resend verification/ })
    ).not.toBeInTheDocument()
  })

  it('shows admin notice for admin users', () => {
    mockUser = {
      email: 'admin@example.com',
      email_verified: false,
      is_admin: true,
    }
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.getByText(/Admin accounts can contribute without email verification/)
    ).toBeInTheDocument()
  })

  it('shows verification error when send fails', () => {
    mockSendVerificationState = {
      isPending: false,
      isError: true,
      isSuccess: false,
      error: new Error('Too many requests'),
    }
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Too many requests')).toBeInTheDocument()
  })

  // --- Data Export Section ---

  it('renders data export section with board J copy', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Export your data')).toBeInTheDocument()
    expect(
      screen.getByText(
        /Download everything tied to your account — profile, contributions, collections, saved shows — as JSON\./
      )
    ).toBeInTheDocument()
  })

  it('shows Export JSON button', () => {
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.getByRole('button', { name: /Export JSON/ })
    ).toBeInTheDocument()
  })

  it('shows export error when export fails', () => {
    mockExportState = {
      isPending: false,
      isError: true,
      isSuccess: false,
      error: new Error('Export failed'),
    }
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Export failed')).toBeInTheDocument()
  })

  it('shows export success message', () => {
    mockExportState = {
      isPending: false,
      isError: false,
      isSuccess: true,
      error: null,
    }
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.getByText(
        'Data exported successfully! Check your downloads folder.'
      )
    ).toBeInTheDocument()
  })

  // --- Admin-Only Sections ---

  it('does not show API Token Management for non-admin users', () => {
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.queryByTestId('api-token-management')
    ).not.toBeInTheDocument()
  })

  it('shows API Token Management for admin users', () => {
    mockUser = {
      email: 'admin@example.com',
      email_verified: true,
      is_admin: true,
    }
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByTestId('api-token-management')).toBeInTheDocument()
  })

  it('does not show CLI authentication for non-admin users', () => {
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.queryByText('CLI authentication')
    ).not.toBeInTheDocument()
  })

  it('shows CLI authentication section for admin users', () => {
    mockUser = {
      email: 'admin@example.com',
      email_verified: true,
      is_admin: true,
    }
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('CLI authentication')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Generate a short-lived token for the ph command-line tool.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Generate CLI token/ })
    ).toBeInTheDocument()
  })

  // --- Danger Zone ---

  it('renders danger zone section', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Danger zone')).toBeInTheDocument()
    expect(
      screen.getByText(/Deleting your account removes your profile and sign-in/)
    ).toBeInTheDocument()
  })

  it('shows Delete account button', () => {
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.getByRole('button', { name: /Delete account/ })
    ).toBeInTheDocument()
  })

  it('opens delete account dialog when Delete account is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SettingsPanel />)

    expect(screen.queryByTestId('delete-dialog')).not.toBeInTheDocument()

    await user.click(
      screen.getByRole('button', { name: /Delete account/ })
    )

    expect(screen.getByTestId('delete-dialog')).toBeInTheDocument()
  })

  // --- CLI Token Section ---

  it('generates CLI token when button is clicked', async () => {
    mockUser = {
      email: 'admin@example.com',
      email_verified: true,
      is_admin: true,
    }
    mockGenerateCLITokenMutateAsync.mockResolvedValue({
      token: 'test-token-abc123',
    })
    const user = userEvent.setup()
    renderWithProviders(<SettingsPanel />)

    await user.click(
      screen.getByRole('button', { name: /Generate CLI token/ })
    )

    expect(mockGenerateCLITokenMutateAsync).toHaveBeenCalled()
  })

  it('shows CLI token error when generation fails', () => {
    mockUser = {
      email: 'admin@example.com',
      email_verified: true,
      is_admin: true,
    }
    mockGenerateCLITokenState = {
      isPending: false,
      isError: true,
      error: new Error('Token generation failed'),
    }
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Token generation failed')).toBeInTheDocument()
  })

  it('shows "Verification email sent!" success UI after Resend verification is clicked', async () => {
    mockSendVerificationMutateAsync.mockResolvedValueOnce(undefined)
    mockSendVerificationState = {
      isPending: false,
      isError: false,
      isSuccess: true,
      error: null,
    }
    const user = userEvent.setup()
    renderWithProviders(<SettingsPanel />)

    await user.click(
      screen.getByRole('button', { name: /Resend verification/ })
    )

    expect(mockSendVerificationMutateAsync).toHaveBeenCalled()
    await waitFor(() => {
      expect(
        screen.getByText('Verification email sent! Check your inbox.')
      ).toBeInTheDocument()
    })
    expect(
      screen.queryByRole('button', { name: /Resend verification/ })
    ).not.toBeInTheDocument()
  })

  it('triggers data-export download flow when Export JSON is clicked', async () => {
    const exportPayload = { profile: { email: 'user@example.com' } }
    mockExportMutateAsync.mockResolvedValueOnce(exportPayload)

    const createObjectURL = vi.fn().mockReturnValue('blob:fake-url')
    const revokeObjectURL = vi.fn()
    const originalCreateObjectURL = URL.createObjectURL
    const originalRevokeObjectURL = URL.revokeObjectURL
    URL.createObjectURL = createObjectURL
    URL.revokeObjectURL = revokeObjectURL

    try {
      const user = userEvent.setup()
      renderWithProviders(<SettingsPanel />)

      await user.click(screen.getByRole('button', { name: /Export JSON/ }))

      expect(mockExportMutateAsync).toHaveBeenCalled()
      await waitFor(() => {
        expect(createObjectURL).toHaveBeenCalled()
        expect(revokeObjectURL).toHaveBeenCalledWith('blob:fake-url')
      })
    } finally {
      URL.createObjectURL = originalCreateObjectURL
      URL.revokeObjectURL = originalRevokeObjectURL
    }
  })

  it('renders the issued CLI token and a Copy button after generation succeeds', async () => {
    mockUser = {
      email: 'admin@example.com',
      email_verified: true,
      is_admin: true,
    }
    mockGenerateCLITokenMutateAsync.mockResolvedValueOnce({
      token: 'cli-token-xyz-789',
    })

    const user = userEvent.setup()
    renderWithProviders(<SettingsPanel />)

    await user.click(
      screen.getByRole('button', { name: /Generate CLI token/ })
    )

    await waitFor(() => {
      expect(screen.getByText('cli-token-xyz-789')).toBeInTheDocument()
    })
    expect(
      screen.getByText(/This token expires in 24 hours/)
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Generate new token/ })
    ).toBeInTheDocument()
  })

  it('copies the issued CLI token to the clipboard when Copy is clicked', async () => {
    mockUser = {
      email: 'admin@example.com',
      email_verified: true,
      is_admin: true,
    }
    mockGenerateCLITokenMutateAsync.mockResolvedValueOnce({
      token: 'cli-copyable-token',
    })

    const user = userEvent.setup()
    const writeTextSpy = vi
      .spyOn(navigator.clipboard, 'writeText')
      .mockResolvedValue(undefined)

    renderWithProviders(<SettingsPanel />)

    await user.click(
      screen.getByRole('button', { name: /Generate CLI token/ })
    )

    await waitFor(() => {
      expect(screen.getByText('cli-copyable-token')).toBeInTheDocument()
    })

    const tokenCodeEl = screen.getByText('cli-copyable-token')
    const copyBtn = tokenCodeEl.parentElement?.querySelector('button')
    expect(copyBtn).toBeTruthy()
    await user.click(copyBtn!)

    await waitFor(() => {
      expect(writeTextSpy).toHaveBeenCalledWith('cli-copyable-token')
    })

    writeTextSpy.mockRestore()
  })

  it('renders board J vertical order: account → … → oauth → passkeys → change password', () => {
    const { container } = renderWithProviders(<SettingsPanel />)

    const texts = [
      'Account',
      'favorite-cities',
      'notification-settings',
      'calendar-feed-section',
      'reply-permission-settings',
      'oauth-accounts',
      'passkey-management',
      'change-password',
    ]

    // Account is a CardTitle text node; the rest are mocked testids.
    const accountIdx = Array.from(container.querySelectorAll('*')).findIndex(
      el => el.textContent === 'Account' && el.children.length === 0
    )
    expect(accountIdx).toBeGreaterThanOrEqual(0)

    const positions = texts.slice(1).map(id =>
      Array.from(container.querySelectorAll('[data-testid]')).findIndex(
        el => el.getAttribute('data-testid') === id
      )
    )
    expect(positions.every(p => p !== -1)).toBe(true)
    for (let i = 1; i < positions.length; i++) {
      expect(positions[i]).toBeGreaterThan(positions[i - 1])
    }
  })
})
