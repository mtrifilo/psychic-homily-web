'use client'

/**
 * EgoTypeLegend
 *
 * Canvas-foot legend for the ego graph's relationship-type NODE fills: one
 * round swatch + lowercase family name per fill family present (plus a
 * neutral "other" row when unclassified types are present), followed by the
 * shared functional markers (green upcoming-show dot, violet playable ring)
 * when any rendered node carries them. Complements the top-right EdgeLegend,
 * which teaches the EDGE grammar (color + dash per type); this one teaches
 * what the node decorations mean. Layout per the locked Option B mock: a
 * horizontal row at the foot of the canvas.
 */

import { memo } from 'react'

import { cn } from '@/lib/utils'
import { egoLegendRows, type EgoFillFamily } from './egoPalette'
import { PLAYABLE_RING_COLOR, UPCOMING_SHOW_DOT_COLOR } from './graphMarkers'

export interface EgoTypeLegendProps {
  /** Fill families assigned to the rendered nodes (null = neutral). */
  families: ReadonlyArray<EgoFillFamily | null>
  /** Any rendered node carries the green upcoming-show dot. */
  showUpcomingDot?: boolean
  /** Any rendered node carries the violet playable-audio ring. */
  showPlayableRing?: boolean
  className?: string
}

// memo: the host re-renders per mousemove while hovering canvas nodes, but
// `families` is referentially stable (derived in the graph-data memo), so
// the legend only needs to re-render when the graph itself changes.
export const EgoTypeLegend = memo(function EgoTypeLegend({
  families,
  showUpcomingDot = false,
  showPlayableRing = false,
  className,
}: EgoTypeLegendProps) {
  const rows = egoLegendRows(families)
  if (rows.length === 0 && !showUpcomingDot && !showPlayableRing) return null

  return (
    <div
      data-testid="ego-type-legend"
      className={cn(
        'flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-border/50 px-3 py-2',
        className,
      )}
    >
      {rows.map(row => (
        <span key={row.key} className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
          <span
            aria-hidden="true"
            className="size-2.5 shrink-0 rounded-full"
            style={{ backgroundColor: row.swatchCSS }}
          />
          {row.label}
        </span>
      ))}
      {showUpcomingDot && (
        <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
          <span
            aria-hidden="true"
            className="size-2 shrink-0 rounded-full"
            style={{ backgroundColor: UPCOMING_SHOW_DOT_COLOR }}
          />
          playing soon
        </span>
      )}
      {showPlayableRing && (
        <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
          <span
            aria-hidden="true"
            className="size-2.5 shrink-0 rounded-full border-[1.5px]"
            style={{ borderColor: PLAYABLE_RING_COLOR }}
          />
          playable audio
        </span>
      )}
    </div>
  )
})
