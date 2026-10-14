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
 * preview open/close cycle. Until the user chooses, it follows `compact`:
 * collapsed when compact, open otherwise.
 * Swatches use the same PSY-1083 `--chart-N` tokens as the dots (via
 * clusterColorCSS), so they track the theme with no JS.
 *
 * Compact, it is a content-width chip with a 14px label, as tall as the
 * Atlas's Drift button, and it opens upward with the toggle held at the
 * bottom, so the toggle never moves under a finger or under chrome above it.
 * Otherwise it is the fixed-width key with the toggle on top. Given a bounded
 * height (`max-h-*` from the caller), the open key scrolls within it at any
 * width. The caller positions it with `className`.
 */

import { ChevronDown, ChevronUp } from 'lucide-react'
import { cn } from '@/lib/utils'
import { clusterColorCSS } from '@/components/graph/graphPalette'
import { GENRE_FAMILIES } from '../genreFamilies'
import { DOT_COLOR_BASE } from './globeScale'

interface GenreLegendProps {
  /** The phone form (the chip) rather than the desktop key. */
  compact: boolean
  /** The user's choice, or null until they make one. */
  openChoice: boolean | null
  onOpenChange: (open: boolean) => void
  /** Placement from the caller (the key positions nothing itself). */
  className?: string
}

export function GenreLegend({ compact, openChoice, onOpenChange, className }: GenreLegendProps) {
  const open = openChoice ?? !compact
  return (
    <div
      className={cn(
        'flex border border-border bg-background/90 text-xs backdrop-blur',
        compact ? 'flex-col-reverse rounded' : 'w-44 flex-col rounded-lg',
        className,
      )}
    >
      <button
        type="button"
        onClick={() => onOpenChange(!open)}
        aria-expanded={open}
        aria-controls="atlas-genre-legend"
        className={cn(
          'flex w-full shrink-0 items-center justify-between gap-2 text-foreground/90 transition-colors hover:text-primary',
          compact ? 'min-h-9 px-2.5 text-sm' : 'min-h-0 px-3 py-1.5 text-xs font-medium',
        )}
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
        className={cn('min-h-0 overflow-y-auto px-3', compact ? 'pb-0.5 pt-2' : 'pb-2 pt-0.5')}
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
