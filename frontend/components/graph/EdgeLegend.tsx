'use client'

/**
 * EdgeLegend (PSY-1083)
 *
 * Shared legend for the typed-edge grammar: one row per edge type with a
 * swatch (color + dash pattern), the human label, an optional live count,
 * and — when `onToggleType` is provided — a show/hide toggle that filters
 * the type out of the simulation (the generalized form of the artist
 * graph's `activeTypes` Set mechanic, PSY-361/PSY-954).
 *
 * Swatch colors are `var(--edge-*)` expressions (see edgeGrammar.ts), so
 * the legend tracks theme changes with no JS re-resolution. Rows render in
 * canonical grammar order; unknown types (collection-derived edges,
 * PSY-555) sort after the canonical set and get the neutral fallback
 * style.
 *
 * Positioning is the caller's job (the graph surfaces pass
 * `absolute top-2 right-2` to float it over the canvas, matching the
 * pre-PSY-1083 ArtistGraph legend).
 */

import { type ReactNode } from 'react'

import { cn } from '@/lib/utils'
import { edgeColorCSS, edgeLineDash, edgeTypeLabel, orderEdgeTypes } from './edgeGrammar'

export interface EdgeLegendProps {
  /** Edge types to render rows for (re-ordered canonically). */
  types: ReadonlyArray<string>
  /** Per-type link counts; when provided, each row shows its live count. */
  counts?: ReadonlyMap<string, number>
  /** Types currently filtered out of the simulation (rendered dimmed). */
  hiddenTypes?: ReadonlySet<string>
  /** When provided, rows become show/hide toggle buttons. */
  onToggleType?: (type: string) => void
  /**
   * PSY-1334: the soloed type, if any. While set, only this type renders in
   * the simulation (solo wins over hiddenTypes, which stays intact
   * underneath) — the legend dims every other row and shows a disclosure
   * line so the filter is never silent.
   */
  soloType?: string | null
  /**
   * PSY-1334: solo toggle — called with the type to solo, or null to clear.
   * When provided (alongside onToggleType), each row gains an "only" button.
   */
  onSoloType?: (type: string | null) => void
  /**
   * Weight-scale affordance (PSY-362) — communicates that line thickness
   * encodes signal magnitude. Defaults on; the artist graph keeps it.
   */
  showWeightHint?: boolean
  /**
   * Optional disclosure line rendered under the rows (PSY-1258). The artist
   * graph uses it to surface its per-node top-k edge cap ("Radio Co-occurrence:
   * each artist's 5 strongest") so the cap is never silent — see CLAUDE.md
   * "no silent caps". Omitted by callers that don't cap.
   */
  footnote?: ReactNode
  className?: string
}

// Mini canvas-dash preview: the same dash arrays the canvas uses, drawn as
// an SVG stroke so the swatch shows color AND pattern (WCAG 1.4.1 — the
// dash channel is part of the grammar, the legend must teach it).
export function EdgeSwatch({ type }: { type: string }) {
  const dash = edgeLineDash(type)
  return (
    <svg width="16" height="4" viewBox="0 0 16 4" aria-hidden="true" className="shrink-0">
      <line
        x1="1"
        y1="2"
        x2="15"
        y2="2"
        stroke={edgeColorCSS(type)}
        strokeWidth="2"
        strokeLinecap="round"
        strokeDasharray={dash.length > 0 ? dash.join(' ') : undefined}
      />
    </svg>
  )
}

export function EdgeLegend({
  types,
  counts,
  hiddenTypes,
  onToggleType,
  soloType,
  onSoloType,
  showWeightHint = true,
  footnote,
  className,
}: EdgeLegendProps) {
  const ordered = orderEdgeTypes(types)
  if (ordered.length === 0) return null

  return (
    <div
      className={cn(
        'p-2 rounded-md bg-background/80 backdrop-blur-sm border border-border/50 text-xs space-y-1',
        className,
      )}
    >
      {ordered.map(type => {
        const label = edgeTypeLabel(type)
        const hidden = hiddenTypes?.has(type) ?? false
        // While a solo is active it overrides the hidden set: the soloed row
        // is the only full-opacity one, mirroring what the simulation shows.
        const dimmed = soloType ? type !== soloType : hidden
        const row = (
          <>
            <EdgeSwatch type={type} />
            <span className="text-muted-foreground">{label}</span>
            {counts && (
              <span className="ml-auto pl-2 tabular-nums text-muted-foreground/70">
                {counts.get(type) ?? 0}
              </span>
            )}
          </>
        )
        if (!onToggleType) {
          return (
            <div key={type} className="flex items-center gap-1.5">
              {row}
            </div>
          )
        }
        const soloed = soloType === type
        return (
          <div key={type} className="group flex items-center gap-1">
            <button
              type="button"
              onClick={() => onToggleType(type)}
              aria-pressed={!hidden}
              title={hidden ? `Show ${label} connections` : `Hide ${label} connections`}
              className={cn(
                'flex w-full items-center gap-1.5 rounded-sm transition-opacity hover:bg-muted/50',
                dimmed ? 'opacity-40' : 'opacity-100',
              )}
            >
              {row}
            </button>
            {/* PSY-1334: solo ("only") affordance. Always in the tab order —
                revealed on row hover for pointer users, on focus for keyboard
                users — so the isolate action is never pointer-only. */}
            {onSoloType && (
              <button
                type="button"
                onClick={() => onSoloType(soloed ? null : type)}
                aria-pressed={soloed}
                aria-label={soloed ? 'Show all connection types' : `Show only ${label} connections`}
                title={soloed ? 'Show all connection types' : `Show only ${label} connections`}
                className={cn(
                  'shrink-0 rounded-sm px-1 text-[10px] leading-4 text-muted-foreground hover:text-foreground hover:bg-muted/50',
                  'focus-visible:opacity-100 group-hover:opacity-100',
                  soloed ? 'opacity-100 text-foreground font-medium' : 'opacity-0',
                )}
              >
                only
              </button>
            )}
          </div>
        )
      })}
      {/* PSY-1334: solo disclosure — the filter must never be silent (same
          rule as the cap footnote below). */}
      {soloType && (
        <div className="pt-1 mt-1 border-t border-border/40 max-w-[12rem] text-[10px] leading-tight text-muted-foreground/80">
          Showing only {edgeTypeLabel(soloType)} connections
        </div>
      )}
      {/* PSY-362: weight-scale affordance — communicates that line thickness
          encodes signal magnitude (similarity score, shared-show count, etc.)
          so users know the visual grammar before hovering individual edges. */}
      {showWeightHint && (
        <div className="pt-1 mt-1 border-t border-border/40 flex items-center gap-1.5">
          <div className="flex flex-col items-center gap-0.5" aria-hidden="true">
            <div className="w-4 h-px rounded-full bg-muted-foreground/60" />
            <div className="w-4 h-[3px] rounded-full bg-muted-foreground" />
          </div>
          <span className="text-[10px] text-muted-foreground/80 leading-tight">
            Thicker = stronger signal
          </span>
        </div>
      )}
      {/* PSY-1258: cap-disclosure line — keeps a per-node edge cap visible instead of
          silently dropping edges. Width-capped so it doesn't stretch the floating legend. */}
      {footnote && (
        <div className="pt-1 mt-1 border-t border-border/40 max-w-[12rem] text-[10px] leading-tight text-muted-foreground/80">
          {footnote}
        </div>
      )}
    </div>
  )
}
