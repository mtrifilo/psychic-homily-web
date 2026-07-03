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
 * Caller checklist (this hook is the lifecycle only — the caller owns):
 *   - the overlay shell: `role="dialog"`, `aria-modal="true"`, an aria-label,
 *     and `z-[60]` (above the z-50 cookie-consent banner, PSY-518);
 *   - the INLINE copy must get `aria-hidden={isFullscreen || undefined}` +
 *     `inert={isFullscreen || undefined}` so assistive tech sees one graph
 *     surface while the overlay is open (PSY-517);
 *   - gate the overlay render on `available` too (the hook resets state, but
 *     the render guard is the caller's);
 *   - `overlayWidth`/`overlayHeight` are null until the first post-open
 *     commit and go stale after close — only read them under `isFullscreen`,
 *     guarded non-null. `open()` while `!available` is a no-op by design.
 *
 * CONTRACT for `available`: it must not flip false on fetch transients while
 * the overlay is open — auto-close is one-way. If the overlay hosts controls
 * that refetch the section's own data (query-key changes), the data hook
 * needs `placeholderData: keepPreviousData` so counts don't collapse to zero
 * mid-fetch (see useVenueBillNetwork, the PSY-1305 review finding).
 *
 * The Expand/Exit trigger buttons stay hand-rolled in each caller
 * (deliberate: they pre-date ui/Button adoption and are visually synced
 * across all graph surfaces; switching is a coordinated four-surface change
 * out of PSY-1305's behavior-preserving scope).
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
      // Layered dismiss (PSY-1334): an inner surface (the ConnectionPanel)
      // claims the Escape by preventDefault in the capture phase — skip it
      // here so one keypress closes the innermost layer only.
      if (e.key === 'Escape' && !e.defaultPrevented) {
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
