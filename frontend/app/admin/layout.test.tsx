import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminLayout from './layout'

// The layout is where AdminGuard is actually wired into every admin route.
// AdminGuard's own behavior is exercised in admin-guard.test.tsx; this test
// confirms the layout delegates to it (so the redirect/gate applies to all
// nested pages, not just the ones with their own page.test.tsx).
//
// PSY-1817 also made this layout the mount point for the mobile admin drawer,
// which used to hang off the global TopBar behind a `pathname === '/admin'`
// string and its own `is_admin` check. Those gates are now this file's
// structure — the route segment scopes the drawer, AdminGuard authorizes it —
// so the guard-delegation tests below double as the drawer's access tests.

const mockPush = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

// Both the rail (AdminSidebarNav) and the drawer (AdminDrawerNav) load their
// bodies through next/dynamic; neither body is under test here.
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
    // The drawer is inside the guard, so a non-admin never gets admin chrome.
    expect(screen.queryByRole('button', { name: 'Open admin menu' })).not.toBeInTheDocument()
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

  // PSY-1817: the drawer trigger is admin-shell chrome, not top-bar chrome.
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
    expect(trigger).toBeInTheDocument()
    expect(
      trigger.compareDocumentPosition(screen.getByText('admin child page'))
        & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })
})
