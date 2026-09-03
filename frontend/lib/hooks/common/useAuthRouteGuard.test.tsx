import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import type { AuthStatus } from '@/lib/context/AuthContext'
import { useAuthRouteGuard } from './useAuthRouteGuard'

const mockPush = vi.fn()
const mockReplace = vi.fn()
const mockRedirect = vi.fn()
let mockPathname = '/library'

// One router object across renders, as Next's own `useRouter` returns, so the
// effect's dependency list behaves the way it does in the app.
const mockRouter = { push: mockPush, replace: mockReplace }

vi.mock('next/navigation', () => ({
  useRouter: () => mockRouter,
  usePathname: () => mockPathname,
  // Non-throwing, so a gated render can be asserted on. In production this
  // throws and the caller never sees the verdict.
  redirect: (href: string) => mockRedirect(href),
}))

let mockAuthStatus: AuthStatus = 'authenticated'
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: mockAuthStatus,
    isAuthenticated: mockAuthStatus === 'authenticated',
  }),
}))

const navigations = ['push', 'replace', 'redirect'] as const

describe('useAuthRouteGuard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname = '/library'
    mockAuthStatus = 'authenticated'
    window.history.replaceState({}, '', '/')
  })

  // The rule this hook owns, asserted for every navigation mode at once: a
  // guard may redirect only on a SETTLED anonymous answer. 'pending' includes
  // a signed-in viewer whose profile fetch failed on a 5xx, a 429, a 403 or a
  // network error, and redirecting there loses the page they were on.
  it.each(navigations)(
    'renders the loading state and issues no navigation while pending (%s mode)',
    navigation => {
      mockAuthStatus = 'pending'

      const { result } = renderHook(() => useAuthRouteGuard(navigation))

      expect(result.current).toBe('loading')
      expect(mockPush).not.toHaveBeenCalled()
      expect(mockReplace).not.toHaveBeenCalled()
      expect(mockRedirect).not.toHaveBeenCalled()
    }
  )

  it.each(navigations)(
    'renders the page for a settled authenticated viewer (%s mode)',
    navigation => {
      const { result } = renderHook(() => useAuthRouteGuard(navigation))

      expect(result.current).toBe('ready')
      expect(mockPush).not.toHaveBeenCalled()
      expect(mockReplace).not.toHaveBeenCalled()
      expect(mockRedirect).not.toHaveBeenCalled()
    }
  )

  it('pushes a settled-anonymous viewer to sign-in by default', () => {
    mockAuthStatus = 'anonymous'

    const { result } = renderHook(() => useAuthRouteGuard())

    expect(result.current).toBe('blank')
    expect(mockPush).toHaveBeenCalledWith('/auth?returnTo=%2Flibrary')
    expect(mockReplace).not.toHaveBeenCalled()
  })

  it('replaces rather than pushes when the caller asks for it', () => {
    mockAuthStatus = 'anonymous'

    renderHook(() => useAuthRouteGuard('replace'))

    expect(mockReplace).toHaveBeenCalledWith('/auth?returnTo=%2Flibrary')
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('redirects during render when the caller asks for it', () => {
    mockAuthStatus = 'anonymous'

    renderHook(() => useAuthRouteGuard('redirect'))

    expect(mockRedirect).toHaveBeenCalledWith('/auth?returnTo=%2Flibrary')
    expect(mockPush).not.toHaveBeenCalled()
    expect(mockReplace).not.toHaveBeenCalled()
  })

  // One destination formula, shared with `useAuthGatedAction`: the query
  // string comes back with the reader, so a filtered list returns filtered.
  it('carries the query string into returnTo', () => {
    mockAuthStatus = 'anonymous'
    mockPathname = '/contribute/submissions'
    window.history.replaceState({}, '', '/contribute/submissions?page=2')

    renderHook(() => useAuthRouteGuard())

    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Fcontribute%2Fsubmissions%3Fpage%3D2'
    )
  })

  it('navigates once for a viewer whose status does not change', () => {
    mockAuthStatus = 'anonymous'

    const { rerender } = renderHook(() => useAuthRouteGuard())
    rerender()
    rerender()

    expect(mockPush).toHaveBeenCalledTimes(1)
  })
})
