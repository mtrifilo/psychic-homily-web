import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BottomTabBar } from './BottomTabBar'
import { primaryLinks } from './PrimaryNav'
import { legalSheetRows, mobileBrowseHrefs, primaryTabs, sidebarGroups } from './navData'
import { BOTTOM_TAB_BAR_BOX } from '@/test/layoutContracts'
import type { AuthStatus } from '@/lib/context/AuthContext'
import { makeAuthFixture, type MockAuthContextValue } from '@/test/authFixture'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import { setPendingLinkHref } from '@/test/mocks/nextLink'

let mockPathname = '/'
const mockRouter = vi.hoisted(() => ({ prefetch: vi.fn() }))
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname,
  useRouter: () => mockRouter,
}))

// next/link stands in as a plain anchor so the sheet rows' prefetch posture is
// assertable. SheetClose wraps these rows in a Radix Slot, which passes a ref
// down; the shared stub spreads it onto the anchor.
vi.mock('next/link', () => import('@/test/mocks/nextLink'))

const mockLogout = vi.fn()
type MockUser = { email: string; username?: string; is_admin: boolean }

// The same shared builder TopBar.test.tsx uses (test/authFixture.ts), because
// the two bars gate the Account affordance on the same signal and must be
// described in the same terms.
const authFixture = makeAuthFixture<MockUser>(mockLogout)

const mockAuthContext = vi.fn<() => MockAuthContextValue<MockUser>>(() =>
  authFixture()
)
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// The unread badge's data source (PSY-1819). Mocked rather than provided via a
// QueryClient so these tests state the COUNT they exercise; the hook's own
// auth-gating and cache-sharing are covered in features/notifications/hooks.
const mockUnreadCount = vi.fn(() => 0)
vi.mock('@/features/notifications/hooks', () => ({
  useUnreadNotificationCount: () => mockUnreadCount(),
}))

// The Account sheet's Cookie preferences row calls the same openPreferences
// the footer and the banner call; BottomTabBar.legalRows.test.tsx runs it
// against the real provider and dialog.
const mockOpenPreferences = vi.fn()
vi.mock('@/lib/context/CookieConsentContext', () => ({
  useCookieConsent: () => ({ openPreferences: mockOpenPreferences }),
}))

// No `theme` field on purpose: the sheet reads useThemeToggle, which keys off
// resolvedTheme alone. Advertising a `theme` the tree cannot observe is what
// made the removed theme="system" duplicate here look meaningful; that rule is
// pinned once, in mode-toggle.test.tsx.
const mockResolvedTheme = 'dark'
const mockSetTheme = vi.fn()
vi.mock('next-themes', () => ({
  useTheme: () => ({
    resolvedTheme: mockResolvedTheme,
    setTheme: mockSetTheme,
  }),
}))

function authedAs(user: MockUser) {
  mockAuthContext.mockReturnValue(authFixture({ user, authStatus: 'authenticated' }))
}

