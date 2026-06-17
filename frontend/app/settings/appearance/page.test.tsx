import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import AppearanceSettingsPage from './page'

// next/navigation: the page reads useRouter().refresh() to re-render the server
// shell after a save, and redirect() for the auth gate. redirect is mocked
// non-throwing so a gated render can still be asserted on.
const mockRefresh = vi.fn()
const mockRedirect = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ refresh: mockRefresh }),
  redirect: (url: string) => mockRedirect(url),
}))

// AuthContext: mutable so each test sets the auth/loading state and the saved
// nav_mode the switch seeds from.
let mockAuthState: {
  isAuthenticated: boolean
  isLoading: boolean
  user: { nav_mode?: string } | null
}
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthState,
}))

const mockMutateAsync = vi.fn()
let mockIsPending = false
vi.mock('@/features/auth', () => ({
  useUpdateProfile: () => ({
    mutateAsync: mockMutateAsync,
    isPending: mockIsPending,
  }),
}))

describe('AppearanceSettingsPage', () => {
  beforeEach(() => {
    mockRefresh.mockClear()
    mockRedirect.mockClear()
    mockMutateAsync.mockReset()
    mockMutateAsync.mockResolvedValue({ success: true })
    mockIsPending = false
    mockAuthState = {
      isAuthenticated: true,
      isLoading: false,
      user: { nav_mode: 'top' },
    }
    // Clear the nav_mode cookie between tests (jsdom persists document.cookie).
    document.cookie = 'nav_mode=; path=/; max-age=0'
  })

  it('shows a loading state (no toggle) while auth resolves', () => {
    mockAuthState = { isAuthenticated: false, isLoading: true, user: null }
    renderWithProviders(<AppearanceSettingsPage />)
    expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  })

  it('redirects unauthenticated users to /auth', () => {
    mockAuthState = { isAuthenticated: false, isLoading: false, user: null }
    renderWithProviders(<AppearanceSettingsPage />)
    expect(mockRedirect).toHaveBeenCalledWith('/auth')
  })

  it('seeds the switch from the saved account preference (side → checked)', () => {
    mockAuthState = {
      isAuthenticated: true,
      isLoading: false,
      user: { nav_mode: 'side' },
    }
    renderWithProviders(<AppearanceSettingsPage />)
    expect(screen.getByRole('switch')).toBeChecked()
  })

  it('seeds the switch from the saved account preference (top → unchecked)', () => {
    renderWithProviders(<AppearanceSettingsPage />)
    expect(screen.getByRole('switch')).not.toBeChecked()
  })

  it('toggling on persists nav_mode=side, writes the cookie, and refreshes the shell', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AppearanceSettingsPage />)

    await user.click(screen.getByRole('switch'))

    expect(mockMutateAsync).toHaveBeenCalledWith({ nav_mode: 'side' })
    await waitFor(() => expect(mockRefresh).toHaveBeenCalledTimes(1))
    expect(document.cookie).toContain('nav_mode=side')
    expect(await screen.findByText('Saved')).toBeInTheDocument()
    expect(screen.getByRole('switch')).toBeChecked()
  })

  it('reverts the optimistic switch, writes no cookie, and surfaces an error when the save fails', async () => {
    mockMutateAsync.mockRejectedValue(new Error('Something went wrong'))
    const user = userEvent.setup()
    renderWithProviders(<AppearanceSettingsPage />)

    const toggle = screen.getByRole('switch')
    await user.click(toggle)

    await waitFor(() => expect(toggle).not.toBeChecked())
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    expect(mockRefresh).not.toHaveBeenCalled()
    // The cookie is written only on a successful save, so a failed save leaves
    // no 'side' cookie behind to flip the shell on next reload.
    expect(document.cookie).not.toContain('nav_mode=side')
  })

  it('follows the saved account preference when it changes (cross-device / refetch re-seed)', () => {
    const { rerender } = renderWithProviders(<AppearanceSettingsPage />)
    expect(screen.getByRole('switch')).not.toBeChecked() // account 'top'

    // The saved preference changes underneath (another device, or a profile
    // refetch) — the control must follow account-as-source-of-truth.
    mockAuthState = {
      ...mockAuthState,
      user: { nav_mode: 'side' },
    }
    rerender(<AppearanceSettingsPage />)

    expect(screen.getByRole('switch')).toBeChecked()
  })

  it('keeps the in-flight optimistic choice when an unrelated profile refetch arrives', async () => {
    // Hold the save open so the optimistic override is still active.
    let resolveSave: (value: unknown) => void = () => {}
    mockMutateAsync.mockReturnValue(
      new Promise((resolve) => {
        resolveSave = resolve
      })
    )
    const user = userEvent.setup()
    const { rerender } = renderWithProviders(<AppearanceSettingsPage />)

    await user.click(screen.getByRole('switch'))
    expect(screen.getByRole('switch')).toBeChecked() // optimistic 'side'

    // An unrelated profile refetch lands (same nav_mode, new object reference).
    // Because the control derives from `optimistic ?? accountMode` (value-keyed,
    // not reference-keyed), it must NOT snap back to 'top' mid-save.
    mockAuthState = {
      ...mockAuthState,
      user: { nav_mode: 'top' },
    }
    rerender(<AppearanceSettingsPage />)
    expect(screen.getByRole('switch')).toBeChecked()

    // Let the save resolve so its post-success work flushes inside the test.
    resolveSave({ success: true })
    await waitFor(() => expect(mockRefresh).toHaveBeenCalledTimes(1))
  })
})
