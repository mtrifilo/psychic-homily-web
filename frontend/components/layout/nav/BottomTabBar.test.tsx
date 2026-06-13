import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BottomTabBar } from './BottomTabBar'

let mockPathname = '/'
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname,
}))

const mockLogout = vi.fn()
type MockAuthContextValue = {
  user: { email: string; is_admin: boolean } | null
  isAuthenticated: boolean
  isLoading: boolean
  logout: () => void
}
const mockAuthContext = vi.fn<() => MockAuthContextValue>(() => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,
  logout: mockLogout,
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

let mockTheme = 'dark'
const mockSetTheme = vi.fn()
vi.mock('next-themes', () => ({
  useTheme: () => ({ theme: mockTheme, resolvedTheme: mockTheme, setTheme: mockSetTheme }),
}))

function authedAs(user: { email: string; is_admin: boolean }) {
  mockAuthContext.mockReturnValue({
    user,
    isAuthenticated: true,
    isLoading: false,
    logout: mockLogout,
  })
}

describe('BottomTabBar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname = '/'
    mockTheme = 'dark'
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: mockLogout,
    })
  })

  describe('tabs', () => {
    it('renders the three primary link tabs with their destinations', () => {
      render(<BottomTabBar />)
      expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('href', '/')
      expect(screen.getByRole('link', { name: 'Shows' })).toHaveAttribute('href', '/shows')
      expect(screen.getByRole('link', { name: 'Radio' })).toHaveAttribute('href', '/radio')
    })

    it('renders the Browse tab as a sheet trigger', () => {
      render(<BottomTabBar />)
      expect(screen.getByRole('button', { name: 'Browse' })).toBeInTheDocument()
    })

    it('marks the tab matching the current route with aria-current', () => {
      mockPathname = '/shows'
      render(<BottomTabBar />)
      expect(screen.getByRole('link', { name: 'Shows' })).toHaveAttribute('aria-current', 'page')
      expect(screen.getByRole('link', { name: 'Home' })).not.toHaveAttribute('aria-current')
    })

    it('marks Home active only on the exact root route', () => {
      mockPathname = '/radio/kexp'
      render(<BottomTabBar />)
      expect(screen.getByRole('link', { name: 'Home' })).not.toHaveAttribute('aria-current')
      expect(screen.getByRole('link', { name: 'Radio' })).toHaveAttribute('aria-current', 'page')
    })

    it('lights Browse for a long-tail destination route', () => {
      mockPathname = '/artists/some-artist'
      render(<BottomTabBar />)
      expect(screen.getByRole('button', { name: 'Browse' })).toHaveAttribute('aria-current', 'page')
    })

    it('gives Shows (not Browse) the shared /shows/submit route', () => {
      // /shows/submit is both a Shows descendant and a Browse-sheet destination;
      // primary tabs win so exactly one tab lights up.
      mockPathname = '/shows/submit'
      render(<BottomTabBar />)
      expect(screen.getByRole('link', { name: 'Shows' })).toHaveAttribute('aria-current', 'page')
      expect(screen.getByRole('button', { name: 'Browse' })).not.toHaveAttribute('aria-current')
    })
  })

  describe('Browse sheet', () => {
    it('opens the long-tail sheet with grouped destinations, incl. Explore', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      // Explore only lives here on mobile (desktop carries it in PrimaryNav).
      expect(await screen.findByRole('link', { name: 'Explore' })).toHaveAttribute('href', '/explore')
      expect(screen.getByRole('link', { name: 'Festivals' })).toHaveAttribute('href', '/festivals')
      expect(screen.getByRole('link', { name: /Substack/ })).toHaveAttribute(
        'href',
        'https://psychichomily.substack.com/'
      )
      expect(screen.getByText('Catalog')).toBeInTheDocument()
      expect(screen.getByText('Curation')).toBeInTheDocument()
    })

    it('hides auth-only destinations from anonymous visitors', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      await screen.findByRole('link', { name: 'Explore' })
      expect(screen.queryByRole('link', { name: 'My Submissions' })).not.toBeInTheDocument()
    })

    it('shows auth-only destinations when signed in', async () => {
      authedAs({ email: 'user@test.com', is_admin: false })
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      expect(await screen.findByRole('link', { name: 'My Submissions' })).toHaveAttribute(
        'href',
        '/submissions'
      )
    })

    it('closes when a destination is clicked', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      await user.click(await screen.findByRole('link', { name: 'Festivals' }))
      expect(screen.queryByRole('link', { name: 'Explore' })).not.toBeInTheDocument()
    })

    it('carries the theme toggle (migrated from the retired hamburger sheet)', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      await user.click(await screen.findByRole('button', { name: 'Light mode' }))
      expect(mockSetTheme).toHaveBeenCalledWith('light')
    })
  })

  describe('Account tab', () => {
    it('is a login link when anonymous', () => {
      render(<BottomTabBar />)
      expect(screen.getByRole('link', { name: 'Account' })).toHaveAttribute('href', '/auth')
    })

    it('lights up on /auth when anonymous', () => {
      mockPathname = '/auth'
      render(<BottomTabBar />)
      expect(screen.getByRole('link', { name: 'Account' })).toHaveAttribute('aria-current', 'page')
    })

    it('is inert while auth is hydrating', () => {
      mockAuthContext.mockReturnValue({
        user: null,
        isAuthenticated: false,
        isLoading: true,
        logout: mockLogout,
      })
      render(<BottomTabBar />)
      expect(screen.queryByRole('link', { name: 'Account' })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Account' })).not.toBeInTheDocument()
    })

    it('opens the account sheet with the UserMenu-mirror entries when signed in', async () => {
      authedAs({ email: 'user@test.com', is_admin: false })
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Account' }))
      expect(await screen.findByRole('link', { name: 'Notifications' })).toHaveAttribute(
        'href',
        '/notifications'
      )
      expect(screen.getByRole('link', { name: 'My Library' })).toHaveAttribute('href', '/library')
      expect(screen.getByRole('link', { name: 'Profile' })).toHaveAttribute('href', '/users/me')
      expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('href', '/profile')
      expect(screen.getByText('user@test.com')).toBeInTheDocument()
      expect(screen.queryByRole('link', { name: 'Admin' })).not.toBeInTheDocument()
    })

    it('adds the Admin entry for admins and signs out via the sheet', async () => {
      authedAs({ email: 'admin@test.com', is_admin: true })
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Account' }))
      expect(await screen.findByRole('link', { name: 'Admin' })).toHaveAttribute('href', '/admin')
      await user.click(screen.getByRole('button', { name: 'Sign out' }))
      expect(mockLogout).toHaveBeenCalledTimes(1)
    })

    it('lights up on account routes when signed in', () => {
      authedAs({ email: 'user@test.com', is_admin: false })
      mockPathname = '/library'
      render(<BottomTabBar />)
      expect(screen.getByRole('button', { name: 'Account' })).toHaveAttribute('aria-current', 'page')
    })
  })
})
