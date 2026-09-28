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
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () => deriveMockAuthSignals({ authStatus: mockAuthStatus }),
  }
})

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
    const { result } = renderHook(() => useAuthGatedAction('save', action))

    act(() => result.current.onClick())

    expect(action).toHaveBeenCalledTimes(1)
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('routes a settled-anonymous viewer to sign-in, naming the intent, instead of acting', () => {
    mockAuthStatus = 'anonymous'
    const action = vi.fn()
    const { result } = renderHook(() => useAuthGatedAction('save', action))

    act(() => result.current.onClick())

    expect(action).not.toHaveBeenCalled()
    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Fartists%2Fcalexico&intent=save'
    )
  })

  // The home page is a destination `buildAuthHref` drops, so the intent is the
  // only parameter left and still has to arrive.
  it('keeps the intent when there is no returnTo to carry', () => {
    mockAuthStatus = 'anonymous'
    mockPathname = '/'
    const { result } = renderHook(() => useAuthGatedAction('save', vi.fn()))

    act(() => result.current.onClick())

    expect(mockPush).toHaveBeenCalledWith('/auth?intent=save')
  })

  // The defect this hook exists to make unrepeatable: the redirect cannot tell
  // "no session" from "profile in flight", so acting on the unsettled window
  // sends a signed-in viewer to the sign-in form.
  it('neither acts nor redirects while auth is unsettled', () => {
    mockAuthStatus = 'pending'
    const action = vi.fn()
    const { result } = renderHook(() => useAuthGatedAction('save', action))

    act(() => result.current.onClick())

    expect(action).not.toHaveBeenCalled()
    expect(mockPush).not.toHaveBeenCalled()
  })

  // The drift PSY-1985 found: four of the nine hand-rolled copies sent the
  // bare pathname, so a reader who clicked from a filtered list came back to
  // the unfiltered page.
  it('carries the query string into returnTo', () => {
    mockAuthStatus = 'anonymous'
    mockPathname = '/shows'
    setLocation('/shows?city=phoenix&when=weekend')
    const { result } = renderHook(() => useAuthGatedAction('save', vi.fn()))

    act(() => result.current.onClick())

    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Fshows%3Fcity%3Dphoenix%26when%3Dweekend&intent=save'
    )
  })

  it('suppresses the event default and propagation before it branches', () => {
    const preventDefault = vi.fn()
    const stopPropagation = vi.fn()
    const { result } = renderHook(() => useAuthGatedAction('save', vi.fn()))

    act(() => result.current.onClick({ preventDefault, stopPropagation }))

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(stopPropagation).toHaveBeenCalledTimes(1)
  })

  // A dialog offers Sign in and Create account as separate buttons: both carry
  // the same returnTo, and only the Create account one names the intent.
  it('hands an anonymous override both hrefs, the intent on Create account only', () => {
    mockAuthStatus = 'anonymous'
    mockPathname = '/shows/example'
    setLocation('/shows/example?tab=bill')
    const onAnonymous = vi.fn()
    const { result } = renderHook(() =>
      useAuthGatedAction('report', vi.fn(), onAnonymous)
    )

    act(() => result.current.onClick())

    expect(onAnonymous).toHaveBeenCalledWith({
      signInHref: '/auth?returnTo=%2Fshows%2Fexample%3Ftab%3Dbill',
      createAccountHref:
        '/auth?returnTo=%2Fshows%2Fexample%3Ftab%3Dbill&intent=report',
    })
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('does not reach an anonymous override from the unsettled window', () => {
    mockAuthStatus = 'pending'
    const onAnonymous = vi.fn()
    const { result } = renderHook(() =>
      useAuthGatedAction('report', vi.fn(), onAnonymous)
    )

    act(() => result.current.onClick())

    expect(onAnonymous).not.toHaveBeenCalled()
  })

  // The render-time recovery link a control shows after a 401 is for a viewer
  // who had a session, so it opens on Sign in.
  it('builds a sign-in href for here that names no intent', () => {
    mockPathname = '/venues/rebel-lounge'
    setLocation('/venues/rebel-lounge')
    const { result } = renderHook(() => useAuthGatedAction('confirm', vi.fn()))

    expect(result.current.buildAuthHrefForHere()).toBe(
      '/auth?returnTo=%2Fvenues%2Frebel-lounge'
    )
  })
})
