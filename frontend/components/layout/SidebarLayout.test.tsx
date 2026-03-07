import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SidebarLayout } from './SidebarLayout'

vi.mock('./TopBar', () => ({
  TopBar: ({ mobileOpen, onMobileOpenChange }: { mobileOpen: boolean; onMobileOpenChange: (open: boolean) => void }) => (
    <div data-testid="topbar" data-mobile-open={mobileOpen}>
      <button onClick={() => onMobileOpenChange(!mobileOpen)}>menu</button>
    </div>
  ),
}))

vi.mock('./Sidebar', () => ({
  Sidebar: ({ collapsed, onToggleCollapse }: { collapsed: boolean; onToggleCollapse: () => void }) => (
    <div data-testid="sidebar" data-collapsed={collapsed}>
      <button onClick={onToggleCollapse} data-testid="toggle-collapse">toggle</button>
    </div>
  ),
}))

describe('SidebarLayout', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('renders children', () => {
    render(
      <SidebarLayout>
        <div>test content</div>
      </SidebarLayout>
    )
    expect(screen.getByText('test content')).toBeInTheDocument()
  })

  it('renders TopBar and Sidebar', () => {
    render(
      <SidebarLayout>
        <div>content</div>
      </SidebarLayout>
    )
    expect(screen.getByTestId('topbar')).toBeInTheDocument()
    expect(screen.getByTestId('sidebar')).toBeInTheDocument()
  })

  it('defaults to expanded sidebar', () => {
    render(
      <SidebarLayout>
        <div>content</div>
      </SidebarLayout>
    )
    expect(screen.getByTestId('sidebar')).toHaveAttribute('data-collapsed', 'false')
  })

  it('reads collapsed state from localStorage on mount', async () => {
    localStorage.setItem('sidebar-collapsed', 'true')
    await act(async () => {
      render(
        <SidebarLayout>
          <div>content</div>
        </SidebarLayout>
      )
    })
    expect(screen.getByTestId('sidebar')).toHaveAttribute('data-collapsed', 'true')
  })

  it('persists collapsed state to localStorage on toggle', async () => {
    const user = userEvent.setup()
    render(
      <SidebarLayout>
        <div>content</div>
      </SidebarLayout>
    )

    await user.click(screen.getByTestId('toggle-collapse'))
    expect(localStorage.getItem('sidebar-collapsed')).toBe('true')

    await user.click(screen.getByTestId('toggle-collapse'))
    expect(localStorage.getItem('sidebar-collapsed')).toBe('false')
  })

  it('toggles mobile menu state', async () => {
    const user = userEvent.setup()
    render(
      <SidebarLayout>
        <div>content</div>
      </SidebarLayout>
    )

    expect(screen.getByTestId('topbar')).toHaveAttribute('data-mobile-open', 'false')
    await user.click(screen.getByText('menu'))
    expect(screen.getByTestId('topbar')).toHaveAttribute('data-mobile-open', 'true')
  })
})
