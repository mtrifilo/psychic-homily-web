'use client'

/**
 * GraphPanelHost (PSY-1473) — the shared canvas-wrap chrome for Section-class
 * graph surfaces that float a context panel over the canvas.
 *
 * Five consumers (HomeSceneGraph, SceneGraphVisualization,
 * StationGraphVisualization, VenueBillNetworkAdapter, CollectionGraph) all
 * need the same focusable relative wrap: `tabIndex={-1}` so
 * `useArtistPanelSelection`'s focus-return can land here after Esc/X, and
 * `relative` so the floated panel's absolute positioning is scoped. Extracted
 * at the 5th consumer (rule of three was already past) so the wrap contract
 * can't drift per surface.
 *
 * The caller still owns selection state, the card/panel body, and panel
 * corner placement (top-left vs top-right depends on whether EdgeLegend
 * owns the opposite corner).
 */

import type { ReactNode, Ref } from 'react'

export interface GraphPanelHostProps {
  /** Attach to `useArtistPanelSelection`'s `canvasWrapRef`. */
  canvasWrapRef: Ref<HTMLDivElement | null>
  /** ForceGraphView (and any canvas-adjacent chrome). */
  children: ReactNode
  /** Floated context panel; omit when nothing is selected. */
  panel?: ReactNode
}

export function GraphPanelHost({
  canvasWrapRef,
  children,
  panel,
}: GraphPanelHostProps) {
  return (
    <div ref={canvasWrapRef} tabIndex={-1} className="relative outline-none">
      {children}
      {panel}
    </div>
  )
}
