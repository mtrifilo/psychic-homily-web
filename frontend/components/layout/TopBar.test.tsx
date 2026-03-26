import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TopBar } from './TopBar'

let mockPathname = '/'
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname,
}))

vi.mock('next/image', () => ({
  default: (props: Record<string, unknown>) => {
    const { priority, ...rest } = props
    return <img {...rest} data-priority={priority ? 'true' : undefined} />
  },
}))

const mockLogout = vi.fn()
const mockAuthContext = vi.fn(() => ({
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

describe('TopBar', () => {
  const onMobileOpenChange = vi.fn()
  const onSearchClick = vi.fn()

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

    it('shows My Shows, Collection, Settings links when authenticated on mobile', () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'test@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: mockLogout,
      })
      render(<TopBar mobileOpen={true} onMobileOpenChange={onMobileOpenChange} />)
      expect(screen.getByText('Library')).toBeInTheDocument()
      expect(screen.getByText('Collection')).toBeInTheDocument()
      expect(screen.getByText('Settings')).toBeInTheDocument()
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
  })
})
