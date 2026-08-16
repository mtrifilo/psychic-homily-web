import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BottomTabBar } from './BottomTabBar'
import { primaryLinks } from './PrimaryNav'
import { mobileBrowseHrefs, primaryTabs, sidebarGroups } from './navData'
import { BOTTOM_TAB_BAR_BOX } from '@/test/layoutContracts'

let mockPathname = '/'
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname,
}))

// next/link stands in as a plain anchor so the sheet rows' prefetch posture is
// assertable (PSY-1820). forwardRef because SheetClose wraps these rows in a
// Radix Slot, which passes a ref down. `prefetch` is re-emitted as a data-
// attribute rather than spread: React warns on a `false` non-boolean attribute.
vi.mock('next/link', () => {
  const MockLink = React.forwardRef<
    HTMLAnchorElement,
    React.AnchorHTMLAttributes<HTMLAnchorElement> & {
      href: string
      prefetch?: boolean
    }
  >(({ href, children, prefetch, ...props }, ref) => (
    <a href={href} ref={ref} data-prefetch={String(prefetch)} {...props}>
      {children}
    </a>
  ))
  MockLink.displayName = 'MockLink'
  return { default: MockLink }
})

const mockLogout = vi.fn()
type MockAuthContextValue = {
  user: { email: string; username?: string; is_admin: boolean } | null
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

// The unread badge's data source (PSY-1819). Mocked rather than provided via a
// QueryClient so these tests state the COUNT they exercise; the hook's own
// auth-gating and cache-sharing are covered in features/notifications/hooks.
const mockUnreadCount = vi.fn(() => 0)
vi.mock('@/features/notifications', () => ({
  useUnreadNotificationCount: () => mockUnreadCount(),
}))

let mockTheme = 'dark'
let mockResolvedTheme = 'dark'
const mockSetTheme = vi.fn()
vi.mock('next-themes', () => ({
  useTheme: () => ({
    theme: mockTheme,
    resolvedTheme: mockResolvedTheme,
    setTheme: mockSetTheme,
  }),
}))

function authedAs(user: { email: string; username?: string; is_admin: boolean }) {
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
    mockResolvedTheme = 'dark'
    mockUnreadCount.mockReturnValue(0)
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: mockLogout,
    })
  })

  // Mobile-reachability guard, simplified per PSY-1821: primaryLinks and
  // sidebarGroups now COMPOSE from the same canonical tables the mobile
  // surfaces render, so for entries composed the natural way reachability is
  // structural and this cannot fail. It survives as a fence for the residual
  // fork paths only: an inline literal added to either list, or a rail entry
  // composed from the account tables (which the mobile sheets don't render).
  it('keeps every desktop primary + rail destination reachable on mobile', () => {
    const mobile = [...primaryTabs.map(t => t.href), ...mobileBrowseHrefs]
    for (const link of [...primaryLinks, ...sidebarGroups.flatMap(g => g.items)]) {
      if (link.external) continue
      expect(mobile).toContain(link.href)
    }
  })

  // primaryTabs' length is coupled to the bar's literal grid-cols-N (tabs +
  // Browse + Account). A tab added without updating the grid class would wrap
  // Account onto a second row outside the fixed bar height — this asserts the
  // RELATIONSHIP, so it fails on the mismatch, not on a correct 4-tab change.
  it('pins the bar grid to primaryTabs + Browse + Account', () => {
    const { container } = render(<BottomTabBar />)
    const grid = container.querySelector('nav[aria-label="Mobile navigation"] > div')
    expect(grid?.className).toContain(`grid-cols-${primaryTabs.length + 2}`)
  })

  // The bar/PrimaryNav breakpoint contract: the bar hides exactly where the
  // desktop primary nav appears. If either literal changes alone, tablets get
  // double nav or none.
  it('hides at xl, the breakpoint PrimaryNav appears at', () => {
    const { container } = render(<BottomTabBar />)
    expect(container.querySelector('nav[aria-label="Mobile navigation"]')).toHaveClass(
      'xl:hidden'
    )
  })

  // PSY-1820 geometry contract. The bar must RENDER exactly the height every
  // other surface RESERVES for it, or page content slides under the bar (too
  // small) or floats above a gap (too large). These assert the two halves of
  // that contract as literals, because the failure is invisible in jsdom and
  // on any non-notched device — it only shows up on real hardware.
  describe('geometry', () => {
    // BOTTOM_TAB_BAR_BOX is the shared copy AppShell.test.tsx asserts from the
    // other side as `pb-[…]`. Rendering and reserving must be the SAME string,
    // so neither file gets to redefine it locally.
    it('renders at exactly the height AppShell reserves for it', () => {
      const { container } = render(<BottomTabBar />)
      expect(container.querySelector('nav[aria-label="Mobile navigation"]')).toHaveClass(
        `h-[${BOTTOM_TAB_BAR_BOX}]`
      )
    })

    // --bottom-tab-bar-height INCLUDES the 1px border-t, and the box is
    // border-box, so the row must derive its height from the bar (h-full)
    // rather than restating the var — restating it would render 1px taller
    // than the reservation, which is the bug this contract exists to prevent.
    it('derives the tab row from the bar box instead of restating the var', () => {
      const { container } = render(<BottomTabBar />)
      const grid = container.querySelector('nav[aria-label="Mobile navigation"] > div')
      expect(grid).toHaveClass('h-full')
      expect(grid?.className).not.toContain('h-[var(--bottom-tab-bar-height)]')
    })

    // viewport-fit=cover makes the left/right insets nonzero in landscape, so
    // the outermost tabs would otherwise sit under the notch / rounded corner.
    // Padding (not an inset-x offset) so the background still reaches both
    // screen edges.
    it('insets its tabs from the landscape notch band', () => {
      const { container } = render(<BottomTabBar />)
      expect(container.querySelector('nav[aria-label="Mobile navigation"]')).toHaveClass(
        'pl-[env(safe-area-inset-left)]',
        'pr-[env(safe-area-inset-right)]'
      )
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
    it('opens the long-tail sheet with grouped destinations, incl. Graph + Atlas', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      // Graph/Atlas only live here on mobile (desktop carries them in PrimaryNav).
      expect(await screen.findByRole('link', { name: 'Graph' })).toHaveAttribute('href', '/graph')
      expect(screen.getByRole('link', { name: 'Atlas' })).toHaveAttribute('href', '/atlas')
      expect(screen.getByRole('link', { name: 'Festivals' })).toHaveAttribute('href', '/festivals')
      expect(screen.getByRole('link', { name: /Substack/ })).toHaveAttribute(
        'href',
        'https://psychichomily.substack.com/'
      )
      expect(screen.getByText('Catalog')).toBeInTheDocument()
      expect(screen.getByText('Curation')).toBeInTheDocument()
    })

    // PSY-1820 prefetch posture. Opening the sheet mounts two dozen links on a
    // phone-only surface, so every ROW opts out of Next's viewport prefetch
    // while the five always-visible TAB links keep theirs. Asserted as a
    // partition (all rows opted out, no tab opted out) rather than by naming
    // routes, so adding a destination cannot quietly reintroduce the burst.
    it('opts sheet rows out of prefetch while the primary tabs keep it', async () => {
      const user = userEvent.setup()
      const { container } = render(<BottomTabBar />)

      const tabs = [...container.querySelectorAll('nav[aria-label="Mobile navigation"] a')]
      expect(tabs.length).toBeGreaterThan(0)
      expect(tabs.every(a => a.getAttribute('data-prefetch') !== 'false')).toBe(true)

      await user.click(screen.getByRole('button', { name: 'Browse' }))
      await screen.findByRole('link', { name: 'Graph' })

      const rows = [...document.querySelectorAll('[data-slot="sheet-content"] a')]
      expect(rows.length).toBeGreaterThan(10)
      expect(rows.every(a => a.getAttribute('data-prefetch') === 'false')).toBe(true)
    })

    it('hides auth-only destinations from anonymous visitors', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      await screen.findByRole('link', { name: 'Graph' })
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
      // The hamburger sheet's Show Submissions link, re-homed here (its old
      // TopBar test was deleted with that surface).
      expect(screen.getByRole('link', { name: 'Show Submissions' })).toHaveAttribute(
        'href',
        '/contribute/submissions'
      )
    })

    it('renders each destination once — Leaderboard is in two desktop menus but ONE sheet', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      // getByRole throws on multiple matches, so this is the dedup guard.
      expect(await screen.findByRole('link', { name: 'Leaderboard' })).toHaveAttribute(
        'href',
        '/community/leaderboard'
      )
    })

    it('keeps the primary-color CTA treatment on Submit a Show (Figma 460:3)', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      expect(await screen.findByRole('link', { name: 'Submit a Show' })).toHaveClass(
        'text-primary'
      )
    })

    it('closes when a destination is clicked', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      await user.click(await screen.findByRole('link', { name: 'Festivals' }))
      expect(screen.queryByRole('link', { name: 'Graph' })).not.toBeInTheDocument()
    })

    it('closes on a route change it did not cause — Android Back must land on a visible page', async () => {
      const user = userEvent.setup()
      const { rerender } = render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      await screen.findByRole('link', { name: 'Graph' })
      mockPathname = '/shows'
      rerender(<BottomTabBar />)
      await waitFor(() =>
        expect(screen.queryByRole('link', { name: 'Graph' })).not.toBeInTheDocument()
      )
    })

    it('carries the theme toggle (migrated from the retired hamburger sheet)', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      await user.click(await screen.findByRole('button', { name: 'Light mode' }))
      expect(mockSetTheme).toHaveBeenCalledWith('light')
    })

    it('flips the VISIBLE theme on the first click under theme="system"', async () => {
      // Regression: keying the toggle off `theme` (not resolvedTheme) would
      // setTheme('dark') on a system-dark device — an apparent no-op.
      mockTheme = 'system'
      mockResolvedTheme = 'dark'
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

    it('deep-links Profile to /users/<username> when the user has one (PSY-1045 rule)', async () => {
      authedAs({ email: 'reg@test.com', username: 'reggie', is_admin: false })
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Account' }))
      expect(await screen.findByRole('link', { name: 'Profile' })).toHaveAttribute(
        'href',
        '/users/reggie'
      )
    })

    it('lights the Account tab on the username profile route', () => {
      authedAs({ email: 'reg@test.com', username: 'reggie', is_admin: false })
      mockPathname = '/users/reggie'
      render(<BottomTabBar />)
      expect(screen.getByRole('button', { name: 'Account' })).toHaveAttribute(
        'aria-current',
        'page'
      )
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

  // PSY-1819. Below `sm` the top bar hides the notification bell, so before
  // this the unread count was unreachable on a phone. It rides the Account tab
  // (visible with nothing open) AND the sheet's Notifications row (so it stays
  // attached to the destination that clears it once the sheet covers the bar).
  describe('unread badge', () => {
    it('badges the Account tab with the count, and announces it in the tab name', () => {
      authedAs({ email: 'user@test.com', is_admin: false })
      mockUnreadCount.mockReturnValue(3)
      render(<BottomTabBar />)
      const tab = screen.getByRole('button', { name: 'Account (3 unread)' })
      expect(within(tab).getByTestId('unread-count-badge')).toHaveTextContent('3')
    })

    it('keeps the plain "Account" name at zero unread, with no badge', () => {
      authedAs({ email: 'user@test.com', is_admin: false })
      render(<BottomTabBar />)
      expect(screen.getByRole('button', { name: 'Account' })).toBeInTheDocument()
      expect(screen.queryByTestId('unread-count-badge')).not.toBeInTheDocument()
    })

    it('badges the sheet Notifications row too, and no other row', async () => {
      authedAs({ email: 'user@test.com', is_admin: false })
      mockUnreadCount.mockReturnValue(7)
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Account (7 unread)' }))
      const row = await screen.findByRole('link', { name: 'Notifications (7 unread)' })
      expect(row).toHaveAttribute('href', '/notifications')
      expect(within(row).getByTestId('unread-count-badge')).toHaveTextContent('7')
      // My Library keeps its bare name — the badge is scoped to its destination.
      expect(screen.getByRole('link', { name: 'My Library' })).toBeInTheDocument()
      // Tab + row, and nothing else in the tree.
      expect(screen.getAllByTestId('unread-count-badge')).toHaveLength(2)
    })

    // Both non-authed Account renderings are badge-less by construction, not
    // just because the hook happens to return 0 — so force a non-zero count and
    // assert nothing renders.
    it('never badges an anonymous visitor', () => {
      mockUnreadCount.mockReturnValue(5)
      render(<BottomTabBar />)
      expect(screen.queryByTestId('unread-count-badge')).not.toBeInTheDocument()
    })

    it('never badges the inert placeholder while auth is hydrating', () => {
      mockAuthContext.mockReturnValue({
        user: null,
        isAuthenticated: false,
        isLoading: true,
        logout: mockLogout,
      })
      mockUnreadCount.mockReturnValue(5)
      render(<BottomTabBar />)
      expect(screen.queryByTestId('unread-count-badge')).not.toBeInTheDocument()
    })
  })
})
