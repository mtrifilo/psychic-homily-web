import { describe, it, expect, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { useUrlHash, GRAPH_HASH } from './useUrlHash'

/**
 * Set `window.location.hash` and fire the `hashchange` event the hook
 * subscribes to. jsdom updates `location.hash` synchronously but does NOT
 * auto-dispatch `hashchange`, so we dispatch it ourselves.
 */
function setHash(hash: string): void {
  window.location.hash = hash
  window.dispatchEvent(new HashChangeEvent('hashchange'))
}

describe('useUrlHash', () => {
  afterEach(() => {
    // Reset the shared jsdom location between tests so hash state doesn't leak.
    window.location.hash = ''
  })

  it('returns "" on the server (getServerSnapshot)', () => {
    // useSyncExternalStore calls getServerSnapshot during server rendering.
    // renderToStaticMarkup exercises that path without touching jsdom's
    // window.location, so the "" server snapshot is what gets serialized.
    function Probe() {
      return createElement('span', null, `[${useUrlHash()}]`)
    }
    const html = renderToStaticMarkup(createElement(Probe))
    expect(html).toBe('<span>[]</span>')
  })

  it('reads the current hash on the client (getSnapshot)', () => {
    window.location.hash = GRAPH_HASH
    const { result } = renderHook(() => useUrlHash())
    expect(result.current).toBe(GRAPH_HASH)
  })

  it('updates the value when a hashchange event fires', () => {
    const { result } = renderHook(() => useUrlHash())
    expect(result.current).toBe('')

    act(() => {
      setHash(GRAPH_HASH)
    })
    expect(result.current).toBe(GRAPH_HASH)

    act(() => {
      setHash('#other')
    })
    expect(result.current).toBe('#other')
  })

  it('removes the hashchange listener on unmount', () => {
    const { result, unmount } = renderHook(() => useUrlHash())

    act(() => {
      setHash(GRAPH_HASH)
    })
    expect(result.current).toBe(GRAPH_HASH)

    unmount()

    // After unmount the listener is gone, so a later hashchange must not
    // re-render the hook. The last value the hook reported stays put.
    act(() => {
      setHash('#after-unmount')
    })
    expect(result.current).toBe(GRAPH_HASH)
  })
})
