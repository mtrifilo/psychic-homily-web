import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useTrailingOverflow } from './useTrailingOverflow'

// Records what the hook observes and lets a test deliver a resize.
class RecordingResizeObserver {
  static instances: RecordingResizeObserver[] = []
  observed: Element[] = []
  disconnected = false
  constructor(private readonly callback: () => void) {
    RecordingResizeObserver.instances.push(this)
  }
  observe(target: Element) {
    this.observed.push(target)
  }
  unobserve() {}
  disconnect() {
    this.disconnected = true
  }
  fire() {
    if (!this.disconnected) this.callback()
  }
}

function makeScroller(metrics: {
  scrollWidth: number
  clientWidth: number
  scrollLeft?: number
}) {
  const scroller = document.createElement('div')
  scroller.append(document.createElement('button'), document.createElement('button'))
  Object.defineProperty(scroller, 'scrollWidth', {
    get: () => metrics.scrollWidth,
  })
  Object.defineProperty(scroller, 'clientWidth', {
    get: () => metrics.clientWidth,
  })
  Object.defineProperty(scroller, 'scrollLeft', {
    get: () => metrics.scrollLeft ?? 0,
  })
  return { scroller, scrollerRef: { current: scroller } }
}

describe('useTrailingOverflow', () => {
  const originalResizeObserver = window.ResizeObserver

  beforeEach(() => {
    RecordingResizeObserver.instances = []
    window.ResizeObserver =
      RecordingResizeObserver as unknown as typeof ResizeObserver
  })

  afterEach(() => {
    window.ResizeObserver = originalResizeObserver
    vi.restoreAllMocks()
  })

  it('is false when the content fits', () => {
    const { scrollerRef } = makeScroller({ scrollWidth: 300, clientWidth: 358 })
    const { result } = renderHook(() => useTrailingOverflow(scrollerRef))
    expect(result.current).toBe(false)
  })

  it('treats a sub-pixel shortfall at the end as the end', () => {
    const { scrollerRef } = makeScroller({
      scrollWidth: 636,
      clientWidth: 358,
      scrollLeft: 277.5,
    })
    const { result } = renderHook(() => useTrailingOverflow(scrollerRef))
    expect(result.current).toBe(false)
  })

  it('turns on when a child widens past the edge without resizing the scroller', () => {
    const metrics = { scrollWidth: 340, clientWidth: 358 }
    const { scroller, scrollerRef } = makeScroller(metrics)
    const { result } = renderHook(() => useTrailingOverflow(scrollerRef))
    expect(result.current).toBe(false)

    const [observer] = RecordingResizeObserver.instances
    expect(observer.observed).toEqual([scroller, ...Array.from(scroller.children)])

    metrics.scrollWidth = 636
    act(() => observer.fire())
    expect(result.current).toBe(true)
  })

  it('releases its listeners on unmount', () => {
    const { scroller, scrollerRef } = makeScroller({ scrollWidth: 636, clientWidth: 358 })
    const removeListener = vi.spyOn(scroller, 'removeEventListener')
    const { unmount } = renderHook(() => useTrailingOverflow(scrollerRef))

    unmount()

    expect(RecordingResizeObserver.instances[0].disconnected).toBe(true)
    expect(removeListener).toHaveBeenCalledWith('scroll', expect.any(Function))
  })
})
