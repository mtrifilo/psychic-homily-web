'use client'

/**
 * useFullscreenGraphOverlay (PSY-1305)
 *
 * Shared fullscreen-overlay lifecycle for graph sections — extracted from
 * the four near-identical copies in SceneGraph / CollectionGraph /
 * VenueBillNetwork / StationGraph. Owns the accreted fixes so they can't
 * drift per surface again:
 *   - body scroll lock that SNAPSHOTS the previous inline overflow value and
 *     restores it on close (a blind reset to '' would clobber a parent
 *     layout's inline value);
 *   - Esc-to-close;
 *   - live viewport tracking so the canvas stays full-bleed on window resize
 *     (200px floor, header/legend reserve subtracted);
 *   - auto-close when `available` flips false while open (PSY-1299 finding,
 *     previously only fixed on the station surface): without it, shrinking
 *     the viewport below the graph breakpoint unmounts the overlay but
 *     leaves scroll locked, the inline copy inert, and the overlay popping
 *     back open on re-widen. Uses the React-documented
 *     adjust-state-during-render pattern (the react-hooks lint errors on the
 *     setState-in-effect form).
 *
 * Callers own all policy: when the overlay is offered, what renders inside
 * it, headers/legends/captions. The overlay element itself stays in the
 * caller (z-[60] shell, aria labels) — this hook is the lifecycle only.
 */

import { useState, useEffect, useCallback } from 'react'

/**
 * Vertical space reserved for the overlay's header bar + legend row +
 * padding; subtracted from the viewport height to size the canvas.
 */
export const OVERLAY_VERTICAL_RESERVE_PX = 140

export function useFullscreenGraphOverlay(available: boolean): {
  isFullscreen: boolean
  open: () => void
  close: () => void
  overlayWidth: number | null
  overlayHeight: number | null
} {
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [overlayHeight, setOverlayHeight] = useState<number | null>(null)
  const [overlayWidth, setOverlayWidth] = useState<number | null>(null)

  // Adjust-state-during-render: reset applies before commit, so the
  // scroll-lock effect below cleans up in the same pass.
  if (isFullscreen && !available) {
    setIsFullscreen(false)
  }

  useEffect(() => {
    if (!isFullscreen) return

    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'

    const updateDimensions = () => {
      setOverlayWidth(window.innerWidth)
      setOverlayHeight(Math.max(200, window.innerHeight - OVERLAY_VERTICAL_RESERVE_PX))
    }
    updateDimensions()

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setIsFullscreen(false)
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    window.addEventListener('resize', updateDimensions)

    return () => {
      document.body.style.overflow = previousOverflow
      document.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('resize', updateDimensions)
    }
  }, [isFullscreen])

  const open = useCallback(() => setIsFullscreen(true), [])
  const close = useCallback(() => setIsFullscreen(false), [])

  return { isFullscreen, open, close, overlayWidth, overlayHeight }
}
