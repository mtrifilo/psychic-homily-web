import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SideNavShell } from './SideNavShell'

const mockPathname = vi.fn(() => '/shows')
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname(),
}))

vi.mock('./Sidebar', () => ({
  Sidebar: ({ collapsed }: { collapsed: boolean }) => (
    <div data-testid="global-sidebar" data-collapsed={collapsed} />
  ),
}))

describe('SideNavShell', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
    mockPathname.mockReturnValue('/shows')
  })

  it('renders the global sidebar + children on a normal page', () => {
    render(
      <SideNavShell>
        <div>page content</div>
      </SideNavShell>
    )
    expect(screen.getByTestId('global-sidebar')).toBeInTheDocument()
    expect(screen.getByText('page content')).toBeInTheDocument()
  })

  it('suppresses the global sidebar under /admin (admin owns its own rail) but still renders content', () => {
    mockPathname.mockReturnValue('/admin')
    render(
      <SideNavShell>
        <div>admin content</div>
      </SideNavShell>
    )
    expect(screen.queryByTestId('global-sidebar')).not.toBeInTheDocument()
    expect(screen.getByText('admin content')).toBeInTheDocument()
  })

  it('suppresses the global sidebar on /admin sub-routes too', () => {
    mockPathname.mockReturnValue('/admin/users')
    render(
      <SideNavShell>
        <div>x</div>
      </SideNavShell>
    )
    expect(screen.queryByTestId('global-sidebar')).not.toBeInTheDocument()
  })

  it('passes the persisted collapsed preference through to the sidebar', () => {
    localStorage.setItem('sidebar-collapsed', 'collapsed')
    render(
      <SideNavShell>
        <div>x</div>
      </SideNavShell>
    )
    expect(screen.getByTestId('global-sidebar')).toHaveAttribute('data-collapsed', 'true')
  })
})
