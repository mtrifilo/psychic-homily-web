import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { AdminSidebar } from './AdminSidebar'

// AdminSidebar lazy-loads AdminSidebarNav via next/dynamic(ssr:false). Stub
// dynamic so the nav body renders synchronously as a marker that echoes the
// `collapsed` prop — AdminSidebarNav's own behavior is covered by
// AdminSidebarNav.test.tsx; here we exercise the rail chrome (collapse toggle +
// localStorage persistence).
vi.mock('next/dynamic', () => ({
  default: () =>
    function MockAdminSidebarNav({ collapsed }: { collapsed: boolean }) {
      return <div data-testid="admin-nav">{collapsed ? 'collapsed' : 'expanded'}</div>
    },
}))

describe('AdminSidebar', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('renders the admin nav body, expanded by default', () => {
    render(<AdminSidebar />)
    expect(screen.getByTestId('admin-nav')).toHaveTextContent('expanded')
    expect(screen.getByRole('button', { name: 'Collapse sidebar' })).toBeInTheDocument()
  })

  it('toggles collapsed state and persists it to localStorage', () => {
    render(<AdminSidebar />)

    fireEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }))

    expect(screen.getByRole('button', { name: 'Expand sidebar' })).toBeInTheDocument()
    expect(screen.getByTestId('admin-nav')).toHaveTextContent('collapsed')
    expect(localStorage.getItem('admin-sidebar-collapsed')).toBe('collapsed')
  })

  it('restores the collapsed preference from localStorage on mount', () => {
    localStorage.setItem('admin-sidebar-collapsed', 'collapsed')
    render(<AdminSidebar />)

    expect(screen.getByRole('button', { name: 'Expand sidebar' })).toBeInTheDocument()
    expect(screen.getByTestId('admin-nav')).toHaveTextContent('collapsed')
  })
})
