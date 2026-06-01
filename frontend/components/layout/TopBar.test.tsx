import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TopBar } from './TopBar'

let mockPathname = '/'
let mockSearchParams = new URLSearchParams()
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname,
  useSearchParams: () => mockSearchParams,
}))

// Admin nav counts hook — stubbed so tests don't need a QueryClientProvider.
// Reassignable so the admin-drawer badge test can supply non-zero counts.
let mockNavCounts = { moderation: 0, pendingShows: 0, unverifiedVenues: 0, reports: 0 }
vi.mock('@/lib/hooks/admin/useAdminNavCounts', () => ({
  useAdminNavCounts: () => mockNavCounts,
}))

vi.mock('next/image', () => ({
  default: (props: Record<string, unknown>) => {
    const { priority, ...rest } = props
    return <img {...rest} data-priority={priority ? 'true' : undefined} />
  },
}))

const mockLogout = vi.fn()
// Return type widened so individual tests can override `user`/`isAuthenticated`
// without TS narrowing from the default-null literal.
type MockAuthContextValue = {
  user: {
    email: string
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
  useTheme: () => ({ theme: mockTheme, setTheme: mockSetTheme }),
}))

// Mock NotificationBell to a stub — it uses TanStack Query and is covered
// by its own unit test (PSY-595). The TopBar test only cares that the bell
// renders for authenticated users; the bell's internal behaviour is out of
// scope here.
vi.mock('@/features/notifications', () => ({
  NotificationBell: () => <button data-testid="notification-bell">Bell</button>,
}))