describe('BottomTabBar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname = '/'
    mockUnreadCount.mockReturnValue(0)
    mockAuthContext.mockReturnValue(authFixture())
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

  // Floating overlays that must stay clear of the bar (the first-save hint)
  // find it by this attribute; FirstSaveHint.test pins what they do with it.
  it('carries the hook floating overlays measure', () => {
    const { container } = render(<BottomTabBar />)
    expect(
      container.querySelector('nav[aria-label="Mobile navigation"]')
    ).toHaveAttribute('data-bottom-tab-bar')
  })

  // PSY-1820 geometry contract. The bar must RENDER exactly the height every
  // other surface RESERVES for it, or page content slides under the bar.
  //
  // What these tests actually prove, precisely: that the two components still
  // spell the same expression. They do NOT evaluate it — jsdom computes no
  // layout, and env()/calc() never resolve here. The arithmetic itself (that
  // the expression yields the bar's true box) was checked in a real engine and
  // has to be re-checked in one; do not read a green suite as proof of the
  // pixels.
  describe('geometry', () => {
    // BOTTOM_TAB_BAR_BOX is the shared copy AppShell.test.tsx asserts from the
    // other side as `pb-[…]`, so editing one component's class text without the
    // other fails here rather than silently shipping.
    it('renders at exactly the height AppShell reserves for it', () => {
      const { container } = render(<BottomTabBar />)
      expect(container.querySelector('nav[aria-label="Mobile navigation"]')).toHaveClass(
        `h-[${BOTTOM_TAB_BAR_BOX}]`
      )
    })

    // --bottom-tab-bar-height INCLUDES the border, and the box is border-box,
    // so the row must derive its height from the bar (h-full). Any restated
    // height — the old h-[var(--bottom-tab-bar-height)], h-14, h-[3.5rem] —
    // renders taller than the reservation, which is the bug this exists to
    // prevent. h-full is the only spelling that cannot drift, so assert it
    // positively rather than blacklisting one wrong spelling.
    it('derives the tab row from the bar box instead of restating a height', () => {
      const { container } = render(<BottomTabBar />)
      const grid = container.querySelector('nav[aria-label="Mobile navigation"] > div')
      expect(grid).toHaveClass('h-full')
    })

    // --bottom-tab-bar-height hardcodes this border as `+ 1px`, and CSS cannot
    // read a Tailwind class back. `border-t` is Tailwind's 1px default, so
    // swapping it for border-t-2 (or dropping it for a shadow) silently makes
    // every subtractor in the app wrong again — the exact PSY-1820 defect.
    // Pinning the class is what makes that edit fail here instead of on a
    // phone. If you change it, change the var in globals.css in the same edit.
    it('keeps the 1px border the height variable reserves for it', () => {
      const { container } = render(<BottomTabBar />)
      expect(container.querySelector('nav[aria-label="Mobile navigation"]')).toHaveClass(
        'border-t'
      )
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

    // On /atlas the tab bar's links are the primary destinations that re-arm
    // their prefetch once the map is up; the sheet rows never prefetch.
    it('re-arms the tab links on /atlas once the map is up, and only them', async () => {
      // A fresh module graph, so releasing this copy of the page-load signal
      // leaves the rest of the file's BottomTabBar untouched.
      vi.resetModules()
      const { markAtlasMapReady } = await import('@/lib/atlasMapReady')
      const { BottomTabBar: FreshTabBar } = await import('./BottomTabBar')
      mockPathname = '/atlas'
      const tabNames = ['Home', 'Shows', 'Radio']
      const user = userEvent.setup()
      const { rerender } = render(<FreshTabBar />)
      for (const name of tabNames) {
        expect(screen.getByRole('link', { name })).toHaveAttribute('data-prefetch', 'false')
      }
      act(() => markAtlasMapReady())
      rerender(<FreshTabBar />)
      for (const name of tabNames) {
        expect(screen.getByRole('link', { name })).toHaveAttribute('data-prefetch', 'default')
      }
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      const sheet = await screen.findByRole('dialog')
      for (const row of within(sheet).getAllByRole('link')) {
        expect(row).toHaveAttribute('data-prefetch', 'false')
      }
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

  // Product Designs XakQQ0nYGqnt77PrHKO9IE, boards 1955:76/1955:104 (idle)
  // and 1955:90/1955:118 (pending). jsdom computes no :has() styles, so these
  // pin the markers the bar's pending rules select on; the colours themselves
  // are checked in a browser (PR screenshots).
  describe('pending tab', () => {
    afterEach(() => {
      setPendingLinkHref(null)
    })

    it('renders every tab icon and no pending marker while idle', () => {
      mockPathname = '/atlas'
      const { container } = render(<BottomTabBar />)
      expect(container.querySelector('[data-tab-pending]')).toBeNull()
      for (const name of ['Home', 'Shows', 'Radio']) {
        expect(screen.getByRole('link', { name }).querySelector('svg.lucide')).not.toBeNull()
      }
      for (const name of ['Browse', 'Account']) {
        expect(screen.getByRole('button', { name }).querySelector('svg.lucide')).not.toBeNull()
      }
    })

    it('swaps the tapped tab icon for the spinner until its destination commits', () => {
      mockPathname = '/atlas'
      setPendingLinkHref('/shows')
      render(<BottomTabBar />)

      const shows = screen.getByRole('link', { name: 'Shows' })
      const marker = shows.querySelector('[data-tab-pending]')
      expect(marker).not.toBeNull()
      expect(marker!.querySelector('svg.animate-spin')).not.toBeNull()
      expect(shows.querySelector('svg.lucide')).toBeNull()
      // The label stays, and the page has not changed yet.
      expect(shows).toHaveTextContent('Shows')
      expect(shows).not.toHaveAttribute('aria-current')

      for (const tab of [
        ...['Home', 'Radio'].map(name => screen.getByRole('link', { name })),
        screen.getByRole('button', { name: 'Account' }),
      ]) {
        expect(tab.querySelector('[data-tab-pending]')).toBeNull()
        expect(tab.querySelector('svg.lucide')).not.toBeNull()
      }
    })

    it('gives every tab, links and sheet triggers alike, the rules that light the pending tab and mute the rest', () => {
      mockPathname = '/atlas'
      render(<BottomTabBar />)
      expect(screen.getByRole('navigation', { name: 'Mobile navigation' })).toHaveClass(
        'group/tabbar'
      )
      const tabs = [
        ...['Home', 'Shows', 'Radio'].map(name => screen.getByRole('link', { name })),
        ...['Browse', 'Account'].map(name => screen.getByRole('button', { name })),
      ]
      for (const tab of tabs) {
        expect(tab).toHaveClass('has-[[data-tab-pending]]:text-primary')
        expect(tab).toHaveClass(
          'group-has-[[data-tab-pending]]/tabbar:not-has-[[data-tab-pending]]:text-muted-foreground'
        )
      }
    })
  })

  describe('on a coarse pointer', () => {
    let matchMedia: ReturnType<typeof installMatchMedia>
    beforeEach(() => {
      matchMedia = installMatchMedia({ '(pointer: coarse)': true })
    })
    afterEach(() => {
      matchMedia.restore()
    })

    it('never prefetches a tab on viewport entry, and prefetches the tab a touch lands on', async () => {
      vi.resetModules()
      const { markAtlasMapReady } = await import('@/lib/atlasMapReady')
      const { BottomTabBar: FreshTabBar } = await import('./BottomTabBar')
      mockPathname = '/atlas'
      const { rerender } = render(<FreshTabBar />)
      act(() => markAtlasMapReady())
      rerender(<FreshTabBar />)
      for (const name of ['Home', 'Shows', 'Radio']) {
        expect(screen.getByRole('link', { name })).toHaveAttribute('data-prefetch', 'false')
      }
      expect(mockRouter.prefetch).not.toHaveBeenCalled()

      fireEvent.touchStart(screen.getByRole('link', { name: 'Radio' }))

      expect(mockRouter.prefetch).toHaveBeenCalledExactlyOnceWith('/radio')
    })

    it('never prefetches a sheet row on viewport entry, and prefetches the row a touch lands on', async () => {
      mockRouter.prefetch.mockClear()
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      const graph = await screen.findByRole('link', { name: 'Graph' })
      const rows = [...document.querySelectorAll('[data-slot="sheet-content"] a')]
      expect(rows.length).toBeGreaterThan(10)
      expect(rows.every(a => a.getAttribute('data-prefetch') === 'false')).toBe(true)
      expect(mockRouter.prefetch).not.toHaveBeenCalled()

      // The open sheet's scroll lock reads the event's first changed touch.
      fireEvent.touchStart(graph, { changedTouches: [{ clientX: 0, clientY: 0 }] })

      expect(mockRouter.prefetch).toHaveBeenCalledExactlyOnceWith('/graph')
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

    // Opening the sheet mounts every destination at once, so on a fine pointer
    // every ROW opts out of Next's prefetch while the always-visible TAB links
    // keep theirs (the coarse-pointer rule is pinned above). Asserted as a
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

      // The sheet's one target="_blank" row (Substack) carries the
      // reverse-tabnabbing guard. Asserted here because these anchors are
      // already in hand and nothing else in the repo pins it — the mock added
      // for the prefetch assertion is what made it observable at all.
      const external = rows.find(a => a.getAttribute('target') === '_blank')
      expect(external).toBeDefined()
      expect(external).toHaveAttribute('rel', 'noopener noreferrer')
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

    // PSY-1818: the flip and the theme="system" regression it used to re-pin
    // here belong to useThemeToggle, covered once in mode-toggle.test.tsx.
    // This sheet's own contract is that the row is present, is labelled by the
    // ACTION, and is wired to that hook.
    it('carries the theme toggle (migrated from the retired hamburger sheet)', async () => {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Browse' }))
      await user.click(await screen.findByRole('button', { name: 'Light mode' }))
      expect(mockSetTheme).toHaveBeenCalledWith('light')
    })
  })

  describe('Account tab', () => {
    it('opens a signed-out sheet whose Sign in row links back to the page the viewer is on', async () => {
      mockPathname = '/artists/calexico'
      const user = userEvent.setup()
      render(<BottomTabBar />)
      expect(screen.queryByRole('link', { name: 'Account' })).not.toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: 'Account' }))
      const sheet = await screen.findByRole('dialog', { name: 'Account' })
      expect(within(sheet).getByRole('link', { name: 'Sign in' })).toHaveAttribute(
        'href',
        '/auth?returnTo=%2Fartists%2Fcalexico'
      )
      // The sheet names no viewer: no subtitle, no account destinations.
      expect(within(sheet).queryByText(/@/)).not.toBeInTheDocument()
      expect(within(sheet).queryByRole('link', { name: 'Notifications' })).not.toBeInTheDocument()
      expect(within(sheet).queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument()
    })

    it('lights up on /auth when anonymous', () => {
      mockPathname = '/auth'
      render(<BottomTabBar />)
      expect(screen.getByRole('button', { name: 'Account' })).toHaveAttribute('aria-current', 'page')
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

    // PSY-1986. Two pending windows that differ only in `isLoading`: the
    // profile is in flight (true), or a non-definitive failure (5xx, 429,
    // network, 403) has left the query errored without settling the viewer
    // (false). The cell opens the signed-out sheet in both, so a gate written
    // on `isLoading` fails one window or the other.
    it.each([
      ['while the profile is in flight', true],
      ['after the profile failed without settling', false],
    ])('opens the signed-out sheet %s', async (_label, isLoading) => {
      mockPathname = '/auth'
      mockAuthContext.mockReturnValue(authFixture({ authStatus: 'pending', isLoading }))
      const user = userEvent.setup()
      render(<BottomTabBar />)
      const tab = screen.getByRole('button', { name: 'Account' })
      // `accountActive` falls to `isActive('/auth')` on this arm.
      expect(tab).toHaveAttribute('aria-current', 'page')
      await user.click(tab)
      const sheet = await screen.findByRole('dialog', { name: 'Account' })
      // Bare: `buildAuthHref` omits a destination the auth page would discard,
      // and the viewer is already on `/auth`.
      expect(within(sheet).getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/auth')
      expect(within(sheet).queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument()
    })

    // A logout in flight is 'authenticated' with `isLoading` true: the profile
    // payload is still cached, so the context still names this viewer until
    // the mutation resolves and clears it. The old `isLoading` gate went inert
    // here; the cell now keeps the sheet, which is what the desktop bar has
    // always done in the same window.
    it('keeps the signed-in account sheet while a logout is in flight', async () => {
      mockAuthContext.mockReturnValue(
        authFixture({
          user: { email: 'user@test.com', is_admin: false },
          authStatus: 'authenticated',
          isLoading: true,
        })
      )
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Account' }))
      const sheet = await screen.findByRole('dialog', { name: 'Account' })
      expect(within(sheet).getByRole('button', { name: 'Sign out' })).toBeInTheDocument()
      expect(within(sheet).queryByRole('link', { name: 'Sign in' })).not.toBeInTheDocument()
    })

    // The pending and settled-anonymous cells are the SAME markup, which is
    // what keeps the tab row from changing shape when a pending read settles
    // to anonymous. Two things are compared, because either alone passes a
    // reflow: the cell's whole subtree (a spacer or spinner rendered beside
    // the trigger would differ) and the grid's child count (a sixth child in a
    // `grid-cols-5` row wraps Account outside the fixed bar height, the
    // PSY-1820 failure). Account is the grid's last child in both arms. The
    // trigger's aria-controls is a per-mount generated id, so it is masked.
    it('renders the pending cell identically to the settled-anonymous cell', () => {
      const cellOf = (authStatus: AuthStatus) => {
        mockAuthContext.mockReturnValue(authFixture({ authStatus }))
        const { container, unmount } = render(<BottomTabBar />)
        const grid = container.querySelector(
          'nav[aria-label="Mobile navigation"] > div'
        )
        const shape = {
          gridChildren: grid?.children.length,
          account: grid?.lastElementChild?.outerHTML.replace(
            /aria-controls="[^"]*"/,
            'aria-controls="(id)"'
          ),
        }
        unmount()
        return shape
      }
      const pending = cellOf('pending')
      expect(pending.gridChildren).toBe(primaryTabs.length + 2)
      expect(pending.account).toMatch(/^<button[^>]*aria-haspopup="dialog"/)
      expect(pending).toEqual(cellOf('anonymous'))
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

  // The legal block (Figma Product Designs 1957:6 / 1957:46): after Sign out,
  // a separator and three quiet rows, from navData's legalSheetRows.
  describe('Account sheet legal rows', () => {
    async function openAccountSheet() {
      authedAs({ email: 'user@test.com', is_admin: false })
      return openSheet()
    }

    async function openSheet() {
      const user = userEvent.setup()
      render(<BottomTabBar />)
      await user.click(screen.getByRole('button', { name: 'Account' }))
      const sheet = await screen.findByRole('dialog', { name: 'Account' })
      return { user, sheet }
    }

    const legalRow = (sheet: HTMLElement, row: (typeof legalSheetRows)[number]) =>
      within(sheet).getByRole(row.kind === 'link' ? 'link' : 'button', { name: row.label })

    it('renders Cookie preferences, Privacy and Terms after Sign out, in list order', async () => {
      const { sheet } = await openAccountSheet()
      const rows = legalSheetRows.map(row => legalRow(sheet, row))
      expect(rows.map(r => r.textContent)).toEqual([
        'Cookie preferences',
        'Privacy',
        'Terms',
      ])
      const sequence = [within(sheet).getByRole('button', { name: 'Sign out' }), ...rows]
      for (let i = 1; i < sequence.length; i++) {
        expect(
          sequence[i - 1].compareDocumentPosition(sequence[i]) &
            Node.DOCUMENT_POSITION_FOLLOWING
        ).toBeTruthy()
      }
    })

    // The board's quiet treatment: a separator of its own above the block,
    // 36px muted rows, a trailing chevron as the only glyph.
    it('renders the block as quiet rows under their own separator', async () => {
      const { sheet } = await openAccountSheet()
      const rows = legalSheetRows.map(row => legalRow(sheet, row))
      const separator = (rows[0].closest('[data-slot]') ?? rows[0]).previousElementSibling
      expect(separator).toHaveClass('border-t')
      expect(separator?.textContent).toBe('')
      for (const row of rows) {
        expect(row).toHaveClass('h-9', 'text-sm', 'text-muted-foreground')
        const glyphs = row.querySelectorAll('svg')
        expect(glyphs).toHaveLength(1)
        expect(row.lastElementChild).toBe(glyphs[0])
      }
    })

    it('links Privacy to /privacy and Terms to /terms without prefetching', async () => {
      const { sheet } = await openAccountSheet()
      const privacy = within(sheet).getByRole('link', { name: 'Privacy' })
      const terms = within(sheet).getByRole('link', { name: 'Terms' })
      expect(privacy).toHaveAttribute('href', '/privacy')
      expect(terms).toHaveAttribute('href', '/terms')
      expect(privacy).toHaveAttribute('data-prefetch', 'false')
      expect(terms).toHaveAttribute('data-prefetch', 'false')
    })

    it('makes Cookie preferences a button that opens the preferences and closes the sheet', async () => {
      const { user, sheet } = await openAccountSheet()
      const row = within(sheet).getByRole('button', { name: 'Cookie preferences' })
      expect(row).toHaveAttribute('type', 'button')
      await user.click(row)
      expect(mockOpenPreferences).toHaveBeenCalledTimes(1)
      await waitFor(() =>
        expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
      )
    })

    it('closes the sheet when Privacy is tapped', async () => {
      const { user, sheet } = await openAccountSheet()
      await user.click(within(sheet).getByRole('link', { name: 'Privacy' }))
      await waitFor(() =>
        expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
      )
    })

    // A viewer not settled as signed in gets the same block under the Sign
    // in row, so every viewer reaches it from the tab bar.
    it.each<AuthStatus>(['anonymous', 'pending'])(
      'renders the rows under Sign in and its own separator when %s',
      async authStatus => {
        mockAuthContext.mockReturnValue(authFixture({ authStatus }))
        const { user, sheet } = await openSheet()
        const rows = legalSheetRows.map(row => legalRow(sheet, row))
        const signIn = within(sheet).getByRole('link', { name: 'Sign in' })
        const separator = (rows[0].closest('[data-slot]') ?? rows[0]).previousElementSibling
        expect(separator).toHaveClass('border-t')
        expect(
          signIn.compareDocumentPosition(separator!) & Node.DOCUMENT_POSITION_FOLLOWING
        ).toBeTruthy()
        await user.click(within(sheet).getByRole('button', { name: 'Cookie preferences' }))
        expect(mockOpenPreferences).toHaveBeenCalledTimes(1)
        await waitFor(() =>
          expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
        )
      }
    )

    // The rows are route-independent: the routes that hide the site footer
    // below `xl` (footerVisibility.ts) get them exactly as every other route.
    it.each(['/', '/atlas', '/shows', '/privacy', '/admin', '/users/me'])(
      'renders the rows on %s',
      async pathname => {
        mockPathname = pathname
        const { sheet } = await openAccountSheet()
        for (const row of legalSheetRows) {
          expect(legalRow(sheet, row)).toBeInTheDocument()
        }
      }
    )
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

    // The signed-out arm is badge-less by construction, not just because the hook
    // happens to return 0, so force a non-zero count and assert nothing
    // renders, for both statuses that reach it.
    it('never badges an anonymous visitor', () => {
      mockUnreadCount.mockReturnValue(5)
      render(<BottomTabBar />)
      expect(screen.queryByTestId('unread-count-badge')).not.toBeInTheDocument()
    })

    it('never badges the Account tab while auth is unsettled', () => {
      mockAuthContext.mockReturnValue(authFixture({ authStatus: 'pending' }))
      mockUnreadCount.mockReturnValue(5)
      render(<BottomTabBar />)
      expect(screen.queryByTestId('unread-count-badge')).not.toBeInTheDocument()
    })
  })
})
