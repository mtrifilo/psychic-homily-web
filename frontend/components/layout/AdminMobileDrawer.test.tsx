import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AdminMobileDrawer } from './AdminMobileDrawer'

// The drawer body is a dynamically-imported chunk (AdminDrawerNav, loaded only
// when the drawer opens); stub next/dynamic so it renders a synchronous marker.
// AdminDrawerNav's own links and badges are covered by AdminDrawerNav.test.tsx.
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

// The drawer's placement, breakpoint and access gating all belong to
// app/admin/layout.tsx and are asserted there. What is left here is the
// component's own behavior: it renders unconditionally, opens, and closes on
// selection.
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
})
