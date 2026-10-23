import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'

import { useAuthArrivalParams, withoutBannerParams } from './auth-arrival-params'

// The App Router syncs useSearchParams with window.history.replaceState. These
// tests stand in for it: `live` is what useSearchParams would return, and
// `syncWithAddressBar` rerenders with the URL replaceState left behind.
function renderArrival(query: string) {
  window.history.replaceState(null, '', `/auth${query ? `?${query}` : ''}`)
  const view = renderHook(
    ({ live }: { live: URLSearchParams }) => useAuthArrivalParams(live),
    { initialProps: { live: new URLSearchParams(query) } }
  )
  return {
    ...view,
    navigate(nextQuery: string) {
      window.history.replaceState(null, '', `/auth${nextQuery ? `?${nextQuery}` : ''}`)
      view.rerender({ live: new URLSearchParams(nextQuery) })
    },
    syncWithAddressBar() {
      view.rerender({ live: new URLSearchParams(window.location.search) })
    },
  }
}

describe('withoutBannerParams', () => {
  it.each([
    ['reason=ACCOUNT_INACTIVE&returnTo=%2Fshows', 'returnTo=%2Fshows'],
    ['error_code=RATE_LIMITED&error=x&returnTo=%2Fvenues', 'returnTo=%2Fvenues'],
    ['returnTo=%2Fshows&error=x&intent=save&utm_source=a', 'returnTo=%2Fshows&intent=save&utm_source=a'],
    ['reason=REAUTH_REQUIRED&returnTo=%2Fprofile', 'reason=REAUTH_REQUIRED&returnTo=%2Fprofile'],
    ['reason=OAUTH_LINK_REAUTH_REQUIRED', 'reason=OAUTH_LINK_REAUTH_REQUIRED'],
    ['reason=something_else', ''],
    ['returnTo=%2Fshows', 'returnTo=%2Fshows'],
    ['', ''],
  ])('%s -> %s', (query, want) => {
    expect(withoutBannerParams(query)).toBe(want)
  })
})

describe('useAuthArrivalParams', () => {
  let pushState: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    pushState = vi.spyOn(window.history, 'pushState')
  })

  afterEach(() => {
    pushState.mockRestore()
    window.history.replaceState(null, '', '/')
  })

  it('keeps the banner parameters on the first render', () => {
    const { result } = renderArrival('reason=ACCOUNT_INACTIVE&returnTo=%2Fshows')

    expect(result.current.get('reason')).toBe('ACCOUNT_INACTIVE')
    expect(result.current.get('returnTo')).toBe('/shows')
  })

  it('takes them out of the address bar, keeping returnTo and the fragment', () => {
    window.history.replaceState(null, '', '/auth?error_code=RATE_LIMITED&error=x&returnTo=%2Fvenues#top')
    renderHook(() =>
      useAuthArrivalParams(new URLSearchParams('error_code=RATE_LIMITED&error=x&returnTo=%2Fvenues'))
    )

    expect(`${window.location.pathname}${window.location.search}${window.location.hash}`).toBe(
      '/auth?returnTo=%2Fvenues#top'
    )
  })

  it('replaces the history entry rather than adding one', () => {
    const before = window.history.length
    const replaceState = vi.spyOn(window.history, 'replaceState')
    renderArrival('reason=ACCOUNT_INACTIVE')

    // The App Router skips its useSearchParams sync for state that already
    // carries its own, so the hook hands it none.
    expect(replaceState).toHaveBeenLastCalledWith(null, '', '/auth')
    expect(pushState).not.toHaveBeenCalled()
    expect(window.history.length).toBe(before)
    replaceState.mockRestore()
  })

  it('still returns the arrival once useSearchParams follows the address bar', () => {
    const view = renderArrival('reason=ACCOUNT_INACTIVE&returnTo=%2Fshows')
    const arrival = view.result.current.toString()

    view.syncWithAddressBar()

    expect(window.location.search).toBe('?returnTo=%2Fshows')
    expect(view.result.current.toString()).toBe(arrival)
  })

  it('leaves the URL alone when it carries no banner parameter', () => {
    const replaceState = vi.spyOn(window.history, 'replaceState')
    renderHook(() =>
      useAuthArrivalParams(new URLSearchParams('returnTo=%2Fshows&intent=save'))
    )

    expect(replaceState).not.toHaveBeenCalled()
    replaceState.mockRestore()
  })

  it('keeps a re-authentication reason in the address bar', () => {
    const view = renderArrival('reason=REAUTH_REQUIRED&returnTo=%2Fprofile&error=x')

    expect(window.location.search).toBe('?reason=REAUTH_REQUIRED&returnTo=%2Fprofile')
    view.syncWithAddressBar()
    expect(view.result.current.get('reason')).toBe('REAUTH_REQUIRED')
  })

  it('shows no banner parameter on a reload of the rewritten URL', () => {
    const first = renderArrival('reason=ACCOUNT_INACTIVE&returnTo=%2Fshows')
    first.unmount()

    const reloaded = renderArrival(window.location.search.slice(1))

    expect(reloaded.result.current.get('reason')).toBeNull()
    expect(reloaded.result.current.get('returnTo')).toBe('/shows')
  })

  it('holds a new arrival with banner parameters on the mounted page', () => {
    const view = renderArrival('reason=ACCOUNT_INACTIVE')
    view.syncWithAddressBar()

    view.navigate('error=second&returnTo=%2Fvenues')

    expect(view.result.current.get('error')).toBe('second')
    expect(view.result.current.get('reason')).toBeNull()
    view.syncWithAddressBar()
    expect(view.result.current.get('error')).toBe('second')
    expect(window.location.search).toBe('?returnTo=%2Fvenues')
  })

  it('does not bring a banner back when navigation returns to the rewritten URL', () => {
    const view = renderArrival('reason=ACCOUNT_INACTIVE&returnTo=%2Fshows')
    view.syncWithAddressBar()

    view.navigate('')
    expect(view.result.current.get('reason')).toBeNull()

    view.navigate('returnTo=%2Fshows')
    expect(view.result.current.get('reason')).toBeNull()
    expect(view.result.current.get('returnTo')).toBe('/shows')
  })
})