describe('TopBar', () => {
  const onMobileOpenChange = vi.fn()
  const onSearchClick = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname = '/'
    mockSearchParams = new URLSearchParams()
    mockNavCounts = { moderation: 0, pendingShows: 0, unverifiedVenues: 0, reports: 0 }
    mockTheme = 'dark'
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: mockLogout,
    })
  })

  it('renders logo image', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByAltText('Psychic Homily Logo')).toBeInTheDocument()
  })

  it('renders site name', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByText('Psychic Homily')).toBeInTheDocument()
  })

  it('renders search placeholder button', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByText('Search...')).toBeInTheDocument()
    expect(screen.getByText('\u2318K')).toBeInTheDocument()
  })

  it('calls onSearchClick when search button is clicked', async () => {
    const user = userEvent.setup()
    render(
      <TopBar
        mobileOpen={false}
        onMobileOpenChange={onMobileOpenChange}
        onSearchClick={onSearchClick}
      />
    )
    await user.click(screen.getByText('Search...'))
    expect(onSearchClick).toHaveBeenCalledTimes(1)
  })

  it('shows login link when unauthenticated', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    // There are two login links (desktop + mobile), just verify at least one exists
    const links = screen.getAllByText('login / sign-up')
    expect(links.length).toBeGreaterThanOrEqual(1)
  })

  it('shows user avatar button when authenticated', () => {
    mockAuthContext.mockReturnValue({
      user: { email: 'test@test.com', first_name: 'John', last_name: 'Doe', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: mockLogout,
    })
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByRole('button', { name: 'User menu' })).toBeInTheDocument()
    expect(screen.getByText('JD')).toBeInTheDocument()
  })

  it('shows loading spinner while auth is loading', () => {
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: true,
      logout: mockLogout,
    })
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.queryByRole('button', { name: 'User menu' })).not.toBeInTheDocument()
  })

  it('renders hamburger menu button', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByRole('button', { name: 'Open menu' })).toBeInTheDocument()
  })

  it('renders theme toggle button', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByRole('button', { name: 'Toggle theme' })).toBeInTheDocument()
  })

  it('calls setTheme when desktop theme toggle is clicked', async () => {
    const user = userEvent.setup()
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(mockSetTheme).toHaveBeenCalledWith('light')
  })

  it('toggles to dark when current theme is light', async () => {
    mockTheme = 'light'
    const user = userEvent.setup()
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(mockSetTheme).toHaveBeenCalledWith('dark')
  })

  describe('authenticated user dropdown', () => {
    beforeEach(() => {
      mockAuthContext.mockReturnValue({
        user: {
          email: 'admin@test.com',
          first_name: 'Alice',
          last_name: 'Admin',
          is_admin: true,
        },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
    })

    it('shows admin link in dropdown when user is admin', async () => {
      const user = userEvent.setup()
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      // Dropdown should show Admin link
      expect(screen.getByRole('menuitem', { name: /Admin/ })).toBeInTheDocument()
    })

    it('shows profile link in dropdown', async () => {
      const user = userEvent.setup()
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      expect(screen.getByRole('menuitem', { name: /Profile/ })).toBeInTheDocument()
    })

    it('shows sign out in dropdown and calls logout on click', async () => {
      const user = userEvent.setup()
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      const signOutItem = screen.getByRole('menuitem', { name: /Sign out/ })
      expect(signOutItem).toBeInTheDocument()
      await user.click(signOutItem)
      expect(mockLogout).toHaveBeenCalledTimes(1)
    })

    it('shows user email in dropdown', async () => {
      const user = userEvent.setup()
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      expect(screen.getByText('admin@test.com')).toBeInTheDocument()
    })

    it('shows user display name in dropdown', async () => {
      const user = userEvent.setup()
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      expect(screen.getByText('Alice Admin')).toBeInTheDocument()
    })
  })

  describe('non-admin authenticated user', () => {
    beforeEach(() => {
      mockAuthContext.mockReturnValue({
        user: {
          email: 'user@test.com',
          first_name: 'Bob',
          is_admin: false,
        },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
    })

    it('does not show admin link in dropdown for non-admin', async () => {
      const user = userEvent.setup()
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      const menuItems = screen.getAllByRole('menuitem')
      const adminItem = menuItems.find(item => item.textContent?.includes('Admin'))
      expect(adminItem).toBeUndefined()
    })

    it('shows initials from first name only when no last name', () => {
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('B')).toBeInTheDocument()
    })
  })

  describe('user with email only (no name)', () => {
    beforeEach(() => {
      mockAuthContext.mockReturnValue({
        user: {
          email: 'emailonly@test.com',
          is_admin: false,
        },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
    })

    it('shows email initial as avatar', () => {
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('E')).toBeInTheDocument()
    })

    it('does not show display name in dropdown when no name provided', async () => {
      const user = userEvent.setup()
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      // Only the email should appear, not a separate display name
      expect(screen.getByText('emailonly@test.com')).toBeInTheDocument()
    })
  })

  describe('mobile menu content', () => {
    it('shows sidebar navigation groups in mobile menu', () => {
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('Discover')).toBeInTheDocument()
      expect(screen.getByText('Community')).toBeInTheDocument()
    })

    it('shows Library and Settings links when authenticated on mobile', () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'test@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('Library')).toBeInTheDocument()
      expect(screen.getByText('Settings')).toBeInTheDocument()
      // "Collection" singular should not exist; the Sidebar Discover group's
      // "Collections" (plural) lives elsewhere.
      expect(screen.queryByText('Collection')).not.toBeInTheDocument()
    })

    it('shows Admin link on mobile when user is admin', () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'admin@test.com', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      // Admin link should appear in the mobile menu
      const adminLinks = screen.getAllByText('Admin')
      expect(adminLinks.length).toBeGreaterThanOrEqual(1)
    })

    it('does not show Admin link on mobile when user is not admin', () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'user@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      // No admin link should appear
      expect(screen.queryByText('Admin')).not.toBeInTheDocument()
    })

    it('calls logout and closes mobile menu on sign out click', async () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'test@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      const user = userEvent.setup()
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      await user.click(screen.getByText('Sign out'))
      expect(mockLogout).toHaveBeenCalledTimes(1)
      expect(onMobileOpenChange).toHaveBeenCalledWith(false)
    })

    it('shows mobile theme toggle with correct label in dark mode', () => {
      mockTheme = 'dark'
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('Light mode')).toBeInTheDocument()
    })

    it('shows mobile theme toggle with correct label in light mode', () => {
      mockTheme = 'light'
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('Dark mode')).toBeInTheDocument()
    })

    it('shows user email in mobile authenticated section', () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'mobile@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('mobile@test.com')).toBeInTheDocument()
    })

    it('shows login link in mobile menu when unauthenticated', () => {
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      const loginLinks = screen.getAllByText('login / sign-up')
      expect(loginLinks.length).toBeGreaterThanOrEqual(1)
    })

    it('highlights active nav item based on pathname', () => {
      mockPathname = '/shows'
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      const showsLink = screen.getByText('Shows').closest('a')
      expect(showsLink?.className).toContain('bg-accent')
    })

    it('highlights items for sub-paths', () => {
      mockPathname = '/artists/some-artist'
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      const artistsLink = screen.getByText('Artists').closest('a')
      expect(artistsLink?.className).toContain('bg-accent')
    })

    it('clicking a mobile nav link closes the mobile menu', async () => {
      const user = userEvent.setup()
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)

      await user.click(screen.getByText('Shows'))
      expect(onMobileOpenChange).toHaveBeenCalledWith(false)
    })

    it('mobile theme toggle calls setTheme on click (light branch)', async () => {
      mockTheme = 'light'
      const user = userEvent.setup()
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)

      // The text "Dark mode" lives on the mobile toggle when current is light.
      await user.click(screen.getByText('Dark mode'))
      expect(mockSetTheme).toHaveBeenCalledWith('dark')
    })

    it('mobile theme toggle calls setTheme on click (dark branch)', async () => {
      mockTheme = 'dark'
      const user = userEvent.setup()
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)

      await user.click(screen.getByText('Light mode'))
      expect(mockSetTheme).toHaveBeenCalledWith('light')
    })

    it('clicking the mobile Notifications link closes the menu (authenticated)', async () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'test@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      const user = userEvent.setup()
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)

      await user.click(screen.getByText('Notifications'))
      expect(onMobileOpenChange).toHaveBeenCalledWith(false)
    })
  })

  // PSY-933: the mobile drawer is the ONLY admin nav on mobile (the desktop
  // Sidebar is hidden < md), so this path is load-bearing — mirror the desktop
  // Sidebar admin-nav suite against the drawer.
  describe('context-aware admin drawer (PSY-933)', () => {
    const asAdmin = () =>
      mockAuthContext.mockReturnValue({
        user: { email: 'admin@test.com', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
    const ACTIVE = 'bg-accent text-accent-foreground'

    it('swaps to the admin groups + Back to site for an admin under /admin', () => {
      asAdmin()
      mockPathname = '/admin'
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('Moderation & Queues')).toBeInTheDocument()
      expect(screen.getByText('Catalog')).toBeInTheDocument()
      expect(screen.getByText('Insights & System')).toBeInTheDocument()
      expect(screen.getByText('Back to site')).toBeInTheDocument()
      // Public groups are swapped out.
      expect(screen.queryByText('Discover')).not.toBeInTheDocument()
      expect(screen.queryByText('Community')).not.toBeInTheDocument()
    })

    it('keeps the public nav for a non-admin even under /admin', () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'user@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      mockPathname = '/admin'
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('Discover')).toBeInTheDocument()
      expect(screen.queryByText('Moderation & Queues')).not.toBeInTheDocument()
    })

    it('marks the section matching ?tab= as active', () => {
      asAdmin()
      mockPathname = '/admin'
      mockSearchParams = new URLSearchParams('tab=moderation')
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('Moderation').closest('a')!.className).toContain(ACTIVE)
      expect(screen.getByText('Releases').closest('a')!.className).not.toContain(ACTIVE)
    })

    it('renders queue badges from useAdminNavCounts and omits zero counts', () => {
      asAdmin()
      mockPathname = '/admin'
      mockNavCounts = { moderation: 5, pendingShows: 2, unverifiedVenues: 0, reports: 3 }
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(within(screen.getByText('Moderation').closest('a')!).getByText('5')).toBeInTheDocument()
      expect(within(screen.getByText('Reports').closest('a')!).getByText('3')).toBeInTheDocument()
      expect(
        within(screen.getByText('Unverified Venues').closest('a')!).queryByText('0')
      ).not.toBeInTheDocument()
    })

    it('clicking an admin section closes the drawer', async () => {
      asAdmin()
      mockPathname = '/admin'
      const user = userEvent.setup()
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      await user.click(screen.getByText('Moderation'))
      expect(onMobileOpenChange).toHaveBeenCalledWith(false)
    })

    it('Back to site points at / and closes the drawer', async () => {
      asAdmin()
      mockPathname = '/admin'
      const user = userEvent.setup()
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      const back = screen.getByText('Back to site').closest('a')!
      expect(back).toHaveAttribute('href', '/')
      await user.click(back)
      expect(onMobileOpenChange).toHaveBeenCalledWith(false)
    })

    it('keeps the public nav on standalone /admin/<section> sub-routes (scoped to the tab-shell, not startsWith)', () => {
      asAdmin()
      mockPathname = '/admin/featured'
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('Discover')).toBeInTheDocument()
      expect(screen.queryByText('Moderation & Queues')).not.toBeInTheDocument()
    })
  })

  describe('notification bell visibility', () => {
    it('renders NotificationBell only when authenticated', () => {
      // Default mock is unauthenticated → no bell.
      const { rerender } = render(
        <TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />
      )
      expect(screen.queryByTestId('notification-bell')).not.toBeInTheDocument()

      mockAuthContext.mockReturnValue({
        user: { email: 'bell@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      rerender(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByTestId('notification-bell')).toBeInTheDocument()
    })

    it('hides NotificationBell while auth is loading', () => {
      mockAuthContext.mockReturnValue({
        user: null,
        isAuthenticated: false,
        isLoading: true,
        logout: mockLogout,
      })
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.queryByTestId('notification-bell')).not.toBeInTheDocument()
    })
  })

  describe('logo link', () => {
    it('logo links to the home page', () => {
      render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
      const logo = screen.getByAltText('Psychic Homily Logo')
      const link = logo.closest('a')
      expect(link).toHaveAttribute('href', '/')
    })
  })
})
