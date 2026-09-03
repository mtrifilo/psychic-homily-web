import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TopBar } from './TopBar'
import type { AuthStatus } from '@/lib/context/AuthContext'

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
type MockAuthContextValue = {
  user: {
    email: string
    username?: string
    first_name?: string
    last_name?: string
    is_admin: boolean
  } | null
  isAuthenticated: boolean
  authStatus: AuthStatus
  isLoading: boolean
  logout: () => void
}

// One fixture builder rather than a literal per test. It pins one coupling:
// `isAuthenticated` derives from `authStatus` and cannot be overridden, so no
// test asserts against a viewer whose two auth signals disagree. `user` and
// `isLoading` stay overridable, because the cells that matter here differ in
// them.
function authFixture(
  overrides: Partial<Omit<MockAuthContextValue, 'isAuthenticated'>> = {}
): MockAuthContextValue {
  const authStatus = overrides.authStatus ?? 'anonymous'
  return {
    user: null,
    authStatus,
    isLoading: authStatus === 'pending',
    logout: mockLogout,
    ...overrides,
    isAuthenticated: authStatus === 'authenticated',
  }
}

const mockAuthContext = vi.fn<() => MockAuthContextValue>(() => authFixture())
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Constant: the flip's direction is useThemeToggle's, covered in
// mode-toggle.test.tsx. TopBar's test only needs one settled theme to click in.
const mockTheme = 'dark'
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
    mockAuthContext.mockReturnValue(authFixture())
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

    // PSY-1818: below `sm` the field condenses to an icon-only tap target
    // (PSY-1020), but as a responsive form of the SAME button — not a second
    // one. The forked icon button this replaced was always in the DOM beside
    // the field with its own accessible name ("Search"), so assistive tech and
    // tests saw two search controls at every width.
    it('renders exactly one search trigger, under one accessible name', () => {
      render(<TopBar />)
      expect(
        screen.getAllByRole('button', { name: 'Search shows, artists, labels' })
      ).toHaveLength(1)
      expect(screen.queryByRole('button', { name: 'Search' })).not.toBeInTheDocument()
    })

    // Collapsing the two nodes into one split the responsive contract across
    // two files: this bar owns the BOX WIDTH, SearchTrigger owns the CHROME
    // that fills it, and both must switch at the same breakpoint. Diverge and
    // there is a viewport band rendering field chrome inside a 36px box, or a
    // 220px box holding a centred bare icon — CSS-only, so no other test here
    // can see it. Asserts the RELATIONSHIP (same prefix), not the literal `sm`,
    // so moving both together still passes.
    it('grows the search box and the trigger chrome at the same breakpoint', () => {
      const { container } = render(<TopBar />)
      const box = container.querySelector('[role="search"]') as HTMLElement
      const trigger = box.querySelector('button') as HTMLElement
      const breakpointOf = (className: string, utility: string) =>
        className.split(/\s+/).find(c => c.endsWith(`:${utility}`))?.split(':')[0]

      const widthGrowsAt = breakpointOf(box.className, 'w-[220px]')
      const chromeAppearsAt = breakpointOf(trigger.className, 'border')
      expect(widthGrowsAt).toBeDefined()
      expect(chromeAppearsAt).toBe(widthGrowsAt)
    })
  })

  describe('primary nav', () => {
    it('renders the explicit links (incl. Radio, PSY-1057) + the two menus', () => {
      render(<TopBar />)
      expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('href', '/')
      expect(screen.getByRole('link', { name: 'Graph' })).toHaveAttribute('href', '/graph')
      expect(screen.getByRole('link', { name: 'Radio' })).toHaveAttribute('href', '/radio')
      expect(screen.getByRole('button', { name: 'Browse the catalog' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Contribute' })).toBeInTheDocument()
    })

    it('omits the primary nav in the slim (side-nav) variant — nav lives in the sidebar', () => {
      render(<TopBar variant="slim" />)
      expect(screen.queryByRole('link', { name: 'Home' })).not.toBeInTheDocument()
      expect(screen.queryByRole('link', { name: 'Graph' })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Browse the catalog' })).not.toBeInTheDocument()
      // Brand + search stay in the slim bar.
      expect(screen.getByRole('link', { name: 'Psychic Homily — home' })).toBeInTheDocument()
    })
  })

  // PSY-1818: the flip itself (both directions, the theme="system" regression,
  // the undefined-during-hydration case) belongs to useThemeToggle and is
  // covered once, in mode-toggle.test.tsx. What is TopBar's own is that its
  // button is wired to that hook at all.
  describe('theme toggle', () => {
    it('renders a bare sun/moon toggle', () => {
      render(<TopBar />)
      expect(screen.getByRole('button', { name: 'Toggle theme' })).toBeInTheDocument()
    })

    it('flips the theme through useThemeToggle when clicked', async () => {
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
      expect(mockSetTheme).toHaveBeenCalledTimes(1)
      expect(mockSetTheme).toHaveBeenCalledWith('light')
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
      mockAuthContext.mockReturnValue(
        authFixture({ user: { email: 'test@test.com', first_name: 'John', last_name: 'Doe', is_admin: false }, authStatus: 'authenticated' })
      )
      render(<TopBar />)
      expect(screen.getByRole('button', { name: 'User menu' })).toBeInTheDocument()
      expect(screen.getByText('JD')).toBeInTheDocument()
      expect(screen.getByTestId('notification-bell')).toBeInTheDocument()
      expect(screen.getByRole('link', { name: '+ Submit' })).toHaveAttribute('href', '/shows/submit')
    })

    // PSY-1986. Both pending cells, because they differ in the signal the bar
    // used to read: the ordinary pre-profile window has `isLoading` true, and
    // the terminal window a non-definitive failure (5xx, 429, network, 403)
    // leaves behind has it false. In both, the bar keeps the anonymous-safe
    // /auth route and suppresses every control that names a viewer, so a
    // regression to an `isLoading` gate fails on one cell or the other.
    it.each([
      ['while the profile is in flight', true],
      ['after the profile failed without settling', false],
    ])('keeps the login link and asserts no identity %s', (_label, isLoading) => {
      mockAuthContext.mockReturnValue(authFixture({ authStatus: 'pending', isLoading }))
      render(<TopBar />)
      expect(screen.getAllByText('login / sign-up').length).toBeGreaterThanOrEqual(1)
      expect(screen.queryByRole('button', { name: 'User menu' })).not.toBeInTheDocument()
      expect(screen.queryByRole('link', { name: '+ Submit' })).not.toBeInTheDocument()
      expect(screen.queryByTestId('notification-bell')).not.toBeInTheDocument()
      // No spinner either. Lucide's Loader2 renders a bare svg with no role,
      // so the class is what this assertion has to look for.
      expect(document.querySelector('.animate-spin')).toBeNull()
    })

    // The pending and settled-anonymous slots are the SAME markup, which is
    // what keeps the row from reflowing when a pending read settles to
    // anonymous. Comparing the rendered node pins that; asserting the link
    // twice would not.
    it('renders the pending slot identically to the settled-anonymous slot', () => {
      const slotOf = (authStatus: AuthStatus) => {
        mockAuthContext.mockReturnValue(authFixture({ authStatus }))
        const { unmount } = render(<TopBar />)
        const html = screen
          .getAllByText('login / sign-up')
          .map(node => node.outerHTML)
          .join('')
        unmount()
        return html
      }
      const pending = slotOf('pending')
      expect(pending).not.toEqual('')
      expect(pending).toEqual(slotOf('anonymous'))
    })

    // The override built at login makes the viewer authenticated before the
    // profile query resolves, with `isLoading` still true. Gating on that
    // would hold the signed-in cluster back until the profile landed.
    it('shows the authenticated cluster as soon as auth settles, even mid-fetch', () => {
      mockAuthContext.mockReturnValue(
        authFixture({
          user: { email: 'test@test.com', first_name: 'John', last_name: 'Doe', is_admin: false },
          authStatus: 'authenticated',
          isLoading: true,
        })
      )
      render(<TopBar />)
      expect(screen.getByRole('button', { name: 'User menu' })).toBeInTheDocument()
      expect(screen.getByRole('link', { name: '+ Submit' })).toBeInTheDocument()
    })

    it('opens the account dropdown with profile, admin, and sign out for an admin', async () => {
      mockAuthContext.mockReturnValue(
        authFixture({ user: { email: 'admin@test.com', first_name: 'Ada', last_name: 'Min', is_admin: true }, authStatus: 'authenticated' })
      )
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
      mockAuthContext.mockReturnValue(
        authFixture({ user: { email: 'user@test.com', first_name: 'Reg', is_admin: false }, authStatus: 'authenticated' })
      )
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      expect(await screen.findByRole('menuitem', { name: 'Profile' })).toBeInTheDocument()
      expect(screen.queryByRole('menuitem', { name: 'Admin' })).not.toBeInTheDocument()
    })

    // PSY-1025: "Profile" lands the user on their OWN public identity view,
    // not the settings form.
    it('points "Profile" at the user public identity page when they have a username', async () => {
      mockAuthContext.mockReturnValue(
        authFixture({ user: { email: 'user@test.com', username: 'reggie', is_admin: false }, authStatus: 'authenticated' })
      )
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
      mockAuthContext.mockReturnValue(
        authFixture({ user: { email: 'user@test.com', is_admin: false }, authStatus: 'authenticated' })
      )
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      const profileItem = await screen.findByRole('menuitem', { name: 'Profile' })
      expect(profileItem).toHaveAttribute('href', '/users/me')
    })

    // PSY-1486: desktop UserMenu Settings → /profile (parity with the retired hamburger sheet).
    it('points "Settings" at the /profile editor', async () => {
      mockAuthContext.mockReturnValue(
        authFixture({ user: { email: 'user@test.com', username: 'reggie', is_admin: false }, authStatus: 'authenticated' })
      )
      const user = userEvent.setup()
      render(<TopBar />)
      await user.click(screen.getByRole('button', { name: 'User menu' }))
      const settingsItem = await screen.findByRole('menuitem', { name: 'Settings' })
      expect(settingsItem).toHaveAttribute('href', '/profile')
    })
  })

  // The top bar is public chrome: it must carry no admin-only surface for ANY
  // user on ANY route, which is what these guard. The admin drawer lives in
  // app/admin/layout.tsx (PSY-1817) — its behavior is covered by
  // AdminMobileDrawer.test.tsx and its mount by app/admin/layout.test.tsx.
  describe('no admin chrome (PSY-1817)', () => {
    it('renders no hamburger for the public', () => {
      render(<TopBar />)
      expect(screen.queryByRole('button', { name: 'Open admin menu' })).not.toBeInTheDocument()
    })

    it('renders no admin drawer trigger even for an admin on /admin', () => {
      mockPathname = '/admin'
      mockAuthContext.mockReturnValue(
        authFixture({ user: { email: 'admin@test.com', is_admin: true }, authStatus: 'authenticated' })
      )
      render(<TopBar />)
      expect(screen.queryByRole('button', { name: 'Open admin menu' })).not.toBeInTheDocument()
    })
  })
})
