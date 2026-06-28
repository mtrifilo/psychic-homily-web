import { describe, it, expect } from 'vitest'

import { mergeEgoGraphs } from './mergeEgoGraphs'
import type { ArtistGraph, ArtistGraphLink, ArtistGraphNode } from '../types'

// PSY-1259 — the pure merge + hop-assignment behind expand-on-demand. The canvas just
// draws the result; this is the data model, tested in isolation like the other graph helpers.

const node = (id: number): ArtistGraphNode => ({
  id,
  name: `a${id}`,
  slug: `a${id}`,
  upcoming_show_count: 0,
})

const link = (s: number, t: number, type = 'similar', score = 0.5): ArtistGraphLink => ({
  source_id: s,
  target_id: t,
  type,
  score,
  votes_up: 0,
  votes_down: 0,
})

// An ego payload: `centerId`'s graph with the given neighbor ids and links.
const ego = (centerId: number, neighborIds: number[], links: ArtistGraphLink[]): ArtistGraph => ({
  center: node(centerId),
  nodes: neighborIds.map(node),
  links,
  user_votes: {},
})

const noExpansions = new Map<number, ArtistGraph>()

describe('mergeEgoGraphs', () => {
  it('with no expansions, returns the base ego graph with all satellites at hop 1', () => {
    const base = ego(1, [2, 3], [link(1, 2), link(1, 3), link(2, 3)])
    const merged = mergeEgoGraphs(base, noExpansions)

    expect(merged.center.id).toBe(1)
    expect(merged.nodes.map(n => n.id).sort()).toEqual([2, 3]) // center excluded from nodes
    expect(merged.links).toHaveLength(3)
    expect(merged.hopByNodeId.get(1)).toBe(0) // center hop 0 (kept in the map)
    expect(merged.hopByNodeId.get(2)).toBe(1)
    expect(merged.hopByNodeId.get(3)).toBe(1)
  })

  it('places an expanded node\'s new neighbors on the next ring (hop 2)', () => {
    const base = ego(1, [2, 3], [link(1, 2), link(1, 3)])
    const exp2 = ego(2, [4, 5], [link(2, 4), link(2, 5)]) // expand node 2
    const merged = mergeEgoGraphs(base, new Map([[2, exp2]]))

    expect(merged.nodes.map(n => n.id).sort()).toEqual([2, 3, 4, 5])
    expect(merged.hopByNodeId.get(4)).toBe(2)
    expect(merged.hopByNodeId.get(5)).toBe(2)
    expect(merged.links).toHaveLength(4) // 1-2, 1-3, 2-4, 2-5
  })

  it('dedupes a link present in both the base and an expansion', () => {
    const base = ego(1, [2, 3], [link(1, 2), link(1, 3), link(2, 3)])
    const exp2 = ego(2, [3, 4], [link(2, 3), link(2, 4)]) // 2-3 also returned here
    const merged = mergeEgoGraphs(base, new Map([[2, exp2]]))

    const key = (l: ArtistGraphLink) => `${l.source_id}|${l.target_id}|${l.type}`
    const keys = merged.links.map(key)
    expect(new Set(keys).size).toBe(keys.length) // no duplicates
    expect(keys).toContain('2|3|similar')
    expect(merged.links).toHaveLength(4) // 1-2, 1-3, 2-3, 2-4
  })

  it('assigns the MINIMUM hop when a node is reachable by two paths', () => {
    // Node 3 is a direct base neighbor (hop 1) AND reappears as node 2's neighbor (hop 2 path).
    const base = ego(1, [2, 3], [link(1, 2), link(1, 3), link(2, 3)])
    const exp2 = ego(2, [3], [link(2, 3)])
    const merged = mergeEgoGraphs(base, new Map([[2, exp2]]))

    expect(merged.hopByNodeId.get(3)).toBe(1) // stays on the inner ring, not pushed to hop 2
  })

  it('prunes collapse-orphans: removing an expansion drops nodes only reachable through it', () => {
    const base = ego(1, [2], [link(1, 2)])
    const exp2 = ego(2, [3], [link(2, 3)]) // node 3 only reachable via expanding 2

    const expanded = mergeEgoGraphs(base, new Map([[2, exp2]]))
    expect(expanded.nodes.map(n => n.id).sort()).toEqual([2, 3])

    // Collapse node 2 (drop its expansion) → node 3 has no path from center → gone.
    const collapsed = mergeEgoGraphs(base, noExpansions)
    expect(collapsed.nodes.map(n => n.id)).toEqual([2])
    expect(collapsed.hopByNodeId.has(3)).toBe(false)
  })

  it('drops links whose endpoint was pruned', () => {
    // A link 3-4 from an expansion of 2; if 2 is collapsed, 3 and 4 are unreachable and the
    // link must not survive into the merged set.
    const base = ego(1, [2], [link(1, 2)])
    const exp2 = ego(2, [3, 4], [link(2, 3), link(3, 4)])
    const collapsed = mergeEgoGraphs(base, noExpansions)
    expect(collapsed.links.map(l => `${l.source_id}-${l.target_id}`)).toEqual(['1-2'])
    // sanity: with the expansion present, 3-4 survives
    const expanded = mergeEgoGraphs(base, new Map([[2, exp2]]))
    expect(expanded.links.some(l => l.source_id === 3 && l.target_id === 4)).toBe(true)
  })

  it('handles a center with no neighbors', () => {
    const base = ego(1, [], [])
    const merged = mergeEgoGraphs(base, noExpansions)
    expect(merged.nodes).toEqual([])
    expect(merged.links).toEqual([])
    expect(merged.hopByNodeId.get(1)).toBe(0)
    expect(merged.hopByNodeId.size).toBe(1)
  })

  it('supports multi-hop chains (hop 3 via two expansions)', () => {
    const base = ego(1, [2], [link(1, 2)])
    const exp2 = ego(2, [3], [link(2, 3)])
    const exp3 = ego(3, [4], [link(3, 4)])
    const merged = mergeEgoGraphs(base, new Map([[2, exp2], [3, exp3]]))
    expect(merged.hopByNodeId.get(3)).toBe(2)
    expect(merged.hopByNodeId.get(4)).toBe(3)
  })
})
