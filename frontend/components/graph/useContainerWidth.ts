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

export function useContainerWidth(): {
  refCallback: (node: HTMLDivElement | null) => void | (() => void)
  containerWidth: number | null
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

  return { refCallback, containerWidth }
}
