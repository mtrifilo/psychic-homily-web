import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import {
  useSoftKeyboardViewport,
  SOFT_KEYBOARD_VIEWPORT_QUERY,
} from './useSoftKeyboardViewport'

type ChangeHandler = (ev: MediaQueryListEvent) => void

/**
 * Controllable matchMedia stand-in. `modern: false` drops
 * addEventListener/removeEventListener so the older-Safari path is exercised
 * by the same helper.
 */
function setupMatchMediaMock(
  initialMatches: boolean,
  { modern = true }: { modern?: boolean } = {}
) {
  const listeners: ChangeHandler[] = []
  let matches = initialMatches
  const queries: string[] = []

  const add = vi.fn((_event: string, handler: ChangeHandler) => {
    listeners.push(handler)
  })
  const remove = vi.fn((_event: string, handler: ChangeHandler) => {
    const idx = listeners.indexOf(handler)
    if (idx >= 0) listeners.splice(idx, 1)
  })
  const legacyAdd = vi.fn((handler: ChangeHandler) => {
    listeners.push(handler)
  })
  const legacyRemove = vi.fn((handler: ChangeHandler) => {
    const idx = listeners.indexOf(handler)
    if (idx >= 0) listeners.splice(idx, 1)
  })

  const mqList = {
    get matches() {
      return matches
    },
    media: SOFT_KEYBOARD_VIEWPORT_QUERY,
    onchange: null,
    ...(modern ? { addEventListener: add, removeEventListener: remove } : {}),
    addListener: legacyAdd,
    removeListener: legacyRemove,
    dispatchEvent: vi.fn(),
  }

  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: vi.fn((query: string) => {
      queries.push(query)
      return mqList
    }),
  })

  return {
    queries,
    legacyAdd,
    legacyRemove,
    remove,
    fireChange(next: boolean) {
      matches = next
      for (const handler of listeners) {
        handler({ matches: next } as MediaQueryListEvent)
      }
    },
  }
}

function restoreDefaultMatchMedia() {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
}

describe('useSoftKeyboardViewport', () => {
  afterEach(() => {
    restoreDefaultMatchMedia()
  })

  it('reports false on a wide fine-pointer viewport', () => {
    const { queries } = setupMatchMediaMock(false)

    const { result } = renderHook(() => useSoftKeyboardViewport())

    expect(result.current).toBe(false)
    expect(queries).toContain(SOFT_KEYBOARD_VIEWPORT_QUERY)
  })

  it('reports true when the narrow-or-coarse query matches', () => {
    setupMatchMediaMock(true)

    const { result } = renderHook(() => useSoftKeyboardViewport())

    expect(result.current).toBe(true)
  })

  it('follows a viewport change mid-session', () => {
    const { fireChange } = setupMatchMediaMock(false)

    const { result } = renderHook(() => useSoftKeyboardViewport())
    expect(result.current).toBe(false)

    act(() => fireChange(true))
    expect(result.current).toBe(true)

    act(() => fireChange(false))
    expect(result.current).toBe(false)
  })

  it('unsubscribes on unmount', () => {
    const { remove } = setupMatchMediaMock(true)

    const { unmount } = renderHook(() => useSoftKeyboardViewport())
    unmount()

    expect(remove).toHaveBeenCalledWith('change', expect.any(Function))
  })

  it('falls back to addListener when addEventListener is unavailable', () => {
    const { legacyAdd, legacyRemove, fireChange } = setupMatchMediaMock(false, {
      modern: false,
    })

    const { result, unmount } = renderHook(() => useSoftKeyboardViewport())
    expect(legacyAdd).toHaveBeenCalled()

    act(() => fireChange(true))
    expect(result.current).toBe(true)

    unmount()
    expect(legacyRemove).toHaveBeenCalled()
  })

  it('reports false when the environment has no matchMedia', () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      configurable: true,
      value: undefined,
    })

    const { result } = renderHook(() => useSoftKeyboardViewport())

    expect(result.current).toBe(false)
  })
})
