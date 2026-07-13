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

    const map = buildHomeSceneGraphMap(nodes, [])

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
})
