'use client'

/**
 * ConnectionPanel (PSY-1334) — the click-to-inspect "why connected?" card.
 *
 * Opens when the user clicks an edge on a graph surface and lists EVERY
 * typed connection between that artist pair, one row per edge type: the
 * grammar swatch (color + dash), the type label, and the same provenance
 * copy the canvas hover tooltip shows (shared source: buildLinkLabelText —
 * the panel and tooltip can never drift). Unlike the canvas tooltip this is
 * a DOM surface, so it works on touch and can carry links.
 *
 * Phase-2 contract (PSY-1335): each connection row accepts an optional
 * `entities` array; when present the row renders the entities as links into
 * the knowledge graph (shared shows, label pages, stations). Phase 1 ships
 * text-only rows because the `detail` blobs carry names, not IDs.
 *
 * Trust boundary: all strings render through React text nodes (auto-escaped).
 * Never dangerouslySetInnerHTML here — detail carries community-contributed
 * entity names (same XSS surface buildLinkLabel escapes for the canvas).
 *
 * Positioning is the caller's job (the graph surfaces float it bottom-left
 * over the canvas). The panel renders inside the graph section container, so
 * it survives the fullscreen overlay (useFullscreenGraphOverlay) for free —
 * and because it's DOM outside the <canvas>, clicks inside it do NOT trip
 * the PSY-1321 zoomToFit canvas pointerdown cancel.
 */

import Link from 'next/link'

import { cn } from '@/lib/utils'
import { buildLinkLabelText, edgeTypeLabel, type EdgeTooltipLink } from './edgeGrammar'
import { EdgeSwatch } from './EdgeLegend'
import { GraphPanelShell } from './GraphPanelShell'
import { useCaptureEscape } from './useCaptureEscape'

/** Phase-2 (PSY-1335) provenance entity — rendered as a link when present. */
export interface ConnectionEntity {
  kind: 'show' | 'label' | 'festival' | 'station'
  id: number
  slug: string
  name: string
  /** ISO date for dated kinds (shows). */
  date?: string
}

const ENTITY_ROUTES: Record<ConnectionEntity['kind'], string> = {
  show: '/shows',
  label: '/labels',
  festival: '/festivals',
  station: '/radio',
}

export interface PanelConnection extends EdgeTooltipLink {
  /** Phase-2: resolvable entities behind the claim. Absent in phase 1. */
  entities?: ConnectionEntity[]
  /**
   * Phase-2: uncapped entity count. When it exceeds entities.length the row
   * discloses "and N more" (no silent caps — same rule as the EdgeLegend
   * footnote).
   */
  entityTotal?: number
}

export interface ConnectionPanelEndpoint {
  name: string
  /** When present the endpoint name links to /artists/{slug}. */
  slug?: string
}

export interface ConnectionPanelProps {
  source: ConnectionPanelEndpoint
  target: ConnectionPanelEndpoint
  /** Pre-aggregated, canonically ordered (see aggregatePairConnections). */
  connections: PanelConnection[]
  onClose: () => void
  className?: string
}

function EndpointName({ endpoint }: { endpoint: ConnectionPanelEndpoint }) {
  if (!endpoint.slug) return <span className="font-medium">{endpoint.name}</span>
  // encodeURIComponent pins the slug to ONE path segment — a malformed or
  // hostile slug ("../admin", "a/b?x") can't traverse to another route.
  return (
    <Link href={`/artists/${encodeURIComponent(endpoint.slug)}`} className="font-medium hover:underline">
      {endpoint.name}
    </Link>
  )
}

export function ConnectionPanel({
  source,
  target,
  connections,
  onClose,
  className,
}: ConnectionPanelProps) {
  // Esc closes, coordinated innermost-first with ArtistContextPanel (PSY-1360).
  // No ignoreFromInput guard: this panel must stay dismissable while the canvas
  // has focus, and its ego-graph case (Escape targeted inside a Radix <Dialog>)
  // is handled at the Dialog boundary — ArtistGraphDialog's onEscapeKeyDown
  // preventDefaults first, so this listener's defaultPrevented check defers to
  // it (PSY-1351), rather than this panel outranking every layer everywhere.
  //
  // enabled gate: hooks run even when the render below bails to null, so an
  // empty-connections mount must not register a listener that silently eats
  // Escape from the fullscreen overlay (both call sites gate upstream, but this
  // keeps the invariant local).
  const hasConnections = connections.length > 0
  useCaptureEscape(onClose, { enabled: hasConnections })

  if (connections.length === 0) return null

  return (
    <GraphPanelShell
      ariaLabel={`Why ${source.name} and ${target.name} are connected`}
      closeLabel="Close connection details"
      onClose={onClose}
      className={cn('max-h-[60%] p-3 space-y-2', className)}
      header={
        <div className="text-sm leading-snug">
          <EndpointName endpoint={source} />
          <span className="text-muted-foreground px-1" aria-hidden="true">
            ↔
          </span>
          <EndpointName endpoint={target} />
        </div>
      }
    >
      <ul className="space-y-2">
        {connections.map(conn => (
          <li key={conn.type} className="space-y-0.5">
            <div className="flex items-center gap-1.5">
              <EdgeSwatch type={conn.type} />
              <span className="font-medium text-muted-foreground">
                {edgeTypeLabel(conn.type)}
              </span>
            </div>
            {/* pl-[22px] = swatch width (16) + row gap (6): aligns the copy
                under the type label, past the swatch. */}
            <p className="pl-[22px] text-foreground/90 leading-snug">
              {buildLinkLabelText(conn)}
            </p>
            {conn.entities && conn.entities.length > 0 && (
              <ul className="pl-[22px] space-y-0.5">
                {conn.entities.map(entity => {
                  // Entities arrive from the wire: a kind this build doesn't
                  // know (backend added one without a lockstep FE deploy) must
                  // degrade to no link, not an href of "undefined/…".
                  const route = ENTITY_ROUTES[entity.kind]
                  if (!route) return null
                  return (
                    <li key={`${entity.kind}-${entity.id}`} className="leading-snug">
                      <Link
                        href={`${route}/${encodeURIComponent(entity.slug)}`}
                        className="text-muted-foreground hover:text-foreground hover:underline"
                      >
                        {entity.date ? `${entity.date} · ${entity.name}` : entity.name}
                      </Link>
                    </li>
                  )
                })}
                {conn.entityTotal !== undefined && conn.entityTotal > conn.entities.length && (
                  <li className="leading-snug text-muted-foreground/70">
                    and {conn.entityTotal - conn.entities.length} more
                  </li>
                )}
              </ul>
            )}
          </li>
        ))}
      </ul>
    </GraphPanelShell>
  )
}
