import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import {
  ATLAS_COMPACT_VIEWPORT_QUERY,
  useAtlasCompactViewport,
} from './atlasViewport'

/** A `matchMedia` whose answer for the compact query a test can flip. */
function installMatchMedia(initial: boolean) {
  let matches = initial
  const listeners = new Set<() => void>()
  const queries: string[] = []
  window.matchMedia = ((query: string) => {
    queries.push(query)
    return {
      get matches() {
        return query === ATLAS_COMPACT_VIEWPORT_QUERY ? matches : false
      },
      media: query,
      onchange: null,
      addEventListener: (_: string, fn: () => void) => listeners.add(fn),
      removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }
  }) as unknown as typeof window.matchMedia
  return {
    queries,
    set(next: boolean) {
      matches = next
      act(() => listeners.forEach((fn) => fn()))
    },
  }
}

describe('useAtlasCompactViewport', () => {
  const original = window.matchMedia
  afterEach(() => {
    window.matchMedia = original
  })

  it('asks for the exact complement of the Tailwind lg breakpoint', () => {
    // `lg:` is `min-width: 64rem`; a branch on this query and an `lg:`
    // utility must flip at the same width.
    expect(ATLAS_COMPACT_VIEWPORT_QUERY).toBe('not all and (min-width: 64rem)')
    const mm = installMatchMedia(false)
    renderHook(() => useAtlasCompactViewport())
    expect(mm.queries).toContain(ATLAS_COMPACT_VIEWPORT_QUERY)
  })

  it('follows the viewport across the breakpoint', () => {
    const mm = installMatchMedia(true)
    const { result } = renderHook(() => useAtlasCompactViewport())
    expect(result.current).toBe(true)
    mm.set(false)
    expect(result.current).toBe(false)
    mm.set(true)
    expect(result.current).toBe(true)
  })
})
