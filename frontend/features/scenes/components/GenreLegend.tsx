'use client'

/**
 * GenreLegend (PSY-1315)
 *
 * The color key for the Atlas globe's dominant-genre dot tint. A fixed key (all
 * eight families, not just the ones currently on screen) since the family ->
 * color mapping is stable and the point of a legend is to teach the whole scheme.
 * Collapsible so it doesn't crowd the globe. The open state is OWNED BY AtlasGlobe
 * (controlled via props): this component is unmounted while a scene preview is
 * open, so local state would reset the collapse on every preview open/close cycle.
 * Swatches use the same PSY-1083 `--chart-N` tokens as the dots (via
 * clusterColorCSS), so they track the theme with no JS.
 */

import { ChevronDown, ChevronUp } from 'lucide-react'
import { clusterColorCSS } from '@/components/graph/graphPalette'
import { GENRE_FAMILIES } from '../genreFamilies'
import { DOT_COLOR_BASE } from './globeScale'

interface GenreLegendProps {
  open: boolean
  onToggle: () => void
}

export function GenreLegend({ open, onToggle }: GenreLegendProps) {
  return (
    <div className="absolute bottom-4 right-4 z-10 w-44 rounded-lg border border-border bg-background/90 text-xs backdrop-blur">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        aria-controls="atlas-genre-legend"
        className="flex w-full items-center justify-between gap-2 px-3 py-1.5 font-medium text-foreground/90 transition-colors hover:text-primary"
      >
        <span>Genres</span>
        {open ? (
          <ChevronUp className="h-3.5 w-3.5" aria-hidden="true" />
        ) : (
          <ChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
        )}
      </button>
      {/* Rendered always (toggled via `hidden`) so the button's aria-controls
          target stays in the DOM when collapsed. */}
      <ul id="atlas-genre-legend" hidden={!open} className="px-3 pb-2 pt-0.5">
        {GENRE_FAMILIES.map((family) => (
          <li key={family.key} className="flex items-center gap-2 py-0.5">
            <span
              className="inline-block h-2.5 w-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: clusterColorCSS(family.colorIndex) }}
              aria-hidden="true"
            />
            <span className="text-foreground/80">{family.label}</span>
          </li>
        ))}
        <li className="mt-1 flex items-center gap-2 border-t border-border/60 py-0.5 pt-1.5">
          <span
            className="inline-block h-2.5 w-2.5 shrink-0 rounded-full"
            style={{ backgroundColor: DOT_COLOR_BASE }}
            aria-hidden="true"
          />
          <span className="text-muted-foreground">Mixed / no data</span>
        </li>
      </ul>
    </div>
  )
}
