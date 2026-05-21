import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminGuard from './admin-guard'

// AdminGuard is the single chokepoint applied to every admin route via
// app/admin/layout.tsx. Testing it once here covers the redirect/access
// behavior for ALL admin pages — the per-page page.test.tsx files render the
// page bodies directly (the guard lives one level up in the layout), so this
// is where the unauthenticated-redirect contract is verified.

const mockPush = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

let mockAuthState: {
  user: { is_admin?: boolean } | null
  isAuthenticated: boolean
  isLoading: boolean
}

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthState,
}))

describe('AdminGuard (shared admin route guard)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthState = { user: null, isAuthenticated: false, isLoading: false }
  })

  it('shows a loading spinner and does not redirect while auth is resolving', () => {
    mockAuthState = { user: null, isAuthenticated: false, isLoading: true }

    render(
      <AdminGuard>
        <div>protected content</div>
      </AdminGuard>
    )

    expect(screen.queryByText('protected content')).not.toBeInTheDocument()
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('redirects an unauthenticated visitor to /auth with returnTo', () => {
    mockAuthState = { user: null, isAuthenticated: false, isLoading: false }

    render(
      <AdminGuard>
        <div>protected content</div>
      </AdminGuard>
    )

    expect(mockPush).toHaveBeenCalledWith('/auth?returnTo=%2Fadmin')
    expect(screen.queryByText('protected content')).not.toBeInTheDocument()
  })

  it('shows Access Denied and redirects a non-admin authenticated user home', () => {
    mockAuthState = {
      user: { is_admin: false },
      isAuthenticated: true,
      isLoading: false,
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
      isAuthenticated: true,
      isLoading: false,
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
