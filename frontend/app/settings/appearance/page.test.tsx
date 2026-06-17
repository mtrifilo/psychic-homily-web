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

  it('reverts the optimistic switch + cookie and surfaces an error when the save fails', async () => {
    mockMutateAsync.mockRejectedValue(new Error('Something went wrong'))
    const user = userEvent.setup()
    renderWithProviders(<AppearanceSettingsPage />)

    const toggle = screen.getByRole('switch')
    await user.click(toggle)

    await waitFor(() => expect(toggle).not.toBeChecked())
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    expect(mockRefresh).not.toHaveBeenCalled()
  })
})
