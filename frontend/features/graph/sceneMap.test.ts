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
      // Hub-scoped: empty at every artist index, set only where a label has a
      // city on file.
      hub_city: ['', '', '', 'Brooklyn'],
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

  // ── The growth replay's clock (PSY-1737) ───────────────────────────────
  it('carries the arrival time of every node and the epoch it counts from', () => {
    const map = buildSceneMap(
      overviewFixture({
        nodes: { ...overviewFixture().nodes, appear: [10, 20, 30, 40] },
      }),
    )!

    expect(map.nodes.map(node => node.appear)).toEqual([10, 20, 30, 40])
    expect(map.epoch.toISOString()).toBe('2020-01-01T00:00:00.000Z')
  })

  it('reads the edge appear column PER SLOT, not per edge', () => {
    // Slot order is [B, hub, A, C, B, A]; the surviving edges are slots 0, 1
    // and 3. Reading these by EDGE index would date every line off an
    // unrelated slot, which no assertion on a uniform fixture could catch.
    const map = buildSceneMap(
      overviewFixture({
        nodes: { ...overviewFixture().nodes, appear: [0, 0, 0, 0] },
        edges: { ...overviewFixture().edges, appear: [11, 22, 0, 33, 0, 0] },
      }),
    )!

    expect(map.edges.map(edge => edge.appear)).toEqual([11, 22, 33])
  })

  it('floors an edge at the later of its two endpoints', () => {
    // A snapshot claiming an edge predates one of its dots must not put a line
    // on the map before the dot it reaches.
    const map = buildSceneMap(
      overviewFixture({
        nodes: { ...overviewFixture().nodes, appear: [5, 900, 0, 0] },
        edges: { ...overviewFixture().edges, appear: [0, 0, 0, 0, 0, 0] },
      }),
    )!

    const edge = map.edges.find(candidate => candidate.target === 102)!
    expect(edge.appear).toBe(900)
  })

  it('still draws a map whose appear columns are missing or the wrong length', () => {
    // Every other column decides where a dot GOES; these only feed the optional
    // replay, so a snapshot without them is drawable and merely unwatchable.
    const map = buildSceneMap(
      overviewFixture({
        nodes: { ...overviewFixture().nodes, appear: [1, 2] },
        edges: { ...overviewFixture().edges, appear: null },
      }),
    )

    expect(map).not.toBeNull()
    expect(map!.nodes.map(node => node.appear)).toEqual([0, 0, 0, 0])
    expect(map!.edges.every(edge => edge.appear === 0)).toBe(true)
  })

  // ── The label hub's home city (PSY-1736) ───────────────────────────────
  it('carries a hub home city only where the snapshot sets one', () => {
    const map = buildSceneMap(overviewFixture())!

    expect(map.nodes.map(node => node.homeCity)).toEqual([null, null, null, 'Brooklyn'])
  })

  it('reads an empty hub city as no city rather than an empty caption', () => {
    // The backend writes "" for a label with nothing on file, and an empty
    // string drawn as a caption is a blank line under the hub, not an absence.
    const map = buildSceneMap(
      overviewFixture({
        nodes: { ...overviewFixture().nodes, hub_city: ['', '', '', ''] },
      }),
    )!

    expect(map.nodes[3].homeCity).toBeNull()
  })

  it('still draws a map whose hub city column is missing or the wrong length', () => {
    // A snapshot written before the column existed carries none at all. That is
    // the NORMAL state on the deploy that ships this, not a corruption: the map
    // draws, it just has no captions until the next nightly build.
    const missing = buildSceneMap(
      overviewFixture({
        nodes: { ...overviewFixture().nodes, hub_city: undefined },
      }),
    )
    const short = buildSceneMap(
      overviewFixture({
        nodes: { ...overviewFixture().nodes, hub_city: ['Brooklyn'] },
      }),
    )

    expect(missing).not.toBeNull()
    expect(missing!.nodes.every(node => node.homeCity === null)).toBe(true)
    expect(short).not.toBeNull()
    expect(short!.nodes.every(node => node.homeCity === null)).toBe(true)
  })

  it('emits each CSR edge exactly once, keyed by entity id', () => {
    const map = buildSceneMap(overviewFixture())!

    // Six neighbour slots describe three edges; the mirror slots are dropped.
    expect(map.edges).toEqual([
      { source: 101, target: 102, kind: 'similarity', appear: 0 },
      { source: 101, target: 900001, kind: 'spoke', appear: 0 },
      { source: 102, target: 103, kind: 'similarity', appear: 0 },
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

  it('stays linear when the CSR offsets jump backwards', () => {
    // Non-monotonic offsets make every even source re-walk the WHOLE targets
    // array. Without the backwards guard this is O(nodes x slots) edge objects
    // from an O(nodes + slots) payload — a frozen tab, not a smaller map.
    const overview = overviewFixture()
    const slots = overview.edges.targets!.length
    overview.edges.offsets = [0, slots, 0, slots, 0]
    const map = buildSceneMap(overview)!

    // Each source contributes at most its own slice; nothing re-walks.
    expect(map.edges.length).toBeLessThanOrEqual(slots)
    expect(map.nodes).toHaveLength(4)
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
    homeCity: null,
    appear: 0,
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
    epoch: new Date('2020-01-01T00:00:00Z'),
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
