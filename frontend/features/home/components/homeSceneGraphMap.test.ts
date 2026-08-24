import { describe, expect, it } from 'vitest'

import type { SceneGraphLink, SceneGraphNode } from '@/features/scenes/types'
import {
  HOME_GRAPH_MAX_NODES,
  buildHomeSceneGraphMap,
} from './homeSceneGraphMap'

function node(
  id: number,
  overrides: Partial<SceneGraphNode> = {},
): SceneGraphNode {
  return {
    id,
    name: `Artist ${String(id).padStart(2, '0')}`,
    slug: `artist-${id}`,
    upcoming_show_count: 0,
    cluster_id: 'other',
    is_isolate: false,
    has_playable_audio: false,
    ...overrides,
  }
}

function link(source_id: number, target_id: number): SceneGraphLink {
  return {
    source_id,
    target_id,
    type: 'shared_bills',
    score: 1,
    is_cross_cluster: false,
  }
}

describe('buildHomeSceneGraphMap', () => {
  it('caps connected artists by degree + upcoming-show activity and filters dangling links', () => {
    const nodes = Array.from({ length: 23 }, (_, index) => node(index + 1))
    nodes.push(node(99, { is_isolate: true, upcoming_show_count: 50 }))
    const links = [
      ...Array.from({ length: 22 }, (_, index) => link(index + 1, index + 2)),
      // Artist 23 is forced into the cap by current activity; its edge survives.
      link(23, 1),
    ]
    nodes[22] = node(23, { upcoming_show_count: 8 })

    const map = buildHomeSceneGraphMap(nodes, links)

    expect(map.nodes).toHaveLength(HOME_GRAPH_MAX_NODES)
    expect(map.nodes[0].id).toBe(23)
    expect(map.nodes.some(item => item.id === 99)).toBe(false)
    const kept = new Set(map.nodes.map(item => item.id))
    expect(
      map.links.every(item => kept.has(item.source_id) && kept.has(item.target_id)),
    ).toBe(true)
    const incidentIds = new Set(
      map.links.flatMap(item => [item.source_id, item.target_id]),
    )
    expect(map.nodes.every(item => incidentIds.has(item.id))).toBe(true)
  })

  it('assigns deterministic 17/13/11 terciles and picks only the top two booked artists for chips', () => {
    const nodes = Array.from({ length: 9 }, (_, index) =>
      node(index + 1, {
        upcoming_show_count: 9 - index,
        next_show:
          index < 4
            ? {
                id: 100 + index,
                event_date: '2026-07-17T02:00:00Z',
                venue_name: `Venue ${index + 1}`,
                venue_city: 'Phoenix',
                venue_state: 'AZ',
                venue_timezone: 'America/Phoenix',
              }
            : undefined,
      }),
    )

    const links = [
      ...Array.from({ length: 8 }, (_, index) => link(index + 1, index + 2)),
      link(9, 1),
    ]
    const map = buildHomeSceneGraphMap(nodes, links)

    expect(map.nodes.map(item => item.id)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9])
    expect([...map.labelStyles.values()]).toEqual([
      { fontSize: 17, fontWeight: 600 },
      { fontSize: 17, fontWeight: 600 },
      { fontSize: 17, fontWeight: 600 },
      { fontSize: 13, fontWeight: 500 },
      { fontSize: 13, fontWeight: 500 },
      { fontSize: 13, fontWeight: 500 },
      { fontSize: 11, fontWeight: 400 },
      { fontSize: 11, fontWeight: 400 },
      { fontSize: 11, fontWeight: 400 },
    ])
    expect(map.showChipNodes.map(item => item.id)).toEqual([1, 2])
  })

  it('backfills lower-ranked partners when top activity nodes form disjoint pairs', () => {
    const nodes = Array.from({ length: 44 }, (_, index) =>
      node(index + 1, { upcoming_show_count: index < 22 ? 100 - index : 0 }),
    )
    const links = Array.from({ length: 22 }, (_, index) => link(index + 1, index + 23))

    const map = buildHomeSceneGraphMap(nodes, links)

    expect(map.nodes).toHaveLength(HOME_GRAPH_MAX_NODES)
    const incidentIds = new Set(map.links.flatMap(item => [item.source_id, item.target_id]))
    expect(map.nodes.every(item => incidentIds.has(item.id))).toBe(true)
  })
})

describe('buildHomeSceneGraphMap — label hubs (PSY-1530)', () => {
  // The teaser INHERITS hubs (locked decision). Excluding them is not a safe
  // simplification: the backend replaces each roster's pairwise clique with hub
  // spokes, so dropping hubs drops that connectivity too.
  const hub = node(2_000_000_007, {
    name: '12XU',
    slug: '12xu',
    entity_type: 'label',
    cluster_id: '',
  })
  const spoke = (artistId: number): SceneGraphLink => ({
    source_id: 2_000_000_007,
    target_id: artistId,
    type: 'on_label',
    score: 1,
    is_cross_cluster: false,
  })

  it('includes label hubs in the teaser node set', () => {
    const map = buildHomeSceneGraphMap(
      [node(1), node(2), node(3), hub],
      [spoke(1), spoke(2), spoke(3)],
    )
    expect(map.nodes.map(n => n.id)).toContain(2_000_000_007)
  })

  // The regression this guards: on a label-dominated scene (Austin is 300 of
  // 302 edges from one label) the ONLY connectivity is the hub's spokes, so
  // dropping hubs would collapse the map to nothing.
  it('does not collapse when a label carries all the connectivity', () => {
    const artists = [node(1), node(2), node(3), node(4), node(5)]
    const map = buildHomeSceneGraphMap(
      [...artists, hub],
      artists.map(a => spoke(a.id)),
    )
    expect(map.nodes.length).toBeGreaterThanOrEqual(artists.length)
    expect(map.links.length).toBeGreaterThan(0)
    // Every roster artist reaches the map through its spoke.
    for (const a of artists) {
      expect(map.nodes.map(n => n.id)).toContain(a.id)
    }
  })

  // HomeSceneGraph publishes this blend as user-facing copy ("Name size =
  // connections across the scene plus upcoming shows"), and nothing else fails
  // when the blend changes — the copy just silently becomes false again, which
  // is the exact regression class PSY-1904/PSY-1906 exist to close. So pin the
  // two inputs and their equal weight here, at the site that owns them.
  it('weights degree and upcoming shows equally, so the published copy stays true', () => {
    // Degree-only would rank the hub-less pair first; upcoming-only would rank
    // the booked isolate-ish node first. Only the SUM puts node 3 on top.
    const nodes = [
      node(1, { upcoming_show_count: 0 }), // degree 2 + 0 = 2
      node(2, { upcoming_show_count: 0 }), // degree 2 + 0 = 2
      node(3, { upcoming_show_count: 3 }), // degree 1 + 3 = 4
      node(4, { upcoming_show_count: 0 }), // degree 1 + 0 = 1
    ]
    const map = buildHomeSceneGraphMap(nodes, [
      link(1, 2),
      link(1, 4),
      link(2, 3),
    ])
    // Label tiers are assigned in activity-rank order, so the largest tier
    // belongs to the highest blended score, not the highest degree.
    const largest = [...map.labelStyles.entries()].filter(
      ([, style]) => style.fontSize === 17,
    )
    expect(largest.map(([id]) => id)).toContain(3)
    expect(map.labelStyles.get(3)!.fontSize).toBeGreaterThan(
      map.labelStyles.get(4)!.fontSize,
    )
  })
})
