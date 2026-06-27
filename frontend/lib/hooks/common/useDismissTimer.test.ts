import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'

import { useDismissTimer } from './useDismissTimer'

// PSY-1218: the timer lifecycle the artist-graph hoverable tooltip depends on.
// Tested in isolation because the canvas-mounted parent can't render in jsdom.

describe('useDismissTimer', () => {
  afterEach(() => vi.useRealTimers())

  it('calls onDismiss once the delay elapses after schedule()', () => {
    vi.useFakeTimers()
    const onDismiss = vi.fn()
    const { result } = renderHook(() => useDismissTimer(onDismiss, 300))
    result.current.schedule()
    vi.advanceTimersByTime(299)
    expect(onDismiss).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(onDismiss).toHaveBeenCalledTimes(1)
  })

  it('cancel() prevents a scheduled dismiss', () => {
    vi.useFakeTimers()
    const onDismiss = vi.fn()
    const { result } = renderHook(() => useDismissTimer(onDismiss, 300))
    result.current.schedule()
    result.current.cancel()
    vi.advanceTimersByTime(1000)
    expect(onDismiss).not.toHaveBeenCalled()
  })

  it('schedule() resets a pending timer (fires once, measured from the latest call)', () => {
    vi.useFakeTimers()
    const onDismiss = vi.fn()
    const { result } = renderHook(() => useDismissTimer(onDismiss, 300))
    result.current.schedule()
    vi.advanceTimersByTime(200)
    result.current.schedule() // reset the clock
    vi.advanceTimersByTime(200) // 400ms total, only 200ms since the reset
    expect(onDismiss).not.toHaveBeenCalled()
    vi.advanceTimersByTime(100) // 300ms since the reset
    expect(onDismiss).toHaveBeenCalledTimes(1)
  })

  it('never fires after unmount (no fire-after-unmount)', () => {
    vi.useFakeTimers()
    const onDismiss = vi.fn()
    const { result, unmount } = renderHook(() => useDismissTimer(onDismiss, 300))
    result.current.schedule()
    unmount()
    vi.advanceTimersByTime(1000)
    expect(onDismiss).not.toHaveBeenCalled()
  })

  it('invokes the LATEST onDismiss (read through a ref, not captured at schedule time)', () => {
    vi.useFakeTimers()
    const first = vi.fn()
    const second = vi.fn()
    const { result, rerender } = renderHook(({ cb }: { cb: () => void }) => useDismissTimer(cb, 300), {
      initialProps: { cb: first },
    })
    result.current.schedule()
    rerender({ cb: second })
    vi.advanceTimersByTime(300)
    expect(first).not.toHaveBeenCalled()
    expect(second).toHaveBeenCalledTimes(1)
  })

  it('keeps stable schedule/cancel identities across renders', () => {
    const onDismiss = vi.fn()
    const { result, rerender } = renderHook(() => useDismissTimer(onDismiss, 300))
    const first = result.current
    rerender()
    expect(result.current.schedule).toBe(first.schedule)
    expect(result.current.cancel).toBe(first.cancel)
  })
})
