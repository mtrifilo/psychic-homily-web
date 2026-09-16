import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useMediaQuery } from './useMediaQuery'

/** A controllable `matchMedia`, so a test can flip a query mid-session. */
function installMatchMedia(matches: Record<string, boolean>) {
  const listeners = new Map<string, Set<() => void>>()
  window.matchMedia = ((query: string) => ({
    get matches() {
      return matches[query] ?? false
    },
    media: query,
    onchange: null,
    addEventListener: (_: string, fn: () => void) => {
      if (!listeners.has(query)) listeners.set(query, new Set())
      listeners.get(query)!.add(fn)
    },
    removeEventListener: (_: string, fn: () => void) => {
      listeners.get(query)?.delete(fn)
    },
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia

  return {
    set(query: string, value: boolean) {
      matches[query] = value
      act(() => {
        listeners.get(query)?.forEach(fn => fn())
      })
    },
    listenerCount(query: string) {
      return listeners.get(query)?.size ?? 0
    },
  }
}

const WIDE = '(min-width: 1280px)'

describe('useMediaQuery', () => {
  const original = window.matchMedia

  afterEach(() => {
    window.matchMedia = original
  })

  it('reports whether the query matches right now', () => {
    installMatchMedia({ [WIDE]: true })
    const { result } = renderHook(() => useMediaQuery(WIDE))
    expect(result.current).toBe(true)
  })

  it('re-renders when the query stops matching', () => {
    const mm = installMatchMedia({ [WIDE]: true })
    const { result } = renderHook(() => useMediaQuery(WIDE))

    mm.set(WIDE, false)
    expect(result.current).toBe(false)

    mm.set(WIDE, true)
    expect(result.current).toBe(true)
  })

  it('drops its listener when the caller unmounts', () => {
    const mm = installMatchMedia({ [WIDE]: false })
    const { unmount } = renderHook(() => useMediaQuery(WIDE))
    expect(mm.listenerCount(WIDE)).toBe(1)

    unmount()
    expect(mm.listenerCount(WIDE)).toBe(0)
  })

  it('answers false where there is no matchMedia to ask', () => {
    // The server, and a jsdom without the shim: the contract is "no match",
    // never a throw, so a gated surface simply does not render.
    // @ts-expect-error deleting the global is the condition under test
    delete window.matchMedia
    const { result } = renderHook(() => useMediaQuery(WIDE))
    expect(result.current).toBe(false)
  })
})

describe('useMediaQuery server snapshot', () => {
  beforeEach(() => {
    installMatchMedia({ [WIDE]: true })
  })

  it('does not report a match before hydration', async () => {
    // `renderToString` runs the server snapshot, which is always false: that is
    // what keeps a query-gated element out of the server HTML.
    const { renderToString } = await import('react-dom/server')
    function Probe() {
      return <span>{String(useMediaQuery(WIDE))}</span>
    }
    expect(renderToString(<Probe />)).toContain('false')
  })
})
