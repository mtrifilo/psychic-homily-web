'use client'

/**
 * GenreLegend (PSY-1315)
 *
 * The color key for the Atlas globe's dominant-genre dot tint. A fixed key (all
 * eight families, not just the ones currently on screen) since the family ->
 * color mapping is stable and the point of a legend is to teach the whole scheme.
 * Collapsible so it doesn't crowd the globe. The user's open/closed choice is
 * OWNED BY AtlasGlobe (controlled via props): this component is unmounted while a
 * scene preview is open, so local state would reset the collapse on every
 * preview open/close cycle. Until the user chooses, it follows the viewport:
 * collapsed on a compact one, open on a wide one.
 * Swatches use the same PSY-1083 `--chart-N` tokens as the dots (via
 * clusterColorCSS), so they track the theme with no JS.
 *
 * Below `lg` it is a content-width chip with a 14px label, as tall as Drift
 * (38px), and it opens upward with the toggle held at the bottom, so the
 * toggle never moves under a finger or under chrome above it. From `lg` it is
 * the fixed-width key with the toggle on top. The caller positions it with
 * `className`.
 */

import { ChevronDown, ChevronUp } from 'lucide-react'
import { cn } from '@/lib/utils'
import { clusterColorCSS } from '@/components/graph/graphPalette'
import { GENRE_FAMILIES } from '../genreFamilies'
import { DOT_COLOR_BASE } from './globeScale'
import { useAtlasCompactViewport } from '../atlasViewport'

interface GenreLegendProps {
  /** The user's choice, or null until they make one. */
  openChoice: boolean | null
  onOpenChange: (open: boolean) => void
  /** Placement from the caller (the key positions nothing itself). */
  className?: string
}

export function GenreLegend({ openChoice, onOpenChange, className }: GenreLegendProps) {
  const compactViewport = useAtlasCompactViewport()
  const open = openChoice ?? !compactViewport
  return (
    <div
      className={cn(
        'flex flex-col-reverse rounded border border-border bg-background/90 text-xs backdrop-blur lg:block lg:w-44 lg:rounded-lg',
        className,
      )}
    >
      <button
        type="button"
        onClick={() => onOpenChange(!open)}
        aria-expanded={open}
        aria-controls="atlas-genre-legend"
        className="flex min-h-9 w-full items-center justify-between gap-2 px-2.5 text-sm text-foreground/90 transition-colors hover:text-primary lg:min-h-0 lg:px-3 lg:py-1.5 lg:text-xs lg:font-medium"
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
      <ul
        id="atlas-genre-legend"
        hidden={!open}
        className="px-3 pb-0.5 pt-2 lg:pb-2 lg:pt-0.5"
      >
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
