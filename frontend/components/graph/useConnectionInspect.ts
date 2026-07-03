'use client'

/**
 * useConnectionInspect (PSY-1334)
 *
 * Selection state for the click-to-inspect ConnectionPanel: which artist
 * pair (if any) is being inspected. One pair at a time — opening a new pair
 * replaces the previous selection (the panel re-targets rather than stacks).
 *
 * Shared by ForceGraphView (scene / station / venue / collection surfaces)
 * and ArtistGraph (the ego graph renders its own canvas but mounts the same
 * panel), so the interaction contract can't drift between the two.
 */

import { useCallback, useMemo, useState } from 'react'

import { type EdgeTooltipLink, orderEdgeTypes } from './edgeGrammar'

/** Unordered artist pair under inspection. */
export interface InspectedPair {
  sourceId: number
  targetId: number
}

export function useConnectionInspect() {
  const [pair, setPair] = useState<InspectedPair | null>(null)

  const open = useCallback((sourceId: number, targetId: number) => {
    setPair({ sourceId, targetId })
  }, [])

  const close = useCallback(() => setPair(null), [])

  // Memoized so the returned object's identity only changes when `pair`
  // does — these graph surfaces re-render on every hover, and a fresh
  // object per render would defeat every useCallback that lists the hook
  // result in its deps (adversarial finding). `open`/`close` are stable;
  // prefer depending on those directly in hot callbacks.
  return useMemo(() => ({ pair, open, close }), [pair, open, close])
}

/**
 * Minimal link shape aggregatePairConnections needs from a graph payload.
 * `detail` is typed loosely to match the payload boundary (GraphLink); the
 * tooltip/panel copy builder defends against non-object shapes field-by-field.
 */
export interface PairSourceLink {
  source_id: number
  target_id: number
  type: string
  score?: number
  votes_up?: number
  votes_down?: number
  detail?: Record<string, unknown> | unknown
}

/**
 * Collect every typed connection between an unordered artist pair, one row
 * per edge type, in canonical grammar order. Untyped links carry no
 * provenance and are skipped. Duplicate same-type links (shouldn't occur —
 * the relationships table is unique per (pair, type)) keep the first seen so
 * a malformed payload can't render duplicate rows.
 */
export function aggregatePairConnections(
  links: ReadonlyArray<PairSourceLink>,
  pair: InspectedPair,
): EdgeTooltipLink[] {
  const byType = new Map<string, EdgeTooltipLink>()
  for (const l of links) {
    if (!l.type) continue
    const matches =
      (l.source_id === pair.sourceId && l.target_id === pair.targetId) ||
      (l.source_id === pair.targetId && l.target_id === pair.sourceId)
    if (!matches) continue
    if (!byType.has(l.type)) {
      byType.set(l.type, {
        type: l.type,
        score: l.score,
        votes_up: l.votes_up,
        votes_down: l.votes_down,
        detail: l.detail as Record<string, unknown> | undefined,
      })
    }
  }
  return orderEdgeTypes([...byType.keys()]).map(t => byType.get(t)!)
}
