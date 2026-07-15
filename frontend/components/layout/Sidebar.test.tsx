import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Sidebar, sidebarGroups } from './Sidebar'

const mockPathname = vi.fn(() => '/shows')
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname(),
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

  it('Discover contains the Graph Observatory and catalog destinations', () => {
    const discover = sidebarGroups.find(g => g.label === 'Discover')!
    expect(discover.items.map(i => i.label)).toEqual(['Shows', 'Festivals', 'Artists', 'Venues', 'Graph', 'Releases', 'Labels', 'Tags', 'Scenes', 'Atlas', 'Collections', 'Charts', 'Radio'])
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
    expect(screen.queryByText('Show Submissions')).not.toBeInTheDocument()
    expect(screen.queryByText('Profile')).not.toBeInTheDocument()
  })

  it('shows Library, show submissions, and Profile when authenticated', () => {
    mockAuthContext.mockReturnValue({
      user: { email: 'test@test.com', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.getByText('Library')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Show Submissions' })).toHaveAttribute(
      'href',
      '/contribute/submissions'
    )
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
  // should NOT. We match on the exact active token (with surrounding whitespace)
  // to avoid colliding with hover utility `bg-sidebar-accent/50`.
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
    expect(substack.className).not.toContain(ACTIVE_TOKEN)
  })

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
    expect(screen.queryByText('Shows')).not.toBeInTheDocument()
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

  // Note: the global Sidebar is now purely the public Discover/Community nav.
  // The PSY-933 admin context-swap was retired in PSY-1116 — the admin area's
  // rail is owned by AdminSidebar (app/admin/layout.tsx), and SideNavShell
  // suppresses this sidebar under /admin to avoid a double rail.
  it('renders the public Discover nav even on /admin (no admin swap)', () => {
    mockAuthContext.mockReturnValue({
      user: { email: 'admin@test.com', is_admin: true },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockPathname.mockReturnValue('/admin')
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.getByText('Discover')).toBeInTheDocument()
  })
})
