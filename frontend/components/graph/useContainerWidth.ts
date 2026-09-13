'use client'

/**
 * useContainerWidth (PSY-1305)
 *
 * Shared width measurement for graph sections — extracted from the five
 * near-identical copies in SceneGraph / CollectionGraph / VenueBillNetwork /
 * InlineGraph / StationGraph.
 *
 * WHY a callback ref and not useRef + useEffect([]): graph sections commonly
 * return null on their first renders (data still loading), so an effect with
 * empty deps fires once while ref.current is still null and never re-runs —
 * the container is never measured and the graph stays hidden forever
 * (PSY-516/PSY-519). A callback ref fires whenever the underlying DOM node
 * mounts/unmounts, so the right node is always measured. The cleanup return
 * from a callback ref is honored by React 19 (this repo pins 19.x).
 */

import { useState, useCallback } from 'react'

/**
 * Graph canvases are unusable below this width (PSY-369/PSY-511): tap
 * targets fail WCAG, the center node lands off-screen. Below it, sections
 * hide the canvas and let their list views carry the content.
 */
export const GRAPH_BREAKPOINT_PX = 640

/**
 * The class an UNMEASURED section's chrome carries: its header, scale line and
 * controls. Below the gate a graph section's settled form is one line of link,
 * and server-rendered HTML has no container width to gate on, so without this
 * a phone paints a header that the first measurement then deletes.
 *
 * Viewport-keyed where the gate is container-keyed: in the narrow band where a
 * padded column measures under 640px on a wider viewport the chrome paints
 * until the measurement replaces it. Pair it with `containerWidth === null`,
 * and use the variant matching the element's own display.
 */
export const GRAPH_CHROME_UNMEASURED_CLASS = 'hidden sm:block'
/** `display: flex` variant of GRAPH_CHROME_UNMEASURED_CLASS. */
export const GRAPH_CHROME_UNMEASURED_FLEX_CLASS = 'hidden sm:flex'

export function useContainerWidth(): {
  refCallback: (node: HTMLDivElement | null) => void | (() => void)
  containerWidth: number | null
  isBelowGraphBreakpoint: boolean
} {
  const [containerWidth, setContainerWidth] = useState<number | null>(null)

  const refCallback = useCallback((node: HTMLDivElement | null) => {
    if (!node) return
    // Known quirk, faithfully inherited from the original copies: the initial
    // measure is border-box (getBoundingClientRect) while observer updates
    // are content-box (contentRect) — on a padded container near the 640px
    // gate the first resize event can shift the value without a layout
    // change. All current consumers measure unpadded wrappers.
    setContainerWidth(node.getBoundingClientRect().width)
    const observer = new ResizeObserver(entries => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width)
      }
    })
    observer.observe(node)
    return () => {
      observer.disconnect()
      // Back to "unmeasured" so a consumer that unmounts just the measured
      // div (none today) can't keep a stale width driving its gates.
      setContainerWidth(null)
    }
  }, [])

  // Measured narrow, which is NOT the same as "no canvas": pre-measurement is
  // also canvas-less, and it is the state that reserves the canvas box. Only
  // this one collapses a section to its sub-breakpoint form, so the hook that
  // owns the measurement owns the distinction. A consumer re-deriving it is one
  // `containerWidth !== null` away from flashing the narrow form on every first
  // paint.
  //
  // The other side of the gate deliberately stays at the call sites. Written
  // inline as `containerWidth !== null && containerWidth >= …`, it NARROWS
  // `containerWidth` to a number for the canvas that consumes it; a boolean
  // returned from here carries no narrowing, so centralizing it would buy one
  // fewer comparison at the cost of a hand-written non-null assertion on the
  // width every canvas is sized from.
  const isBelowGraphBreakpoint =
    containerWidth !== null && containerWidth < GRAPH_BREAKPOINT_PX

  return { refCallback, containerWidth, isBelowGraphBreakpoint }
}
