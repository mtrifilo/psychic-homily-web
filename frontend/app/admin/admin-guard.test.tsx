import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminGuard from './admin-guard'

// AdminGuard is the single chokepoint applied to every admin route via
// app/admin/layout.tsx. Testing it once here covers the redirect/access
// behavior for ALL admin pages — the per-page page.test.tsx files render the
// page bodies directly (the guard lives one level up in the layout), so this
// is where the unauthenticated-redirect contract is verified.

const mockPush = vi.fn()
let mockPathname = '/admin/reports'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  usePathname: () => mockPathname,
  redirect: vi.fn(),
}))

// `authStatus` is the setting; `isAuthenticated` derives from it, so a case
// cannot describe a viewer whose two auth signals disagree, and the unsettled
// window is expressible (it is not, when `isAuthenticated` is the input).
let mockAuthState: {
  user: { is_admin?: boolean } | null
  authStatus: 'pending' | 'anonymous' | 'authenticated'
}

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    ...mockAuthState,
    isAuthenticated: mockAuthState.authStatus === 'authenticated',
    isLoading: mockAuthState.authStatus === 'pending',
  }),
}))

describe('AdminGuard (shared admin route guard)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname = '/admin/reports'
    mockAuthState = { user: null, authStatus: 'anonymous' }
  })

  it('shows a loading spinner and does not redirect while auth is unsettled', () => {
    // The window this guard exists to survive: a signed-in viewer whose
    // profile fetch failed on a non-definitive error reads 'pending', and
    // redirecting there dumps them on the sign-in form and loses the page.
    mockAuthState = { user: null, authStatus: 'pending' }

    render(
      <AdminGuard>
        <div>protected content</div>
      </AdminGuard>
    )

    expect(screen.queryByText('protected content')).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Access Denied' })).toBeNull()
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('redirects a settled-anonymous visitor to /auth with the page they asked for', () => {
    mockAuthState = { user: null, authStatus: 'anonymous' }

    render(
      <AdminGuard>
        <div>protected content</div>
      </AdminGuard>
    )

    expect(mockPush).toHaveBeenCalledWith('/auth?returnTo=%2Fadmin%2Freports')
    expect(screen.queryByText('protected content')).not.toBeInTheDocument()
  })

  it('shows Access Denied and redirects a non-admin authenticated user home', () => {
    mockAuthState = {
      user: { is_admin: false },
      authStatus: 'authenticated',
    }

    render(
      <AdminGuard>
        <div>protected content</div>
      </AdminGuard>
    )

    expect(mockPush).toHaveBeenCalledWith('/')
    expect(
      screen.getByRole('heading', { name: 'Access Denied' })
    ).toBeInTheDocument()
    expect(screen.queryByText('protected content')).not.toBeInTheDocument()
  })

  it('renders children for an authenticated admin without redirecting', () => {
    mockAuthState = {
      user: { is_admin: true },
      authStatus: 'authenticated',
    }

    render(
      <AdminGuard>
        <div>protected content</div>
      </AdminGuard>
    )

    expect(screen.getByText('protected content')).toBeInTheDocument()
    expect(mockPush).not.toHaveBeenCalled()
  })
})
