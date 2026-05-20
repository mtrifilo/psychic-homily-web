import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminLayout from './layout'

// The layout is where AdminGuard is actually wired into every admin route.
// AdminGuard's own behavior is exercised in admin-guard.test.tsx; this test
// confirms the layout delegates to it (so the redirect/gate applies to all
// nested pages, not just the ones with their own page.test.tsx).

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
})
