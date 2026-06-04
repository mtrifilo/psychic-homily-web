import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useAutoDismissBanner, useAutoDismissFlag } from './useAutoDismissBanner'

// PSY-957: the one timer implementation behind auto-dismiss banners. These
// tests pin the lifecycle contract all call sites rely on — arm on show,
// re-arm on re-show, sticky variant, clear-on-unmount — so a future edit to
// the primitive can't silently regress one of them.
describe('useAutoDismissBanner (PSY-957)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('starts with no value', () => {
    const { result } = renderHook(() => useAutoDismissBanner<string>(3000))
    expect(result.current.value).toBeNull()
  })

  it('exposes the value after show() and auto-dismisses after the delay', () => {
    const { result } = renderHook(() => useAutoDismissBanner<string>(3000))

    act(() => {
      result.current.show('saved')
    })
    expect(result.current.value).toBe('saved')

    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(result.current.value).toBeNull()
  })

  it('re-arms the timer when show() is called again before dismissal', () => {
    const { result } = renderHook(() => useAutoDismissBanner<string>(3000))

    act(() => {
      result.current.show('first')
    })
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    act(() => {
      result.current.show('second')
    })

    // The first show()'s window has fully elapsed by now, but the re-shown
    // banner stays up for its own full window.
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(result.current.value).toBe('second')

    act(() => {
      vi.advanceTimersByTime(1000)
    })
    expect(result.current.value).toBeNull()
  })

  it('re-arms even when the re-shown value is identical', () => {
    // Two consecutive failures with the same message must keep the banner
    // up for a full window after the SECOND failure (entry identity is the
    // re-arm key, not value equality).
    const { result } = renderHook(() => useAutoDismissBanner<string>(3000))

    act(() => {
      result.current.show('same message')
    })
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    act(() => {
      result.current.show('same message')
    })
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(result.current.value).toBe('same message')
  })

  it('showSticky() keeps the value past the dismiss window', () => {
    const { result } = renderHook(() => useAutoDismissBanner<string>(3000))

    act(() => {
      result.current.showSticky('error the user needs to read')
    })
    act(() => {
      vi.advanceTimersByTime(60_000)
    })
    expect(result.current.value).toBe('error the user needs to read')
  })

  it('showSticky() cancels a pending auto-dismiss from a prior show()', () => {
    // Regression guard for the pre-PSY-957 AddItemsPanel bug: a sticky
    // error shown while a prior success's timer was still pending got
    // dismissed early by that stale timer.
    const { result } = renderHook(() => useAutoDismissBanner<string>(3000))

    act(() => {
      result.current.show('auto-dismissing success')
    })
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    act(() => {
      result.current.showSticky('sticky error')
    })

    act(() => {
      vi.advanceTimersByTime(10_000)
    })
    expect(result.current.value).toBe('sticky error')
  })

  it('clear() hides immediately and cancels the pending dismissal', () => {
    const { result } = renderHook(() => useAutoDismissBanner<string>(3000))

    act(() => {
      result.current.show('visible')
    })
    act(() => {
      result.current.clear()
    })
    expect(result.current.value).toBeNull()

    // No stray timer fires later.
    act(() => {
      vi.advanceTimersByTime(5000)
    })
    expect(result.current.value).toBeNull()
  })

  it('clears the pending timer on unmount (no setState after unmount)', () => {
    const { result, unmount } = renderHook(() =>
      useAutoDismissBanner<string>(3000)
    )

    act(() => {
      result.current.show('visible')
    })

    // Unmounting before the timer fires must not throw / leak.
    expect(() => {
      unmount()
      vi.advanceTimersByTime(5000)
    }).not.toThrow()
  })

  it('holds non-string values (feedback objects)', () => {
    const { result } = renderHook(() =>
      useAutoDismissBanner<{ variant: string; message: string }>(4000)
    )

    act(() => {
      result.current.show({ variant: 'success', message: 'Added 3 items' })
    })
    expect(result.current.value).toEqual({
      variant: 'success',
      message: 'Added 3 items',
    })

    act(() => {
      vi.advanceTimersByTime(4000)
    })
    expect(result.current.value).toBeNull()
  })

  it('returns referentially stable show/showSticky/clear across renders', () => {
    // Call sites put these in dependency arrays / useCallback deps; unstable
    // identities would defeat their memoization.
    const { result, rerender } = renderHook(() =>
      useAutoDismissBanner<string>(3000)
    )
    const { show, showSticky, clear } = result.current

    rerender()
    expect(result.current.show).toBe(show)
    expect(result.current.showSticky).toBe(showSticky)
    expect(result.current.clear).toBe(clear)
  })
})

describe('useAutoDismissFlag (PSY-957)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('starts hidden, shows on trigger, hides after the delay', () => {
    const { result } = renderHook(() => useAutoDismissFlag(2000))
    expect(result.current[0]).toBe(false)

    act(() => {
      result.current[1]()
    })
    expect(result.current[0]).toBe(true)

    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(result.current[0]).toBe(false)
  })

  it('re-arms on rapid double trigger (e.g. Share clicked twice)', () => {
    const { result } = renderHook(() => useAutoDismissFlag(2000))

    act(() => {
      result.current[1]()
    })
    act(() => {
      vi.advanceTimersByTime(1500)
    })
    act(() => {
      result.current[1]()
    })

    // First trigger's window has elapsed; flag stays up for the full window
    // after the second trigger.
    act(() => {
      vi.advanceTimersByTime(1500)
    })
    expect(result.current[0]).toBe(true)

    act(() => {
      vi.advanceTimersByTime(500)
    })
    expect(result.current[0]).toBe(false)
  })

  it('returns a referentially stable trigger across renders', () => {
    // Call sites put trigger in dependency arrays (e.g. handleShare's
    // useCallback) — an unstable identity would defeat their memoization.
    const { result, rerender } = renderHook(() => useAutoDismissFlag(2000))
    const firstTrigger = result.current[1]

    rerender()
    expect(result.current[1]).toBe(firstTrigger)
  })
})
