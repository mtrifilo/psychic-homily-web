import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Sidebar, sidebarGroups } from './Sidebar'

const mockPathname = vi.fn(() => '/shows')
let mockSearchParams = new URLSearchParams()
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname(),
  useSearchParams: () => mockSearchParams,
}))

// Admin nav counts are wired in the admin tests below; default to zeros so the
// public-nav tests don't need a QueryClientProvider.
const mockNavCounts = vi.fn(() => ({
  moderation: 0,
  pendingShows: 0,
  unverifiedVenues: 0,
  reports: 0,
}))
vi.mock('@/lib/hooks/admin/useAdminNavCounts', () => ({
  useAdminNavCounts: () => mockNavCounts(),
}))

// Return type widened so individual tests can override `user`/`isAuthenticated`
// without TS narrowing from the default-null literal.
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
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

describe('sidebarGroups', () => {
  it('has Discover and Community groups', () => {
    expect(sidebarGroups.map(g => g.label)).toEqual(['Discover', 'Community'])
  })

  it('Discover contains Shows, Festivals, Artists, Venues, Explore, Releases, Labels, Tags, Scenes, Collections, Charts, Radio', () => {
    const discover = sidebarGroups.find(g => g.label === 'Discover')!
    expect(discover.items.map(i => i.label)).toEqual(['Shows', 'Festivals', 'Artists', 'Venues', 'Explore', 'Releases', 'Labels', 'Tags', 'Scenes', 'Collections', 'Charts', 'Radio'])
  })

  it('Community contains Contribute, Requests, Blog, DJ Sets, Substack, Submit a Show, My Submissions', () => {
    const community = sidebarGroups.find(g => g.label === 'Community')!
    expect(community.items.map(i => i.label)).toEqual(['Contribute', 'Leaderboard', 'Requests', 'Blog', 'DJ Sets', 'Substack', 'Submit a Show', 'My Submissions'])
  })

  it('only Substack is marked external', () => {
    const external = sidebarGroups.flatMap(g => g.items).filter(i => i.external)
    expect(external).toHaveLength(1)
    expect(external[0].label).toBe('Substack')
  })

  it('all internal items have paths starting with /', () => {
    const internal = sidebarGroups.flatMap(g => g.items).filter(i => !i.external)
    for (const item of internal) {
      expect(item.href).toMatch(/^\//)
    }
  })

  it('all items have an icon', () => {
    for (const item of sidebarGroups.flatMap(g => g.items)) {
      expect(item.icon).toBeTruthy()
    }
  })
})

describe('Sidebar', () => {
  const onToggleCollapse = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname.mockReturnValue('/shows')
    mockSearchParams = new URLSearchParams()
    mockNavCounts.mockReturnValue({
      moderation: 0,
      pendingShows: 0,
      unverifiedVenues: 0,
      reports: 0,
    })
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
  })

  it('renders group headers when expanded', () => {
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.getByText('Discover')).toBeInTheDocument()
    expect(screen.getByText('Community')).toBeInTheDocument()
  })

  it('renders all nav item labels when expanded', () => {
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.getByText('Shows')).toBeInTheDocument()
    expect(screen.getByText('Festivals')).toBeInTheDocument()
    expect(screen.getByText('Artists')).toBeInTheDocument()
    expect(screen.getByText('Venues')).toBeInTheDocument()
    expect(screen.getByText('Blog')).toBeInTheDocument()
    expect(screen.getByText('DJ Sets')).toBeInTheDocument()
    expect(screen.getByText('Substack')).toBeInTheDocument()
    expect(screen.getByText('My Submissions')).toBeInTheDocument()
  })

  it('hides group headers when collapsed', () => {
    render(<Sidebar collapsed={true} onToggleCollapse={onToggleCollapse} />)
    expect(screen.queryByText('Discover')).not.toBeInTheDocument()
    expect(screen.queryByText('Community')).not.toBeInTheDocument()
  })

  it('hides item labels when collapsed', () => {
    render(<Sidebar collapsed={true} onToggleCollapse={onToggleCollapse} />)
    expect(screen.queryByText('Shows')).not.toBeInTheDocument()
    expect(screen.queryByText('Artists')).not.toBeInTheDocument()
  })

  it('shows "Collapse" button when expanded', () => {
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.getByText('Collapse')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Collapse sidebar' })).toBeInTheDocument()
  })

  it('shows expand button when collapsed', () => {
    render(<Sidebar collapsed={true} onToggleCollapse={onToggleCollapse} />)
    expect(screen.queryByText('Collapse')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Expand sidebar' })).toBeInTheDocument()
  })

  it('calls onToggleCollapse when collapse button clicked', async () => {
    const user = userEvent.setup()
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    await user.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
    expect(onToggleCollapse).toHaveBeenCalledOnce()
  })

  it('does not show Library/Profile when unauthenticated', () => {
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.queryByText('Library')).not.toBeInTheDocument()
    expect(screen.queryByText('Profile')).not.toBeInTheDocument()
  })

  it('shows Library/Profile when authenticated', () => {
    mockAuthContext.mockReturnValue({
      user: { email: 'test@test.com', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.getByText('Library')).toBeInTheDocument()
    expect(screen.getByText('Profile')).toBeInTheDocument()
  })

  it('does not show a standalone Collection entry in auth section', () => {
    mockAuthContext.mockReturnValue({
      user: { email: 'test@test.com', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    // Only "Collections" (plural, in Discover group) should exist — no "Collection" singular entry.
    expect(screen.queryByText('Collection')).not.toBeInTheDocument()
    expect(screen.getByText('Collections')).toBeInTheDocument()
  })

  it('does not show My Shows or Following entries', () => {
    mockAuthContext.mockReturnValue({
      user: { email: 'test@test.com', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.queryByText('My Shows')).not.toBeInTheDocument()
    expect(screen.queryByText('Following')).not.toBeInTheDocument()
  })

  it('shows Admin link for admin users', () => {
    mockAuthContext.mockReturnValue({
      user: { email: 'admin@test.com', is_admin: true },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.getByText('Admin')).toBeInTheDocument()
  })

  it('does not show Admin link for non-admin users', () => {
    mockAuthContext.mockReturnValue({
      user: { email: 'user@test.com', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.queryByText('Admin')).not.toBeInTheDocument()
  })

  it('sets target="_blank" on external links', () => {
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    const link = screen.getByText('Substack').closest('a')!
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('does not set target="_blank" on internal links', () => {
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    const link = screen.getByText('Shows').closest('a')!
    expect(link).not.toHaveAttribute('target')
  })

  // Active state: the current route should get the active class; siblings
  // should NOT. Catches regressions where every link styles as active.
  // We match on the exact active token (with surrounding whitespace) to
  // avoid colliding with hover utility `bg-sidebar-accent/50`.
  const ACTIVE_TOKEN = 'bg-sidebar-accent text-sidebar-accent-foreground'

  it('marks the current route as active when pathname matches exactly', () => {
    mockPathname.mockReturnValue('/shows')
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    const showsLink = screen.getByText('Shows').closest('a')!
    expect(showsLink.className).toContain(ACTIVE_TOKEN)
    expect(showsLink.className).not.toContain('text-sidebar-foreground/70')
  })

  it('does NOT mark sibling routes as active', () => {
    mockPathname.mockReturnValue('/shows')
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    const venuesLink = screen.getByText('Venues').closest('a')!
    expect(venuesLink.className).toContain('text-sidebar-foreground/70')
    expect(venuesLink.className).not.toContain(ACTIVE_TOKEN)
  })

  it('marks a route active for sub-paths (pathname.startsWith(href + "/"))', () => {
    mockPathname.mockReturnValue('/artists/sunn-o')
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    const artistsLink = screen.getByText('Artists').closest('a')!
    expect(artistsLink.className).toContain(ACTIVE_TOKEN)
  })

  it('does NOT mark external links as active even on a matching pathname', () => {
    mockPathname.mockReturnValue('https://psychichomily.substack.com/')
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    const substack = screen.getByText('Substack').closest('a')!
    // External items are never treated as "active" so the highlight follows
    // in-app navigation only.
    expect(substack.className).not.toContain(ACTIVE_TOKEN)
  })

  // Collapsible behavior: the collapse button is the canonical way to flip
  // state. The label flip (Collapse → expand) drives the existing tests; add
  // explicit aria-label on each branch.
  it('collapse button toggles aria-label between collapsed/expanded states', () => {
    const { rerender } = render(
      <Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />
    )
    expect(screen.getByRole('button', { name: 'Collapse sidebar' })).toBeInTheDocument()

    rerender(<Sidebar collapsed={true} onToggleCollapse={onToggleCollapse} />)
    expect(screen.getByRole('button', { name: 'Expand sidebar' })).toBeInTheDocument()
  })

  it('collapsed mode keeps nav links present (icons-only) — labels hidden', () => {
    mockPathname.mockReturnValue('/shows')
    render(<Sidebar collapsed={true} onToggleCollapse={onToggleCollapse} />)
    // The label "Shows" should NOT render in collapsed mode...
    expect(screen.queryByText('Shows')).not.toBeInTheDocument()
    // ...but the underlying nav still has the /shows link element so
    // tooltips (rendered on hover) and clickable icons still work.
    const links = document.querySelectorAll('a')
    const showsLink = Array.from(links).find(a => a.getAttribute('href') === '/shows')
    expect(showsLink).toBeTruthy()
  })

  it('collapse button calls onToggleCollapse exactly once per click', async () => {
    const user = userEvent.setup()
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    await user.click(screen.getByRole('button', { name: 'Collapse sidebar' }))
    expect(onToggleCollapse).toHaveBeenCalledTimes(1)
  })

  // Context-aware admin nav (PSY-933): under /admin the rail swaps the public
  // Discover/Community groups for the 6 grouped admin sections + "Back to site".
  describe('context-aware admin nav', () => {
    const asAdmin = () =>
      mockAuthContext.mockReturnValue({
        user: { email: 'admin@test.com', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })

    it('swaps to the admin groups (and hides public groups) for an admin in /admin', () => {
      asAdmin()
      mockPathname.mockReturnValue('/admin')
      render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
      // Admin group headers + a representative item.
      expect(screen.getByText('Moderation & Queues')).toBeInTheDocument()
      expect(screen.getByText('Catalog')).toBeInTheDocument()
      expect(screen.getByText('Insights & System')).toBeInTheDocument()
      expect(screen.getByText('Back to site')).toBeInTheDocument()
      // Public groups are gone.
      expect(screen.queryByText('Discover')).not.toBeInTheDocument()
      expect(screen.queryByText('Community')).not.toBeInTheDocument()
    })

    it('keeps the public nav for a NON-admin even at /admin (mid-redirect safety)', () => {
      mockAuthContext.mockReturnValue({
        user: { email: 'user@test.com', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockPathname.mockReturnValue('/admin')
      render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
      expect(screen.getByText('Discover')).toBeInTheDocument()
      expect(screen.queryByText('Moderation & Queues')).not.toBeInTheDocument()
    })

    it('marks the section matching ?tab= as active', () => {
      asAdmin()
      mockPathname.mockReturnValue('/admin')
      mockSearchParams = new URLSearchParams('tab=moderation')
      render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
      const moderation = screen.getByText('Moderation').closest('a')!
      expect(moderation.className).toContain(ACTIVE_TOKEN)
      const releases = screen.getByText('Releases').closest('a')!
      expect(releases.className).not.toContain(ACTIVE_TOKEN)
    })

    it('treats bare /admin (no tab) as Dashboard active', () => {
      asAdmin()
      mockPathname.mockReturnValue('/admin')
      render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
      const dashboard = screen.getByText('Dashboard').closest('a')!
      expect(dashboard.className).toContain(ACTIVE_TOKEN)
    })

    it('renders queue count badges from useAdminNavCounts (and omits zero counts)', () => {
      asAdmin()
      mockPathname.mockReturnValue('/admin')
      mockNavCounts.mockReturnValue({
        moderation: 5,
        pendingShows: 2,
        unverifiedVenues: 0, // zero → no badge
        reports: 3,
      })
      render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
      const moderation = screen.getByText('Moderation').closest('a')!
      expect(within(moderation).getByText('5')).toBeInTheDocument()
      const reports = screen.getByText('Reports').closest('a')!
      expect(within(reports).getByText('3')).toBeInTheDocument()
      // Unverified Venues has a zero count → no badge text beyond the label.
      const unverified = screen.getByText('Unverified Venues').closest('a')!
      expect(within(unverified).queryByText('0')).not.toBeInTheDocument()
    })

    it('points the back-to-site link at /', () => {
      asAdmin()
      mockPathname.mockReturnValue('/admin')
      render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
      expect(screen.getByText('Back to site').closest('a')).toHaveAttribute('href', '/')
    })

    it('keeps the public nav on standalone /admin/<section> sub-routes (rail is scoped to the tab-shell, not startsWith)', () => {
      asAdmin()
      mockPathname.mockReturnValue('/admin/featured')
      render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
      expect(screen.getByText('Discover')).toBeInTheDocument()
      expect(screen.queryByText('Moderation & Queues')).not.toBeInTheDocument()
    })
  })
})
