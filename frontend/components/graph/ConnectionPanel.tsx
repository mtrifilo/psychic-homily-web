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

import { useEffect } from 'react'
import Link from 'next/link'
import { X } from 'lucide-react'

import { cn } from '@/lib/utils'
import { buildLinkLabelText, edgeTypeLabel, type EdgeTooltipLink } from './edgeGrammar'
import { EdgeSwatch } from './EdgeLegend'

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
  // Esc closes. Document-level so it works while canvas has focus; the panel
  // is non-modal (no focus trap) — it's an inspector, not a dialog.
  //
  // Layered dismiss (adversarial finding, 3 lenses): the fullscreen overlay
  // ALSO listens for Escape on document, so one keypress would close the
  // panel AND eject the user from fullscreen. Claim the key in the CAPTURE
  // phase (fires before any bubble-phase document listener regardless of
  // registration order) and preventDefault + stopPropagation; the overlay
  // hook skips defaultPrevented events — innermost layer closes first.
  // Guarded on connections.length: hooks run even when the render below
  // bails to null, so an empty-connections mount must not register an
  // invisible listener that silently eats Escape from the fullscreen
  // overlay (self-review finding — latent, both call sites gate upstream).
  const hasConnections = connections.length > 0
  useEffect(() => {
    if (!hasConnections) return
    const onKeyDown = (e: KeyboardEvent) => {
      // defaultPrevented guard + stopImmediatePropagation: sibling panels
      // (ArtistContextPanel, PSY-1345) also listen on document/capture, and
      // stopPropagation alone does not stop same-target listeners — without
      // this pair, one Esc closes both panels.
      if (e.key !== 'Escape' || e.defaultPrevented) return
      e.preventDefault()
      e.stopImmediatePropagation()
      onClose()
    }
    document.addEventListener('keydown', onKeyDown, { capture: true })
    return () => document.removeEventListener('keydown', onKeyDown, { capture: true })
  }, [onClose, hasConnections])

  if (connections.length === 0) return null

  return (
    <section
      aria-label={`Why ${source.name} and ${target.name} are connected`}
      className={cn(
        'w-72 max-w-[calc(100%-1rem)] max-h-[60%] overflow-y-auto rounded-md border border-border/50',
        'bg-background/95 backdrop-blur-sm p-3 text-xs shadow-lg space-y-2',
        className,
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="text-sm leading-snug">
          <EndpointName endpoint={source} />
          <span className="text-muted-foreground px-1" aria-hidden="true">
            ↔
          </span>
          <EndpointName endpoint={target} />
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close connection details"
          className="shrink-0 rounded-sm p-0.5 text-muted-foreground hover:text-foreground hover:bg-muted/50"
        >
          <X className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      </div>

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
                {conn.entities.map(entity => (
                  <li key={`${entity.kind}-${entity.id}`} className="leading-snug">
                    <Link
                      href={`${ENTITY_ROUTES[entity.kind]}/${encodeURIComponent(entity.slug)}`}
                      className="text-muted-foreground hover:text-foreground hover:underline"
                    >
                      {entity.date ? `${entity.date} · ${entity.name}` : entity.name}
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}
