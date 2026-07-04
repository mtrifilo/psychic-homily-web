'use client'

/**
 * useConnectionProvenance (PSY-1335) — fetches the entities behind each typed
 * connection for the inspected pair and merges them into the panel rows.
 *
 * Lives in components/graph (not a feature module) because both panel hosts —
 * the shared ForceGraphView and the ego ArtistGraph — mount it; importing a
 * feature module from here would invert the feature → components dependency
 * direction. The query key gets its own 'graph' namespace for the same reason.
 *
 * Deliberate deviation from the ticket's keepPreviousData suggestion: the
 * query keys on the PAIR, so "previous data" across a key change is the
 * PREVIOUS pair's entities — merging those under the new pair's header shows
 * wrong provenance, which is worse than the loading gap. The never-blank
 * requirement is met by the phase-1 text rows, which render from graph data
 * regardless of this query's state (loading/error simply means no entity
 * links yet). The PSY-1305 overlay contract is untouched: this query never
 * feeds the overlay's `available` signal, so fetch transients can't eject
 * fullscreen. Retry policy is the app-wide default (lib/queryClient.ts:
 * no retry on 4xx — a 404 means no stored connection).
 */

import { useQuery } from '@tanstack/react-query'

import { apiRequest } from '@/lib/api'
import { API_BASE_URL } from '@/lib/api-base'
import { orderEdgeTypes, type EdgeTooltipLink } from './edgeGrammar'
import type { ConnectionEntity, PanelConnection } from './ConnectionPanel'
import type { InspectedPair } from './useConnectionInspect'

const PROVENANCE_STALE_TIME = 5 * 60 * 1000 // matches the graph queries

export interface ProvenanceConnection {
  type: string
  score: number
  detail?: unknown
  entities?: ConnectionEntity[]
  entity_total?: number
}

export interface RelationshipProvenance {
  connections: ProvenanceConnection[]
}

/** Canonical (sorted) pair — one cache entry regardless of click orientation. */
function canonicalPair(pair: InspectedPair): [number, number] {
  return pair.sourceId < pair.targetId
    ? [pair.sourceId, pair.targetId]
    : [pair.targetId, pair.sourceId]
}

export function connectionProvenanceQueryKey(pair: InspectedPair) {
  const [lo, hi] = canonicalPair(pair)
  return ['graph', 'connection-provenance', String(lo), String(hi)] as const
}

/**
 * Fetch provenance for the inspected pair. Disabled while no pair is
 * selected. `enabled` lets a host that gates the panel feature itself (e.g.
 * ForceGraphView's showConnectionPanel opt-in) keep the query off even if a
 * stale pair lingers for a render — a 404 (no stored connection) resolves to
 * "no entities" via the panel keeping its text rows.
 */
export function useConnectionProvenance(pair: InspectedPair | null, enabled = true) {
  const canonical = pair ? canonicalPair(pair) : null
  return useQuery({
    queryKey: pair
      ? connectionProvenanceQueryKey(pair)
      : (['graph', 'connection-provenance', 'idle'] as const),
    queryFn: (): Promise<RelationshipProvenance> =>
      apiRequest<RelationshipProvenance>(
        // enabled gates on canonical — non-null when this runs.
        `${API_BASE_URL}/artists/${canonical![0]}/relationships/${canonical![1]}/provenance`,
        { method: 'GET' },
      ),
    enabled: enabled && canonical !== null,
    staleTime: PROVENANCE_STALE_TIME,
  })
}

/**
 * Merge provenance entities into the panel's aggregated connections, by type.
 *
 * - Rows the graph payload asserts are never dropped; unmatched ones pass
 *   through unchanged (text-only).
 * - Connection types ONLY the endpoint knows are APPENDED: the backend unions
 *   query-time signals (festival_cobill) that most graph surfaces don't
 *   request, and the panel's contract is ALL typed connections between the
 *   pair — without the append, that union would be unreachable and the same
 *   pair would show different connection sets per surface.
 * - The combined list is re-ranked by the edge grammar's canonical order so
 *   appended rows don't trail arbitrarily.
 */
export function mergeProvenanceEntities(
  connections: EdgeTooltipLink[],
  provenance: RelationshipProvenance | undefined,
): PanelConnection[] {
  if (!provenance || provenance.connections.length === 0) return connections

  const byType = new Map(provenance.connections.map(conn => [conn.type, conn]))
  const merged: PanelConnection[] = connections.map(conn => {
    const match = conn.type ? byType.get(conn.type) : undefined
    if (!match?.entities || match.entities.length === 0) return conn
    return {
      ...conn,
      entities: match.entities,
      entityTotal: match.entity_total,
    }
  })

  const graphTypes = new Set(connections.map(conn => conn.type))
  const appended: PanelConnection[] = provenance.connections
    .filter(conn => conn.type && !graphTypes.has(conn.type))
    .map(conn => ({
      type: conn.type,
      score: conn.score,
      detail: conn.detail as Record<string, unknown> | undefined,
      entities: conn.entities?.length ? conn.entities : undefined,
      entityTotal: conn.entity_total,
    }))
  if (appended.length === 0) return merged

  const all = [...merged, ...appended]
  const rank = new Map(orderEdgeTypes(all.map(conn => conn.type)).map((t, i) => [t, i]))
  return all.sort((a, b) => (rank.get(a.type) ?? 0) - (rank.get(b.type) ?? 0))
}
