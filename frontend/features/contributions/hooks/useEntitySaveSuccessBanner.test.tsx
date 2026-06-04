import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useEntitySaveSuccessBanner } from './useEntitySaveSuccessBanner'

// Pure client-state hook (no react-query) — drives the page-level
// "Changes saved" banner that follows a direct admin/trusted save.
describe('useEntitySaveSuccessBanner', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('starts hidden', () => {
    const { result } = renderHook(() => useEntitySaveSuccessBanner())

    expect(result.current.isVisible).toBe(false)
    expect(typeof result.current.handleSaveSuccess).toBe('function')
  })

  it('shows the banner on a direct save (applied: true)', () => {
    const { result } = renderHook(() => useEntitySaveSuccessBanner())

    act(() => {
      result.current.handleSaveSuccess({ applied: true })
    })

    expect(result.current.isVisible).toBe(true)
  })

  it('does NOT show the banner for a pending submission (applied: false)', () => {
    const { result } = renderHook(() => useEntitySaveSuccessBanner())

    act(() => {
      result.current.handleSaveSuccess({ applied: false })
    })

    // Pending submissions keep the in-drawer amber banner instead.
    expect(result.current.isVisible).toBe(false)
  })

  it('auto-dismisses the banner after 5 seconds', () => {
    const { result } = renderHook(() => useEntitySaveSuccessBanner())

    act(() => {
      result.current.handleSaveSuccess({ applied: true })
    })
    expect(result.current.isVisible).toBe(true)

    // Just before the 5s threshold it is still visible.
    act(() => {
      vi.advanceTimersByTime(4999)
    })
    expect(result.current.isVisible).toBe(true)

    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(result.current.isVisible).toBe(false)
  })

  it('re-arms a full window when a second direct save lands while still visible (PSY-958)', () => {
    // PSY-958 behavior note: the primitive re-arms on every trigger, so a
    // repeat save extends the window to a fresh 5s from the latest save (the
    // pre-PSY-958 [isVisible]-keyed effect did not re-arm).
    const { result } = renderHook(() => useEntitySaveSuccessBanner())

    act(() => {
      result.current.handleSaveSuccess({ applied: true })
    })
    act(() => {
      vi.advanceTimersByTime(3000)
    })
    // Second save 3s in — must re-arm, not inherit the first window.
    act(() => {
      result.current.handleSaveSuccess({ applied: true })
    })
    // Past the FIRST save's original 5s deadline, still visible (re-armed).
    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(result.current.isVisible).toBe(true)
    // A full 5s after the second save, it dismisses.
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(result.current.isVisible).toBe(false)
  })

  it('clears the pending timer on unmount (no post-unmount state update)', () => {
    const { result, unmount } = renderHook(() =>
      useEntitySaveSuccessBanner()
    )

    act(() => {
      result.current.handleSaveSuccess({ applied: true })
    })
    expect(result.current.isVisible).toBe(true)

    // Unmounting before the timer fires must not throw / leak.
    expect(() => {
      unmount()
      vi.advanceTimersByTime(5000)
    }).not.toThrow()
  })
})
