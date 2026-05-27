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

vi.mock('./CommandPalette', () => ({
  CommandPalette: () => <div data-testid="command-palette" />,
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

  it('mounts the CommandPalette so global Cmd+K works', () => {
    render(
      <SidebarLayout>
        <div>content</div>
      </SidebarLayout>
    )
    expect(screen.getByTestId('command-palette')).toBeInTheDocument()
  })

  it('mobile menu closes when toggled again (close branch)', async () => {
    const user = userEvent.setup()
    render(
      <SidebarLayout>
        <div>content</div>
      </SidebarLayout>
    )

    await user.click(screen.getByText('menu'))
    expect(screen.getByTestId('topbar')).toHaveAttribute('data-mobile-open', 'true')
    await user.click(screen.getByText('menu'))
    expect(screen.getByTestId('topbar')).toHaveAttribute('data-mobile-open', 'false')
  })

  it('persists the collapsed=true state after a second toggle round-trip', async () => {
    const user = userEvent.setup()
    render(
      <SidebarLayout>
        <div>content</div>
      </SidebarLayout>
    )

    // Default false → collapse → expand → collapse again — localStorage
    // must reflect the FINAL state, not the first.
    await user.click(screen.getByTestId('toggle-collapse'))
    await user.click(screen.getByTestId('toggle-collapse'))
    await user.click(screen.getByTestId('toggle-collapse'))
    expect(localStorage.getItem('sidebar-collapsed')).toBe('true')
    expect(screen.getByTestId('sidebar')).toHaveAttribute('data-collapsed', 'true')
  })

  it('treats localStorage value other than "true" as expanded (defensive)', async () => {
    localStorage.setItem('sidebar-collapsed', 'maybe')
    await act(async () => {
      render(
        <SidebarLayout>
          <div>content</div>
        </SidebarLayout>
      )
    })
    // Anything except the string "true" leaves the default expanded state.
    expect(screen.getByTestId('sidebar')).toHaveAttribute('data-collapsed', 'false')
  })

  it('opens the command palette in response to the openCommandPalette custom event', async () => {
    // SidebarLayout's TopBar invokes openCommandPalette() on search click,
    // which dispatches a window event the palette listens for. Verify the
    // event fires when handleSearchClick runs.
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')
    const user = userEvent.setup()

    // Re-mock TopBar so onSearchClick is exposed as a button we can click.
    // Use try/finally so a failed assertion doesn't leak the doMock to
    // sibling tests in the same vitest worker (the file-level vi.mock would
    // otherwise stay overridden for any future dynamic import of TopBar).
    vi.doMock('./TopBar', () => ({
      TopBar: ({ onSearchClick }: { onSearchClick?: () => void }) => (
        <button data-testid="topbar-search" onClick={onSearchClick}>
          search
        </button>
      ),
    }))
    try {
      vi.resetModules()
      const { SidebarLayout: FreshLayout } = await import('./SidebarLayout')

      render(
        <FreshLayout>
          <div>content</div>
        </FreshLayout>
      )

      await user.click(screen.getByTestId('topbar-search'))

      // openCommandPalette() dispatches a custom event named "open-command-palette"
      const event = dispatchSpy.mock.calls.find(
        call => call[0] instanceof Event && (call[0] as Event).type === 'open-command-palette'
      )
      expect(event).toBeDefined()
    } finally {
      dispatchSpy.mockRestore()
      vi.doUnmock('./TopBar')
    }
  })
})
