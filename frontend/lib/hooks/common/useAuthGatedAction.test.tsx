import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import type { AuthStatus } from '@/lib/context/AuthContext'
import { useAuthGatedAction } from './useAuthGatedAction'

const mockPush = vi.fn()
let mockPathname = '/artists/calexico'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  usePathname: () => mockPathname,
}))

let mockAuthStatus: AuthStatus = 'authenticated'
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: mockAuthStatus,
    isAuthenticated: mockAuthStatus === 'authenticated',
  }),
}))

function setLocation(url: string) {
  window.history.replaceState({}, '', url)
}

describe('useAuthGatedAction', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname = '/artists/calexico'
    mockAuthStatus = 'authenticated'
    setLocation('/')
  })

  it('runs the action for a settled authenticated viewer', () => {
    const action = vi.fn()
    const { result } = renderHook(() => useAuthGatedAction(action))

    act(() => result.current.onClick())

    expect(action).toHaveBeenCalledTimes(1)
    expect(mockPush).not.toHaveBeenCalled()
    expect(result.current.isPending).toBe(false)
  })

  it('routes a settled-anonymous viewer to sign-in instead of acting', () => {
    mockAuthStatus = 'anonymous'
    const action = vi.fn()
    const { result } = renderHook(() => useAuthGatedAction(action))

    act(() => result.current.onClick())

    expect(action).not.toHaveBeenCalled()
    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Fartists%2Fcalexico'
    )
  })

  // The defect this hook exists to make unrepeatable: the redirect cannot tell
  // "no session" from "profile in flight", so acting on the unsettled window
  // sends a signed-in viewer to the sign-in form.
  it('neither acts nor redirects while auth is unsettled', () => {
    mockAuthStatus = 'pending'
    const action = vi.fn()
    const { result } = renderHook(() => useAuthGatedAction(action))

    act(() => result.current.onClick())

    expect(action).not.toHaveBeenCalled()
    expect(mockPush).not.toHaveBeenCalled()
    expect(result.current.isPending).toBe(true)
  })

  // The drift PSY-1985 found: four of the nine hand-rolled copies sent the
  // bare pathname, so a reader who clicked from a filtered list came back to
  // the unfiltered page.
  it('carries the query string into returnTo', () => {
    mockAuthStatus = 'anonymous'
    mockPathname = '/shows'
    setLocation('/shows?city=phoenix&when=weekend')
    const { result } = renderHook(() => useAuthGatedAction(vi.fn()))

    act(() => result.current.onClick())

    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Fshows%3Fcity%3Dphoenix%26when%3Dweekend'
    )
  })

  it('suppresses the event default and propagation before it branches', () => {
    const preventDefault = vi.fn()
    const stopPropagation = vi.fn()
    const { result } = renderHook(() => useAuthGatedAction(vi.fn()))

    act(() => result.current.onClick({ preventDefault, stopPropagation }))

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(stopPropagation).toHaveBeenCalledTimes(1)
  })

  it('hands an anonymous override the same href the default push would use', () => {
    mockAuthStatus = 'anonymous'
    mockPathname = '/shows/example'
    setLocation('/shows/example?tab=bill')
    const onAnonymous = vi.fn()
    const { result } = renderHook(() =>
      useAuthGatedAction(vi.fn(), { onAnonymous })
    )

    act(() => result.current.onClick())

    expect(onAnonymous).toHaveBeenCalledWith(
      '/auth?returnTo=%2Fshows%2Fexample%3Ftab%3Dbill'
    )
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('does not reach an anonymous override from the unsettled window', () => {
    mockAuthStatus = 'pending'
    const onAnonymous = vi.fn()
    const { result } = renderHook(() =>
      useAuthGatedAction(vi.fn(), { onAnonymous })
    )

    act(() => result.current.onClick())

    expect(onAnonymous).not.toHaveBeenCalled()
  })

  it('reports the viewer state its callers render from', () => {
    mockAuthStatus = 'anonymous'
    const { result } = renderHook(() => useAuthGatedAction(vi.fn()))

    expect(result.current.authStatus).toBe('anonymous')
    expect(result.current.isAnonymous).toBe(true)
    expect(result.current.isPending).toBe(false)
    expect(result.current.isAuthenticated).toBe(false)
  })
})
