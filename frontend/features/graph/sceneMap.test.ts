import { describe, expect, it } from 'vitest'

import {
  SCENE_MAP_WORLD_HALF_EXTENT,
  buildSceneMap,
  decodeByteColumn,
  groupNodesByRegion,
  sceneMapColorIndex,
  type GraphOverview,
  type SceneMap,
  type SceneMapNode,
} from './sceneMap'

const QUANT_SCALE = 32767

function encodeBytes(bytes: number[]): string {
  return btoa(String.fromCharCode(...bytes))
}

/**
 * A four-node map: three artists in one community plus a label hub, wired
 * A—B, B—C and hub→A as a spoke. Small enough to assert every column by hand,
 * complete enough that every decode rule has something to bite on.
 */
function overviewFixture(overrides: Partial<GraphOverview> = {}): GraphOverview {
  return {
    version: 1,
    last_mapped: '2026-08-02T04:00:00Z',
    epoch: '2020-01-01T00:00:00Z',
    extent: 500,
    node_count: 4,
    edge_count: 3,
    isolate_count: 12,
    rank_metric: 'betweenness',
    hull_kind: 'convex',
    nodes: {
      id: [101, 102, 103, 900001],
      // artist, artist, artist, label hub
      kind: encodeBytes([0, 0, 0, 1]),
      name: ['Alpha', 'Beta', 'Gamma', 'Doom Records'],
      slug: ['alpha', 'beta', 'gamma', 'doom-records'],
      x: [0, QUANT_SCALE, -QUANT_SCALE, 0],
      y: [0, 0, 0, QUANT_SCALE],
      community: [7, 7, 7, -1],
      degree: [2, 2, 1, 1],
      rank: [0, 1, 2, 3],
      // Beta has an upcoming show; Gamma has playable audio.
      flags: encodeBytes([0, 0b10, 0b01, 0]),
      appear: [0, 0, 0, 0],
    },
    edges: {
      // CSR: A↔B, B↔C, A↔hub. Both directions present.
      offsets: [0, 2, 4, 5, 6],
      targets: [1, 3, 0, 2, 1, 0],
      kind: encodeBytes([0, 1, 0, 0, 0, 1]),
      appear: [0, 0, 0, 0, 0, 0],
    },
    regions: [
      {
        community: 7,
        label: 'Around Alpha',
        member_count: 3,
        hull: [
          [-QUANT_SCALE, -QUANT_SCALE],
          [QUANT_SCALE, -QUANT_SCALE],
          [0, QUANT_SCALE],
        ],
      },
    ],
    ...overrides,
  } as GraphOverview
}

describe('decodeByteColumn', () => {
  it('decodes a base64 column to its bytes', () => {
    expect(Array.from(decodeByteColumn(encodeBytes([0, 1, 255]), 3)!)).toEqual([0, 1, 255])
  })

  it('refuses a column whose length disagrees with the count it indexes', () => {
    expect(decodeByteColumn(encodeBytes([0, 1]), 3)).toBeNull()
  })

  it('refuses input that is not base64', () => {
    expect(decodeByteColumn('not base64!!', 3)).toBeNull()
  })
})

