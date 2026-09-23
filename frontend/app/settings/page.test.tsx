import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import SettingsPage from './page'

// redirect is mocked non-throwing so the gated render can still be asserted on.
const mockRedirect = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn(), replace: vi.fn() }),
  usePathname: () => '/settings',
  redirect: (url: string) => mockRedirect(url),
}))

let mockAuthState: {
  authStatus: 'pending' | 'anonymous' | 'authenticated'
  user: object | null
  isLoading?: boolean
}
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return { useAuthContext: () => deriveMockAuthSignals(mockAuthState) }
})

// The hub itself is covered by its own suite; here only the gate is under
// test, so the page body is a marker.
vi.mock('@/features/settings/components/SettingsHub', () => ({
  SettingsHub: () => <div data-testid="settings-hub" />,
}))

describe('/settings', () => {
  beforeEach(() => {
    mockRedirect.mockClear()
    mockAuthState = { authStatus: 'authenticated', user: { id: 1 } }
  })

  it('renders the hub for a signed-in viewer', () => {
    renderWithProviders(<SettingsPage />)
    expect(screen.getByTestId('settings-hub')).toBeInTheDocument()
    expect(mockRedirect).not.toHaveBeenCalled()
  })

  it('shows a loading state and does not redirect while auth is unsettled', () => {
    mockAuthState = { authStatus: 'pending', user: null, isLoading: false }
    renderWithProviders(<SettingsPage />)
    expect(screen.queryByTestId('settings-hub')).not.toBeInTheDocument()
    expect(mockRedirect).not.toHaveBeenCalled()
  })

  it('redirects a settled-anonymous visitor to /auth with a returnTo', () => {
    mockAuthState = { authStatus: 'anonymous', user: null }
    renderWithProviders(<SettingsPage />)
    expect(mockRedirect).toHaveBeenCalledWith('/auth?returnTo=%2Fsettings')
  })
})
