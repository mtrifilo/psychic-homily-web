/**
 * Immediate ResizeObserver test shim (PSY-1305) — extracted from the
 * per-file copies in the SceneGraph / VenueBillNetwork / BillComposition /
 * RelatedArtists / StationGraph tests. AtlasGlobe.test.tsx deliberately
 * keeps its own shim: the globe consumes HEIGHT, which this shim does not
 * emit.
 *
 * `observe()` fires the callback synchronously with a mock width so
 * components using the callback-ref measurement pattern (useContainerWidth)
 * see a measured container on mount. `fireResize()` re-fires the LAST
 * observer with a new width, simulating the viewport crossing a breakpoint
 * after mount (e.g. the overlay auto-close path). Last-writer-wins: with
 * multiple observed nodes in one render tree, fireResize targets only the
 * most recently observed one — fine for single-graph tests, broadcast would
 * be needed for multi-graph fixtures.
 *
 * Usage:
 *   const ro = installImmediateResizeObserver()   // in beforeEach
 *   ro.setWidth(500)                              // before render
 *   act(() => ro.fireResize(500))                 // after render
 *   ro.restore()                                  // in afterEach
 */

interface ImmediateResizeObserverControls {
  /** Set the width reported to the NEXT observe() call. */
  setWidth: (width: number) => void
  /** Re-fire the most recent observer/target with a new width. */
  fireResize: (width: number) => void
  /** Restore the real ResizeObserver. */
  restore: () => void
}

export function installImmediateResizeObserver(
  initialWidth = 1024,
): ImmediateResizeObserverControls {
  let mockWidth = initialWidth
  let lastCallback: ResizeObserverCallback | null = null
  let lastTarget: Element | null = null

  const emit = (callback: ResizeObserverCallback, target: Element) => {
    callback(
      [
        {
          target,
          contentRect: { width: mockWidth } as DOMRectReadOnly,
        } as ResizeObserverEntry,
      ],
      undefined as unknown as ResizeObserver,
    )
  }

  class ImmediateResizeObserver {
    private callback: ResizeObserverCallback
    constructor(callback: ResizeObserverCallback) {
      this.callback = callback
      lastCallback = callback
    }
    observe(target: Element): void {
      lastTarget = target
      emit(this.callback, target)
    }
    unobserve(): void {}
    disconnect(): void {}
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const original = (globalThis as any).ResizeObserver
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  ;(globalThis as any).ResizeObserver = ImmediateResizeObserver

  return {
    setWidth: (width: number) => {
      mockWidth = width
    },
    fireResize: (width: number) => {
      mockWidth = width
      if (lastCallback && lastTarget) emit(lastCallback, lastTarget)
    },
    restore: () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(globalThis as any).ResizeObserver = original
    },
  }
}