describe('buildSceneMap', () => {
  it('decodes the columnar payload into drawable nodes', () => {
    const map = buildSceneMap(overviewFixture())!

    expect(map.nodes).toHaveLength(4)
    expect(map.nodes[0]).toMatchObject({
      id: 101,
      kind: 'artist',
      name: 'Alpha',
      slug: 'alpha',
      community: 7,
      degree: 2,
      rank: 0,
    })
    expect(map.nodes[3].kind).toBe('label')
  })

  it('decodes the base64 flag bitfield per node', () => {
    const map = buildSceneMap(overviewFixture())!

    expect(map.nodes.map(n => n.hasUpcomingShow)).toEqual([false, true, false, false])
    expect(map.nodes.map(n => n.hasPlayableAudio)).toEqual([false, false, true, false])
  })

  it('rescales quantized coordinates into the fixed world extent', () => {
    const map = buildSceneMap(overviewFixture())!

    expect(map.nodes[0].x).toBe(0)
    expect(map.nodes[1].x).toBeCloseTo(SCENE_MAP_WORLD_HALF_EXTENT, 6)
    expect(map.nodes[2].x).toBeCloseTo(-SCENE_MAP_WORLD_HALF_EXTENT, 6)
    expect(map.nodes[3].y).toBeCloseTo(SCENE_MAP_WORLD_HALF_EXTENT, 6)
  })

  it('emits each CSR edge exactly once, keyed by entity id', () => {
    const map = buildSceneMap(overviewFixture())!

    // Six neighbour slots describe three edges; the mirror slots are dropped.
    expect(map.edges).toEqual([
      { source: 101, target: 102, kind: 'similarity' },
      { source: 101, target: 900001, kind: 'spoke' },
      { source: 102, target: 103, kind: 'similarity' },
    ])
  })

  it('counts artists and label hubs separately, and reports isolates verbatim', () => {
    const map = buildSceneMap(overviewFixture())!

    expect(map.artistCount).toBe(3)
    expect(map.labelCount).toBe(1)
    expect(map.isolateCount).toBe(12)
  })

  it('rescales region hulls and anchors the caption at their centroid', () => {
    const map = buildSceneMap(overviewFixture())!

    expect(map.regions).toHaveLength(1)
    expect(map.regions[0].label).toBe('Around Alpha')
    expect(map.regions[0].hull).toHaveLength(3)
    expect(map.regions[0].hull[1][0]).toBeCloseTo(SCENE_MAP_WORLD_HALF_EXTENT, 6)
    expect(map.regions[0].captionAnchor![0]).toBeCloseTo(0, 6)
    expect(map.regions[0].captionAnchor![1]).toBeCloseTo(-SCENE_MAP_WORLD_HALF_EXTENT / 3, 6)
  })

  it('leaves a region with no hull anchor-less but still counted', () => {
    const map = buildSceneMap(
      overviewFixture({
        regions: [{ community: 7, label: 'Around Alpha', member_count: 2, hull: [] }],
      }),
    )!

    expect(map.regions).toHaveLength(1)
    expect(map.regions[0].hull).toEqual([])
    expect(map.regions[0].captionAnchor).toBeNull()
  })

  it('refuses a payload written by a newer schema version', () => {
    expect(buildSceneMap(overviewFixture({ version: 2 }))).toBeNull()
  })

  it('refuses an empty map', () => {
    expect(buildSceneMap(overviewFixture({ node_count: 0 }))).toBeNull()
  })

  it('refuses a node column shorter than node_count', () => {
    const overview = overviewFixture()
    overview.nodes.name = ['Alpha', 'Beta']
    expect(buildSceneMap(overview)).toBeNull()
  })

  it('refuses a byte column whose decoded length disagrees with node_count', () => {
    const overview = overviewFixture()
    overview.nodes.flags = encodeBytes([0, 0])
    expect(buildSceneMap(overview)).toBeNull()
  })

  it('drops the edges but keeps the map when the CSR offsets are malformed', () => {
    const overview = overviewFixture()
    overview.edges.offsets = [0, 2]
    const map = buildSceneMap(overview)!

    expect(map.nodes).toHaveLength(4)
    expect(map.edges).toEqual([])
  })

  it('degrades every edge to similarity when the kind column cannot be read', () => {
    const overview = overviewFixture()
    overview.edges.kind = encodeBytes([0])
    const map = buildSceneMap(overview)!

    expect(map.edges.map(edge => edge.kind)).toEqual([
      'similarity',
      'similarity',
      'similarity',
    ])
  })
})

describe('sceneMapColorIndex', () => {
  it('wraps community ids around the ramp', () => {
    expect(sceneMapColorIndex(0, 8)).toBe(0)
    expect(sceneMapColorIndex(7, 8)).toBe(7)
    expect(sceneMapColorIndex(8, 8)).toBe(0)
  })

  it('returns the no-cluster sentinel for a node in no community', () => {
    expect(sceneMapColorIndex(-1, 8)).toBe(-1)
  })
})

// ── Grouping the map for the list view ────────────────────────────────────

function node(
  overrides: Partial<SceneMapNode> & Pick<SceneMapNode, 'id' | 'name'>,
): SceneMapNode {
  return {
    kind: 'artist',
    slug: overrides.name.toLowerCase(),
    x: 0,
    y: 0,
    community: 7,
    degree: 1,
    rank: 0,
    hasUpcomingShow: false,
    hasPlayableAudio: false,
    ...overrides,
  }
}

function sceneMapFixture(overrides: Partial<SceneMap> = {}): SceneMap {
  return {
    nodes: [
      node({ id: 1, name: 'Alpha', rank: 0 }),
      node({ id: 2, name: 'Beta', rank: 1 }),
      node({ id: 3, name: 'Gamma', rank: 2, community: 9 }),
      node({ id: 900001, name: 'Doom Records', kind: 'label', community: -1, degree: 4 }),
    ],
    edges: [],
    regions: [
      { community: 7, label: 'Around Alpha', memberCount: 2, hull: [], captionAnchor: null },
      { community: 9, label: 'Around Gamma', memberCount: 1, hull: [], captionAnchor: null },
    ],
    artistCount: 3,
    labelCount: 1,
    isolateCount: 42,
    lastMapped: new Date('2026-08-02T04:00:00Z'),
    ...overrides,
  }
}

describe('groupNodesByRegion', () => {
  it('groups artists under their region, biggest first, most central first', () => {
    const groups = groupNodesByRegion(sceneMapFixture())

    expect(groups.map(g => g.label)).toEqual(['Around Alpha', 'Around Gamma'])
    expect(groups[0].nodes.map(n => n.name)).toEqual(['Alpha', 'Beta'])
  })

  it('leaves label hubs out — a hub opens a card, it does not re-root', () => {
    const groups = groupNodesByRegion(sceneMapFixture())

    expect(groups.flatMap(g => g.nodes).map(n => n.name)).not.toContain('Doom Records')
  })

  it('collects artists whose community has no region so the list is never short', () => {
    const map = sceneMapFixture({
      regions: [
        { community: 7, label: 'Around Alpha', memberCount: 2, hull: [], captionAnchor: null },
      ],
    })

    const groups = groupNodesByRegion(map)

    expect(groups.map(g => g.label)).toEqual(['Around Alpha', 'Elsewhere on the map'])
    expect(groups[1].nodes.map(n => n.name)).toEqual(['Gamma'])
    // Every artist on the map is reachable from the list.
    expect(groups.flatMap(g => g.nodes)).toHaveLength(map.artistCount)
  })
})
