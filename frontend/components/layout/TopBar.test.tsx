import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TopBar } from './TopBar'

let mockPathname = '/'
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname,
}))

// The admin drawer is a dynamically-imported chunk (AdminDrawerNav, kept off the
// public bundle); stub next/dynamic so the mobile sheet renders a synchronous
// marker. The drawer's own behavior is covered by AdminDrawerNav.test.tsx.
vi.mock('next/dynamic', () => ({
  default: () =>
    function AdminDrawerNavStub() {
      return <div data-testid="admin-drawer-nav" />
    },
}))

vi.mock('next/image', () => ({
  default: (props: Record<string, unknown>) => {
    const { priority, ...rest } = props
    return <img {...rest} data-priority={priority ? 'true' : undefined} />
  },
}))

const mockLogout = vi.fn()
type MockAuthContextValue = {
  user: {
    email: string
    username?: string
    first_name?: string
    last_name?: string
    is_admin: boolean
  } | null
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

vi.mock('@/features/notifications', () => ({
  NotificationBell: () => <button data-testid="notification-bell">Bell</button>,
}))

// SearchTrigger opens the global CommandPalette directly; assert the call.
const mockOpenCommandPalette = vi.fn()
vi.mock('@/lib/hooks/common/useCommandPalette', () => ({
  openCommandPalette: () => mockOpenCommandPalette(),
}))

describe('TopBar', () => {
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

  describe('brand', () => {
    it('renders the brand link to home with the logo', () => {
      render(<TopBar />)
      const brand = screen.getByRole('link', { name: /psychic homily/i })
      expect(brand).toHaveAttribute('href', '/')
      expect(brand.querySelector('img')).toBeInTheDocument()
      expect(screen.getByText('Psychic Homily')).toBeInTheDocument()
    })
  })

  describe('search', () => {
    it('renders the search field with placeholder + shortcut', () => {
      render(<TopBar />)
      expect(screen.getByText(/Search shows, artists, labels/)).toBeInTheDocument()
      expect(screen.getByText('⌘K')).toBeInTheDocument()
    })

    it('opens the command palette when the search field is clicked', async () => {
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'Search shows, artists, labels' }))
      expect(mockOpenCommandPalette).toHaveBeenCalledTimes(1)
    })

    // PSY-1020: below `sm` the field condenses to an icon-only tap target so
    // search stays reachable on phones.
    it('opens the command palette from the mobile search icon', async () => {
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'Search' }))
      expect(mockOpenCommandPalette).toHaveBeenCalledTimes(1)
    })
  })

  describe('primary nav', () => {
    it('renders the explicit links (incl. Radio, PSY-1057) + the two menus', () => {
      render(<TopBar />)
      expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('href', '/')
      expect(screen.getByRole('link', { name: 'Explore' })).toHaveAttribute('href', '/explore')
      expect(screen.getByRole('link', { name: 'Radio' })).toHaveAttribute('href', '/radio')
      expect(screen.getByRole('button', { name: 'Browse the catalog' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Contribute' })).toBeInTheDocument()
    })
  })

  describe('theme toggle', () => {
    it('renders a bare sun/moon toggle', () => {
      render(<TopBar />)
      expect(screen.getByRole('button', { name: 'Toggle theme' })).toBeInTheDocument()
    })

    it('toggles to light when current theme is dark', async () => {
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
      expect(mockSetTheme).toHaveBeenCalledWith('light')
    })

    it('toggles to dark when current theme is light', async () => {
      mockTheme = 'light'
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
      expect(mockSetTheme).toHaveBeenCalledWith('dark')
    })
  })

  describe('account cluster', () => {
    it('shows the login link and no Submit CTA when unauthenticated', () => {
      render(<TopBar />)
      expect(screen.getAllByText('login / sign-up').length).toBeGreaterThanOrEqual(1)
      // + Submit is an authenticated-only CTA; anon keeps Submit in the
      // Contribute menu (OQ-2).
      expect(screen.queryByRole('link', { name: '+ Submit' })).not.toBeInTheDocument()
    })

    it('shows the + Submit CTA, avatar, and notification bell when authenticated', () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'test@test.com', first_name: 'John', last_name: 'Doe', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      render(<TopBar />)
      expect(screen.getByRole('button', { name: 'User menu' })).toBeInTheDocument()
      expect(screen.getByText('JD')).toBeInTheDocument()
      expect(screen.getByTestId('notification-bell')).toBeInTheDocument()
      expect(screen.getByRole('link', { name: '+ Submit' })).toHaveAttribute('href', '/shows/submit')
    })

    it('hides the Submit CTA, bell + avatar while auth is loading', () => {
      mockAuthContext.mockReturnValue({
        user: null,
        isAuthenticated: false,
        isLoading: true,
        logout: mockLogout,
      })
      render(<TopBar />)
      expect(screen.queryByRole('button', { name: 'User menu' })).not.toBeInTheDocument()
      expect(screen.queryByTestId('notification-bell')).not.toBeInTheDocument()
      expect(screen.queryByRole('link', { name: '+ Submit' })).not.toBeInTheDocument()
      expect(screen.queryByText('login / sign-up')).not.toBeInTheDocument()
    })

    it('opens the account dropdown with profile, admin, and sign out for an admin', async () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'admin@test.com', first_name: 'Ada', last_name: 'Min', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      expect(await screen.findByRole('menuitem', { name: 'Profile' })).toBeInTheDocument()
      expect(screen.getByRole('menuitem', { name: 'Admin' })).toBeInTheDocument()
      expect(screen.getByText('Ada Min')).toBeInTheDocument()
      expect(screen.getByText('admin@test.com')).toBeInTheDocument()
      await user.click(screen.getByRole('menuitem', { name: 'Sign out' }))
      expect(mockLogout).toHaveBeenCalledTimes(1)
    })

    it('does not show the Admin item for a non-admin', async () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'user@test.com', first_name: 'Reg', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      expect(await screen.findByRole('menuitem', { name: 'Profile' })).toBeInTheDocument()
      expect(screen.queryByRole('menuitem', { name: 'Admin' })).not.toBeInTheDocument()
    })

    // PSY-1025: "Profile" lands the user on their OWN public identity view,
    // not the settings form.
    it('points "Profile" at the user public identity page when they have a username', async () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'user@test.com', username: 'reggie', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      const profileItem = await screen.findByRole('menuitem', { name: 'Profile' })
      expect(profileItem).toHaveAttribute('href', '/users/reggie')
    })

    it('falls back "Profile" to /users/me (claim-username self view) when the user has no username', async () => {
      // PSY-1045: previously fell back to /profile (settings); now lands on
      // the claim-username self view so the user still gets the profile
      // experience before picking a handle.
      mockAuthContext.mockReturnValue({
        user: { email: 'user@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      const profileItem = await screen.findByRole('menuitem', { name: 'Profile' })
      expect(profileItem).toHaveAttribute('href', '/users/me')
    })
  })

  // PSY-1020: the public hamburger sheet is retired — the bottom tab bar
  // (BottomTabBar, mounted by AppShell) is the primary mobile nav. The drawer
  // survives only as the admin-sections nav on the /admin tab-shell (PSY-933).
  describe('admin drawer', () => {
    it('renders no hamburger for the public', () => {
      render(<TopBar />)
      expect(screen.queryByRole('button', { name: 'Open admin menu' })).not.toBeInTheDocument()
    })

    it('opens the admin drawer for an admin on the /admin shell', async () => {
      mockPathname = '/admin'
      mockAuthContext.mockReturnValue({
        user: { email: 'admin@test.com', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'Open admin menu' }))
      expect(await screen.findByTestId('admin-drawer-nav')).toBeInTheDocument()
    })

    it('renders no drawer for a non-admin on /admin', () => {
      mockPathname = '/admin'
      mockAuthContext.mockReturnValue({
        user: { email: 'user@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      render(<TopBar />)
      expect(screen.queryByRole('button', { name: 'Open admin menu' })).not.toBeInTheDocument()
    })

    it('renders no drawer for an admin off the /admin shell', () => {
      mockPathname = '/shows'
      mockAuthContext.mockReturnValue({
        user: { email: 'admin@test.com', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      render(<TopBar />)
      expect(screen.queryByRole('button', { name: 'Open admin menu' })).not.toBeInTheDocument()
    })
  })
})
