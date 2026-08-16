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
  // non-admin. Both have to withhold the drawer, and only the first is covered
  // by the redirect case, so this pins the second. It replaces the deleted
  // TopBar test that used to assert the drawer's own `is_admin` check.
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

  // PSY-1817: the drawer trigger is admin-shell chrome, not top-bar chrome, so
  // it belongs to the content column and precedes the page.
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
  // both admin navs or neither. Asserting them in the SAME render is the point:
  // a one-sided assertion in either component's own test still passes when the
  // other side's breakpoint is what moved.
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
    const drawerBar = screen
      .getByRole('button', { name: 'Open admin menu' })
      .closest('div')

    expect(rail).toHaveClass('hidden', 'md:flex')
    expect(drawerBar).toHaveClass('md:hidden')
  })
})
