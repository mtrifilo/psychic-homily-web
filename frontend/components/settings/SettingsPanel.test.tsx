import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { SettingsPanel } from './SettingsPanel'

// --- Mocks ---

vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
}))

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

vi.mock('@/lib/hooks/useAuth', () => ({
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
  useProfile: () => ({ data: null }),
}))

// Mock sub-components to isolate SettingsPanel tests
vi.mock('@/components/settings/change-password', () => ({
  ChangePassword: () => <div data-testid="change-password">ChangePassword</div>,
}))

vi.mock('@/components/settings/delete-account-dialog', () => ({
  DeleteAccountDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="delete-dialog">DeleteAccountDialog</div> : null,
}))

vi.mock('@/components/settings/oauth-accounts', () => ({
  OAuthAccounts: () => <div data-testid="oauth-accounts">OAuthAccounts</div>,
}))

vi.mock('@/components/settings/api-token-management', () => ({
  APITokenManagement: () => (
    <div data-testid="api-token-management">APITokenManagement</div>
  ),
}))

vi.mock('@/components/settings/favorite-cities', () => ({
  FavoriteCitiesSettings: () => (
    <div data-testid="favorite-cities">FavoriteCitiesSettings</div>
  ),
}))

vi.mock('@/components/settings/notification-settings', () => ({
  NotificationSettings: () => (
    <div data-testid="notification-settings">NotificationSettings</div>
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

  // --- Sub-component rendering ---

  it('renders FavoriteCitiesSettings component', () => {
    renderWithProviders(<SettingsPanel />)
    expect(screen.getByTestId('favorite-cities')).toBeInTheDocument()
  })

  it('renders NotificationSettings component', () => {
    renderWithProviders(<SettingsPanel />)
    expect(screen.getByTestId('notification-settings')).toBeInTheDocument()
  })

  it('renders OAuthAccounts component', () => {
    renderWithProviders(<SettingsPanel />)
    expect(screen.getByTestId('oauth-accounts')).toBeInTheDocument()
  })

  it('renders ChangePassword component', () => {
    renderWithProviders(<SettingsPanel />)
    expect(screen.getByTestId('change-password')).toBeInTheDocument()
  })

  // --- Email Verification Section ---

  it('renders email verification section', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Email Verification')).toBeInTheDocument()
    expect(
      screen.getByText('Verify your email to submit shows to the calendar')
    ).toBeInTheDocument()
  })

  it('shows user email', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('user@example.com')).toBeInTheDocument()
  })

  it('shows "Not Verified" badge when email is not verified', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Not Verified')).toBeInTheDocument()
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

  it('shows Send Verification Email button for unverified users', () => {
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.getByRole('button', { name: /Send Verification Email/ })
    ).toBeInTheDocument()
  })

  it('does not show Send Verification Email button for verified users', () => {
    mockUser = { email: 'user@example.com', email_verified: true }
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.queryByRole('button', { name: /Send Verification Email/ })
    ).not.toBeInTheDocument()
  })

  it('shows "Your email is verified" message for verified non-admin users', () => {
    mockUser = { email: 'user@example.com', email_verified: true }
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Your email is verified')).toBeInTheDocument()
    expect(
      screen.getByText(
        'You can submit shows to the Arizona music calendar.'
      )
    ).toBeInTheDocument()
  })

  it('shows admin notice for admin users', () => {
    mockUser = {
      email: 'admin@example.com',
      email_verified: false,
      is_admin: true,
    }
    renderWithProviders(<SettingsPanel />)

    // "Admin account" appears both in the email status subtitle and the admin notice
    const adminTexts = screen.getAllByText('Admin account')
    expect(adminTexts.length).toBeGreaterThanOrEqual(1)
    expect(
      screen.getByText(
        /As an admin, you can submit shows without email verification/
      )
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

  it('renders data export section', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Export Your Data')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Download a copy of all your data in JSON format'
      )
    ).toBeInTheDocument()
  })

  it('shows export includes list', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Profile information')).toBeInTheDocument()
    expect(screen.getByText('Email preferences')).toBeInTheDocument()
    expect(screen.getByText('Connected accounts')).toBeInTheDocument()
    expect(screen.getByText('Passkeys')).toBeInTheDocument()
    expect(screen.getByText('Saved shows')).toBeInTheDocument()
    expect(screen.getByText('Submitted shows')).toBeInTheDocument()
  })

  it('shows Export My Data button', () => {
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.getByRole('button', { name: /Export My Data/ })
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

  it('does not show CLI Authentication for non-admin users', () => {
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.queryByText('CLI Authentication')
    ).not.toBeInTheDocument()
  })

  it('shows CLI Authentication section for admin users', () => {
    mockUser = {
      email: 'admin@example.com',
      email_verified: true,
      is_admin: true,
    }
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('CLI Authentication')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Generate a short-lived token for the admin CLI tool'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Generate CLI Token/ })
    ).toBeInTheDocument()
  })

  // --- Danger Zone ---

  it('renders danger zone section', () => {
    renderWithProviders(<SettingsPanel />)

    expect(screen.getByText('Danger Zone')).toBeInTheDocument()
    expect(
      screen.getByText('Irreversible actions that affect your account')
    ).toBeInTheDocument()
  })

  it('shows Delete Account button', () => {
    renderWithProviders(<SettingsPanel />)

    expect(
      screen.getByRole('button', { name: /Delete Account/ })
    ).toBeInTheDocument()
  })

  it('opens delete account dialog when Delete Account is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SettingsPanel />)

    expect(screen.queryByTestId('delete-dialog')).not.toBeInTheDocument()

    await user.click(
      screen.getByRole('button', { name: /Delete Account/ })
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
      screen.getByRole('button', { name: /Generate CLI Token/ })
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
})
