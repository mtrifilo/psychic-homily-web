import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminLayout from './layout'

// The layout is where AdminGuard is actually wired into every admin route.
// AdminGuard's own behavior is exercised in admin-guard.test.tsx; this test
// confirms the layout delegates to it (so the redirect/gate applies to all
// nested pages, not just the ones with their own page.test.tsx).
//
// It is also the mount point for both admin navs (PSY-1817), so it is where
// their access gating and their shared breakpoint invariant are asserted.

const mockPush = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

// Both the rail (AdminSidebarNav) and the drawer (AdminDrawerNav) load their
// bodies through next/dynamic; neither body is under test here. This is load
// bearing beyond chunk-splitting: AdminSidebarNav calls useSearchParams(),
// which the next/navigation mock above does not provide, so an unstubbed
// dynamic import would throw. The chrome under test is the rail and drawer
// containers, both of which are outside these lazy bodies.
vi.mock('next/dynamic', () => ({
  default: () => function DynamicNavStub() {
    return null
  },
}))

let mockAuthState: {
  user: { is_admin?: boolean } | null
  isAuthenticated: boolean
  isLoading: boolean
}

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthState,
}))

describe('AdminLayout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthState = { user: null, isAuthenticated: false, isLoading: false }
  })

  it('gates children behind AdminGuard (unauthenticated → redirect, no content)', () => {
    render(
      <AdminLayout>
        <div>admin child page</div>
      </AdminLayout>
    )

    expect(mockPush).toHaveBeenCalledWith('/auth?returnTo=%2Fadmin')
    expect(screen.queryByText('admin child page')).not.toBeInTheDocument()
    // The drawer is inside the guard, so it goes with the content.
    expect(screen.queryByRole('button', { name: 'Open admin menu' })).not.toBeInTheDocument()
  })

  // AdminGuard rejects in two structurally different ways — a bare `null` when
  // unauthenticated (above) and an Access Denied panel for a signed-in
  // non-admin, which renders a real subtree. Both have to withhold the drawer,
  // and the redirect case only covers the first, so this pins the second.
  it('withholds the drawer from a signed-in non-admin (Access Denied branch)', () => {
    mockAuthState = {
      user: { is_admin: false },
      isAuthenticated: true,
      isLoading: false,
    }

    render(
      <AdminLayout>
        <div>admin child page</div>
      </AdminLayout>
    )

    expect(screen.getByText('Access Denied')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Open admin menu' })).not.toBeInTheDocument()
    expect(screen.queryByText('admin child page')).not.toBeInTheDocument()
  })

  it('renders children when an admin is authenticated', () => {
    mockAuthState = {
      user: { is_admin: true },
      isAuthenticated: true,
      isLoading: false,
    }

    render(
      <AdminLayout>
        <div>admin child page</div>
      </AdminLayout>
    )

    expect(screen.getByText('admin child page')).toBeInTheDocument()
    expect(mockPush).not.toHaveBeenCalled()
  })

  // The drawer trigger is admin-shell chrome, not top-bar chrome: it belongs to
  // the content column, which is what puts it ahead of the page (PSY-1817).
  it('mounts the mobile admin drawer for an admin, above the page content', () => {
    mockAuthState = {
      user: { is_admin: true },
      isAuthenticated: true,
      isLoading: false,
    }

    render(
      <AdminLayout>
        <div>admin child page</div>
      </AdminLayout>
    )

    const trigger = screen.getByRole('button', { name: 'Open admin menu' })
    expect(
      trigger.compareDocumentPosition(screen.getByText('admin child page'))
        & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
  })

  // The rail and the drawer must be exact inverses, or some viewport band gets
  // both admin navs or neither. Asserting them in the SAME render is half the
  // point — a one-sided assertion in either component's own test still passes
  // when the other side's breakpoint moved.
  //
  // The other half is asserting the PROPERTY, not the two literal strings.
  // `toHaveClass('md:flex')` is a presence check, so `hidden md:flex lg:hidden`
  // would sail through it while leaving every viewport at or above `lg` with no
  // admin nav at all. So: pull every responsive display utility off each side,
  // require exactly one, and require both to name the same breakpoint.
  const displayUtilities = (el: Element | null | undefined) =>
    Array.from(el?.classList ?? []).filter(cls =>
      /^[a-z0-9]+:(flex|hidden|block|inline|inline-block|grid|contents)$/.test(cls)
    )

  it('pairs the drawer against the rail as exact breakpoint inverses', () => {
    mockAuthState = {
      user: { is_admin: true },
      isAuthenticated: true,
      isLoading: false,
    }

    const { container } = render(
      <AdminLayout>
        <div>admin child page</div>
      </AdminLayout>
    )

    const rail = container.querySelector('aside[aria-label="Admin navigation"]')
    const drawerBar = screen.getByTestId('admin-drawer-bar')

    // Base state: the rail is out until its breakpoint, the drawer bar is in.
    expect(rail).toHaveClass('hidden')
    expect(drawerBar).not.toHaveClass('hidden')

    const railSwitch = displayUtilities(rail)
    const drawerSwitch = displayUtilities(drawerBar)

    expect(railSwitch).toEqual(['md:flex'])
    expect(drawerSwitch).toEqual(['md:hidden'])

    // Restating the invariant independently of the literals above: one switch
    // each, at the same breakpoint, in opposite directions.
    const [railBreakpoint, railDisplay] = railSwitch[0].split(':')
    const [drawerBreakpoint, drawerDisplay] = drawerSwitch[0].split(':')
    expect(railBreakpoint).toBe(drawerBreakpoint)
    expect(railDisplay).not.toBe('hidden')
    expect(drawerDisplay).toBe('hidden')
  })
})
