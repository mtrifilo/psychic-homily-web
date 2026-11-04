'use client'

import { useLayoutEffect, useState, type RefObject } from 'react'

// Fractional zoom and device pixel ratios can leave scrollLeft a fraction
// short of its maximum when the scroller is at its end.
const SCROLL_END_TOLERANCE_PX = 1

/**
 * Whether a horizontal scroller hides content past its right edge: true while
 * it overflows and is not scrolled to its end.
 *
 * The ref must be attached when the calling component mounts; the listeners
 * bind once. Direct children are observed as well as the scroller, because a
 * child that widens grows scrollWidth without resizing the scroller. Only the
 * children present at mount are observed, so the scroller's child list must
 * be fixed for its lifetime.
 */
export function useTrailingOverflow(
  scrollerRef: RefObject<HTMLElement | null>
): boolean {
  const [hasTrailingOverflow, setHasTrailingOverflow] = useState(false)

  // Measured before paint, so an overflowing scroller never paints a frame
  // without its fade.
  useLayoutEffect(() => {
    const scroller = scrollerRef.current
    if (!scroller) return

    const measure = () => {
      const visibleEnd = scroller.scrollLeft + scroller.clientWidth
      setHasTrailingOverflow(
        visibleEnd < scroller.scrollWidth - SCROLL_END_TOLERANCE_PX
      )
    }

    measure()
    scroller.addEventListener('scroll', measure, { passive: true })
    const resizeObserver = new ResizeObserver(measure)
    resizeObserver.observe(scroller)
    for (const child of Array.from(scroller.children)) {
      resizeObserver.observe(child)
    }

    return () => {
      scroller.removeEventListener('scroll', measure)
      resizeObserver.disconnect()
    }
  }, [scrollerRef])

  return hasTrailingOverflow
}
