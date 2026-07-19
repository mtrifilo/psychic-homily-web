import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders, screen, waitFor, within } from '@/test/utils'
import userEvent from '@testing-library/user-event'

const mockReplace = vi.fn()
const mockPush = vi.fn()
const mockRedirect = vi.fn()
const mockUseAuthContext = vi.fn()
const mockUseUpdateProfile = vi.fn()
const mockMutateAsync = vi.fn()

// The page reads `searchParams.get('tab')` once per render, so reassigning
// this before a render is enough to drive which tab is active.
let mockSearchParams = new URLSearchParams()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: mockReplace, push: mockPush }),
  useSearchParams: () => mockSearchParams,
  // `redirect()` halts rendering in production by throwing; in tests we mock it
  // to a no-op spy so the unauthenticated path can be asserted without throwing.
  redirect: (path: string) => mockRedirect(path),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

vi.mock('@/features/auth', () => ({
  useUpdateProfile: () => mockUseUpdateProfile(),
  SettingsPanel: () => <div data-testid="settings-panel">Settings panel</div>,
}))

// The non-Profile tabs render panels that fetch their own data. They are
// out of scope for this surface, so we stub them to sentinels and keep the
// test focused on the ProfileTab form, tab switching, and the auth redirect.
vi.mock('@/features/profile', () => ({
  ContributorProfilePreview: () => <div data-testid="contributor-preview" />,
  TierAdvancementCard: ({ tier }: { tier: string }) => (
    <div data-testid="tier-advancement">{tier}</div>
  ),
  PrivacySettingsPanel: () => <div data-testid="privacy-panel">Privacy panel</div>,
  ProfileSectionsEditor: () => <div data-testid="sections-panel">Sections panel</div>,
}))

import ProfilePage from './page'

const AUTHED_USER = {
  id: '1',
  email: 'alice@example.com',
  username: 'alice',
  display_name: 'Alice Anderson',
  bio: 'I like loud guitars.',
  email_verified: true,
  user_tier: 'trusted',
}

function setAuthenticated(overrides: Record<string, unknown> = {}) {
  mockUseAuthContext.mockReturnValue({
    isAuthenticated: true,
    isLoading: false,
    user: AUTHED_USER,
    ...overrides,
  })
}

function setUpdateProfile(overrides: Record<string, unknown> = {}) {
  mockUseUpdateProfile.mockReturnValue({
    mutateAsync: mockMutateAsync,
    isPending: false,
    ...overrides,
  })
}

