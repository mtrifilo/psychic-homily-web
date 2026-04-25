import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useReducedMotion } from './useReducedMotion'

// Helper that swaps out window.matchMedia with a controllable mock so
// we can assert both the initial snapshot and live updates when the
// user toggles their OS-level setting mid-session.
function setupMatchMediaMock(initialReduced: boolean) {
  const listeners: Array<(ev: MediaQueryListEvent) => void> = []
  let matches = initialReduced

  const mqList = {
    get matches() {
      return matches
    },
    media: '(prefers-reduced-motion: reduce)',
    onchange: null,
    addEventListener: vi.fn((_event: string, handler: (ev: MediaQueryListEvent) => void) => {
      listeners.push(handler)
    }),
    removeEventListener: vi.fn((_event: string, handler: (ev: MediaQueryListEvent) => void) => {
      const idx = listeners.indexOf(handler)
      if (idx >= 0) listeners.splice(idx, 1)
    }),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }

  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockReturnValue(mqList),
  })

  return {
    fireChange(next: boolean) {
      matches = next
      for (const handler of listeners) {
        handler({ matches: next } as MediaQueryListEvent)
      }
    },
  }
}

describe('useReducedMotion', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    // Reset to the default (non-reduced) matchMedia behaviour from
    // test/setup.ts so other suites in the same file aren't polluted.
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
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
  })

  it('returns false when prefers-reduced-motion does not match', () => {
    setupMatchMediaMock(false)
    const { result } = renderHook(() => useReducedMotion())
    expect(result.current).toBe(false)
  })

  it('returns true when prefers-reduced-motion matches', () => {
    setupMatchMediaMock(true)
    const { result } = renderHook(() => useReducedMotion())
    expect(result.current).toBe(true)
  })

  it('updates when the OS-level setting toggles mid-session', () => {
    const { fireChange } = setupMatchMediaMock(false)
    const { result } = renderHook(() => useReducedMotion())
    expect(result.current).toBe(false)

    act(() => fireChange(true))
    expect(result.current).toBe(true)

    act(() => fireChange(false))
    expect(result.current).toBe(false)
  })
})
