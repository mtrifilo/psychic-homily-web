import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AdminMobileDrawer } from './AdminMobileDrawer'

// The drawer body is a dynamically-imported chunk (AdminDrawerNav, kept out of
// the bundle until the drawer is opened); stub next/dynamic so it renders a
// synchronous marker. AdminDrawerNav's own links/badges are covered by
// AdminDrawerNav.test.tsx.
vi.mock('next/dynamic', () => ({
  default: () =>
    function AdminDrawerNavStub({ onNavigate }: { onNavigate: () => void }) {
      return (
        <button data-testid="admin-drawer-nav" onClick={onNavigate}>
          nav item
        </button>
      )
    },
}))

// PSY-1817: the drawer is mounted by app/admin/layout.tsx inside AdminGuard, so
// it carries no route or auth gate of its own — it renders whenever it is
// mounted. These assert the behavior that survived the move.
describe('AdminMobileDrawer', () => {
  it('renders the trigger with no route or auth gate of its own', () => {
    render(<AdminMobileDrawer />)
    expect(screen.getByRole('button', { name: 'Open admin menu' })).toBeInTheDocument()
  })

  it('opens the drawer and shows the admin nav', async () => {
    const user = userEvent.setup()
    render(<AdminMobileDrawer />)
    await user.click(screen.getByRole('button', { name: 'Open admin menu' }))
    expect(await screen.findByTestId('admin-drawer-nav')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Admin' })).toBeInTheDocument()
  })

  it('closes the drawer when a nav item is selected', async () => {
    const user = userEvent.setup()
    render(<AdminMobileDrawer />)
    await user.click(screen.getByRole('button', { name: 'Open admin menu' }))
    await user.click(await screen.findByTestId('admin-drawer-nav'))
    expect(screen.queryByTestId('admin-drawer-nav')).not.toBeInTheDocument()
  })

  // The rail (AdminSidebar) is `hidden md:flex`; the drawer must be its exact
  // inverse so there is no band where both or neither admin nav is reachable.
  it('is hidden from `md` up, mirroring AdminSidebar’s md:flex rail', () => {
    const { container } = render(<AdminMobileDrawer />)
    expect(container.firstElementChild).toHaveClass('md:hidden')
  })
})