describe('ProfilePage (PSY-683)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams = new URLSearchParams()
    window.location.hash = ''
    mockMutateAsync.mockResolvedValue({ success: true, message: 'ok' })
    setAuthenticated()
    setUpdateProfile()
  })

  describe('tabs', () => {
    it('renders all four tab triggers with Profile active by default', () => {
      renderWithProviders(<ProfilePage />)

      const tablist = screen.getByRole('tablist')
      expect(within(tablist).getByRole('tab', { name: /profile/i })).toBeTruthy()
      expect(within(tablist).getByRole('tab', { name: /privacy/i })).toBeTruthy()
      expect(within(tablist).getByRole('tab', { name: /sections/i })).toBeTruthy()
      expect(within(tablist).getByRole('tab', { name: /settings/i })).toBeTruthy()

      // Default tab is "profile" — its form (and not the privacy panel) renders.
      // The "Edit Profile" CardTitle is a styled <div>, not a heading element.
      expect(screen.getByText('Edit Profile')).toBeTruthy()
      expect(screen.queryByTestId('privacy-panel')).toBeNull()
    })

    it('opens the privacy tab when ?tab=privacy is in the URL', () => {
      mockSearchParams = new URLSearchParams('tab=privacy')

      renderWithProviders(<ProfilePage />)

      expect(screen.getByTestId('privacy-panel')).toBeTruthy()
      // Profile form content is not mounted while privacy is the active tab.
      expect(screen.queryByText('Edit Profile')).toBeNull()
    })

    it('replaces the URL with ?tab=privacy when the privacy tab is clicked', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfilePage />)

      await user.click(screen.getByRole('tab', { name: /privacy/i }))

      expect(mockReplace).toHaveBeenCalledWith('/profile?tab=privacy', {
        scroll: false,
      })
    })

    it('replaces the URL with bare /profile when returning to the profile tab', async () => {
      mockSearchParams = new URLSearchParams('tab=settings')
      const user = userEvent.setup()
      renderWithProviders(<ProfilePage />)

      await user.click(screen.getByRole('tab', { name: /^profile$/i }))

      expect(mockReplace).toHaveBeenCalledWith('/profile', { scroll: false })
    })
  })

  describe('profile form', () => {
    it('populates the form fields from useAuthContext().user', () => {
      renderWithProviders(<ProfilePage />)

      expect((screen.getByLabelText(/username/i) as HTMLInputElement).value).toBe(
        'alice'
      )
      expect(
        (screen.getByLabelText(/display name/i) as HTMLInputElement).value
      ).toBe('Alice Anderson')
      expect((screen.getByLabelText(/^bio$/i) as HTMLTextAreaElement).value).toBe(
        'I like loud guitars.'
      )
      // Read-only account email is surfaced too.
      expect(screen.getByText('alice@example.com')).toBeTruthy()
    })

    it('disables Save Changes initially (no changes) and enables it after an edit', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfilePage />)

      const save = screen.getByRole('button', { name: /save changes/i })
      expect(save).toBeDisabled()

      await user.type(screen.getByLabelText(/^bio$/i), '!')

      expect(save).toBeEnabled()
    })

    it('keeps Save Changes disabled while the mutation is pending', () => {
      setUpdateProfile({ isPending: true })
      renderWithProviders(<ProfilePage />)

      // Pending state shows a spinner label and stays disabled even if dirty.
      const save = screen.getByRole('button', { name: /saving/i })
      expect(save).toBeDisabled()
    })

    it('saves trimmed field values and shows "Profile updated", then auto-dismisses', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfilePage />)

      // Edit the bio with surrounding whitespace to assert it is trimmed.
      const bio = screen.getByLabelText(/^bio$/i)
      await user.clear(bio)
      await user.type(bio, '  new bio  ')

      await user.click(screen.getByRole('button', { name: /save changes/i }))

      await waitFor(() => {
        expect(mockMutateAsync).toHaveBeenCalledWith({
          username: 'alice',
          display_name: 'Alice Anderson',
          bio: 'new bio',
        })
      })

      expect(await screen.findByText(/profile updated/i)).toBeTruthy()

      // Banner auto-dismisses after the real 3s timeout (real timers + a
      // slightly longer waitFor window, matching FollowButton's pattern).
      await waitFor(
        () => {
          expect(screen.queryByText(/profile updated/i)).toBeNull()
        },
        { timeout: 4000 }
      )
    })

    it('surfaces the error message inline when the save fails', async () => {
      mockMutateAsync.mockRejectedValueOnce(new Error('Username already taken'))
      const user = userEvent.setup()
      renderWithProviders(<ProfilePage />)

      await user.type(screen.getByLabelText(/^bio$/i), ' more')
      await user.click(screen.getByRole('button', { name: /save changes/i }))

      expect(await screen.findByText('Username already taken')).toBeTruthy()
      expect(screen.queryByText(/profile updated/i)).toBeNull()
    })

    it('falls back to a generic error message for non-Error rejections', async () => {
      mockMutateAsync.mockRejectedValueOnce('boom')
      const user = userEvent.setup()
      renderWithProviders(<ProfilePage />)

      await user.type(screen.getByLabelText(/^bio$/i), ' more')
      await user.click(screen.getByRole('button', { name: /save changes/i }))

      expect(await screen.findByText(/failed to update profile/i)).toBeTruthy()
    })

    it('sends undefined for an emptied username so it is not persisted as blank', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfilePage />)

      await user.clear(screen.getByLabelText(/username/i))
      await user.click(screen.getByRole('button', { name: /save changes/i }))

      await waitFor(() => {
        expect(mockMutateAsync).toHaveBeenCalledWith(
          expect.objectContaining({ username: undefined })
        )
      })
    })

    it('redirects to /users/{username} after a first-time username claim', async () => {
      setAuthenticated({
        user: { ...AUTHED_USER, username: null, display_name: '', bio: '' },
      })
      mockMutateAsync.mockResolvedValueOnce({
        success: true,
        message: 'ok',
        user: { username: 'newhandle' },
      })
      const user = userEvent.setup()
      renderWithProviders(<ProfilePage />)

      await user.type(screen.getByLabelText(/username/i), 'newhandle')
      await user.click(screen.getByRole('button', { name: /save changes/i }))

      await waitFor(() => {
        expect(mockMutateAsync).toHaveBeenCalled()
      })
      expect(mockPush).toHaveBeenCalledWith('/users/newhandle')
      expect(screen.queryByText(/profile updated/i)).toBeNull()
    })

    it('falls back to the form username when the mutation response omits user', async () => {
      setAuthenticated({
        user: { ...AUTHED_USER, username: '', display_name: '', bio: '' },
      })
      mockMutateAsync.mockResolvedValueOnce({ success: true, message: 'ok' })
      const user = userEvent.setup()
      renderWithProviders(<ProfilePage />)

      await user.type(screen.getByLabelText(/username/i), 'fallbackhandle')
      await user.click(screen.getByRole('button', { name: /save changes/i }))

      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith('/users/fallbackhandle')
      })
    })

    it('does not redirect when editing an already-set username', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfilePage />)

      const usernameInput = screen.getByLabelText(/username/i)
      await user.clear(usernameInput)
      await user.type(usernameInput, 'alice2')
      await user.click(screen.getByRole('button', { name: /save changes/i }))

      await waitFor(() => {
        expect(mockMutateAsync).toHaveBeenCalled()
      })
      expect(mockPush).not.toHaveBeenCalled()
      expect(await screen.findByText(/profile updated/i)).toBeTruthy()
    })

    it('focuses the Username field when the URL hash is #username', async () => {
      window.location.hash = '#username'
      renderWithProviders(<ProfilePage />)

      await waitFor(() => {
        expect(document.activeElement).toBe(screen.getByLabelText(/username/i))
      })
    })

    it('focuses the Bio field when the URL hash is #bio', async () => {
      window.location.hash = '#bio'
      renderWithProviders(<ProfilePage />)

      await waitFor(() => {
        expect(document.activeElement).toBe(screen.getByLabelText(/^bio$/i))
      })
    })
  })

  describe('authentication gate', () => {
    it('redirects unauthenticated users to /auth', () => {
      setAuthenticated({ isAuthenticated: false, user: null })

      renderWithProviders(<ProfilePage />)

      expect(mockRedirect).toHaveBeenCalledWith('/auth')
    })

    it('shows a loading spinner (no redirect) while auth state is resolving', () => {
      setAuthenticated({ isAuthenticated: false, isLoading: true, user: null })

      renderWithProviders(<ProfilePage />)

      expect(mockRedirect).not.toHaveBeenCalled()
      // Tabs are not rendered yet during the loading state.
      expect(screen.queryByRole('tablist')).toBeNull()
    })
  })
})
