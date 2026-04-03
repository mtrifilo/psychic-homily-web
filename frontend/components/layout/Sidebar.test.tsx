import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Sidebar, sidebarGroups } from './Sidebar'

const mockPathname = vi.fn(() => '/shows')
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname(),
}))

const mockAuthContext = vi.fn(() => ({
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

  it('Discover contains Shows, Festivals, Artists, Venues, Releases, Labels, Tags, Crates, Radio', () => {
    const discover = sidebarGroups.find(g => g.label === 'Discover')!
    expect(discover.items.map(i => i.label)).toEqual(['Shows', 'Festivals', 'Artists', 'Venues', 'Releases', 'Labels', 'Tags', 'Scenes', 'Crates', 'Charts', 'Radio'])
  })

  it('Community contains Contribute, Requests, Blog, DJ Sets, Substack, Submissions', () => {
    const community = sidebarGroups.find(g => g.label === 'Community')!
    expect(community.items.map(i => i.label)).toEqual(['Contribute', 'Leaderboard', 'Requests', 'Blog', 'DJ Sets', 'Substack', 'Submissions'])
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
    expect(screen.getByText('Submissions')).toBeInTheDocument()
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

  it('does not show Library/Collection/Settings when unauthenticated', () => {
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.queryByText('Library')).not.toBeInTheDocument()
    expect(screen.queryByText('Collection')).not.toBeInTheDocument()
    expect(screen.queryByText('Settings')).not.toBeInTheDocument()
  })

  it('shows Library/Collection/Settings when authenticated', () => {
    mockAuthContext.mockReturnValue({
      user: { email: 'test@test.com', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    render(<Sidebar collapsed={false} onToggleCollapse={onToggleCollapse} />)
    expect(screen.getByText('Library')).toBeInTheDocument()
    expect(screen.getByText('Collection')).toBeInTheDocument()
    expect(screen.getByText('Settings')).toBeInTheDocument()
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
})
