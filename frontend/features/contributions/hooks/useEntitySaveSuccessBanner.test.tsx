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
