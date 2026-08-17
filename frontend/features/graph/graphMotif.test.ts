import { describe, expect, it } from 'vitest'

import { OG_SIZE } from '@/lib/og/brand'

import {
  CARD_MOTIF,
  TEASER_MOTIF,
  buildGraphWeekMotif,
  type MotifSpec,
} from './graphMotif'
import { resolveGraphWeek } from './graphWeek'
import { TEXT_WIDTH } from './graphWeekOgLayout'
import type { SceneMap, SceneMapEdge, SceneMapNode } from './sceneMap'

const EPOCH = new Date('2005-01-01T00:00:00Z')
const LAST_MAPPED = new Date('2026-08-02T04:30:00Z')

function appearAt(iso: string): number {
  return Math.floor((new Date(iso).getTime() - EPOCH.getTime()) / 1000)
}

const OLD = appearAt('2019-06-01T00:00:00Z')
const NEW = appearAt('2026-07-30T00:00:00Z')

function node(id: number, x: number, y: number, appear: number): SceneMapNode {
  return {
    id,
    kind: 'artist',
    name: `Artist ${id}`,
    slug: `artist-${id}`,
    x,
    y,
    community: 0,
    degree: 1,
    rank: 0,
    hasUpcomingShow: false,
    hasPlayableAudio: false,
    homeCaption: null,
    appear,
  }
}

function mapOf(nodes: SceneMapNode[], edges: SceneMapEdge[] = []): SceneMap {
  return {
    nodes,
    edges,
    regions: [],
    artistCount: nodes.length,
    labelCount: 0,
    isolateCount: 0,
    lastMapped: LAST_MAPPED,
    epoch: EPOCH,
  }
}

describe('buildGraphWeekMotif', () => {
  const map = mapOf(
    [
      node(1, -100, -100, OLD),
      node(2, 100, 100, OLD),
      node(3, 0, 50, NEW),
      node(4, 50, 0, NEW),
    ],
    [
      { source: 3, target: 4, kind: 'similarity', appear: NEW } as SceneMapEdge,
      { source: 1, target: 2, kind: 'similarity', appear: OLD } as SceneMapEdge,
    ]
  )
  const week = resolveGraphWeek(map)!

  it('splits the nodes by the window, not by anything it re-derives', () => {
    const motif = buildGraphWeekMotif(map, week, CARD_MOTIF)
    expect(motif.newDots).toHaveLength(2)
    expect(motif.dots).toHaveLength(2)
  })

  it('draws only the connections the week counted', () => {
    const motif = buildGraphWeekMotif(map, week, CARD_MOTIF)
    expect(motif.connectors).toHaveLength(week.newConnectionCount)
    expect(week.newConnectionCount).toBe(1)
  })

  it('fits the projection inside whichever box it is given', () => {
    for (const spec of [CARD_MOTIF, TEASER_MOTIF]) {
      const { box } = spec
      const motif = buildGraphWeekMotif(map, week, spec)
      for (const dot of [...motif.dots, ...motif.newDots]) {
        expect(dot.x).toBeGreaterThanOrEqual(box.x)
        expect(dot.x).toBeLessThanOrEqual(box.x + box.width)
        expect(dot.y).toBeGreaterThanOrEqual(box.y)
        expect(dot.y).toBeLessThanOrEqual(box.y + box.height)
      }
    }
  })

  it('is deterministic — two renders of one snapshot draw the same dots', () => {
    // The card is cached in front of every reader and re-rendered on every cache
    // miss, so a sampled motif that moved between renders would be a different
    // picture of the same week.
    const crowd = crowdedMap(CARD_MOTIF.limits.dots * 2)
    const crowdWeek = resolveGraphWeek(crowd)!
    expect(buildGraphWeekMotif(crowd, crowdWeek, CARD_MOTIF)).toEqual(
      buildGraphWeekMotif(crowd, crowdWeek, CARD_MOTIF)
    )
  })

  it('caps what it draws without touching what the week counted', () => {
    // A production snapshot is thousands of nodes; the motif draws a sample. The
    // COUNT must stay the real one — a capped number would be a wrong number.
    const crowd = mapOf(
      Array.from({ length: 5000 }, (_, i) => node(i + 1, (i % 71) - 35, Math.floor(i / 71), NEW))
    )
    const crowdWeek = resolveGraphWeek(crowd)!
    const motif = buildGraphWeekMotif(crowd, crowdWeek, CARD_MOTIF)

    expect(motif.newDots.length).toBeLessThanOrEqual(CARD_MOTIF.limits.newDots)
    expect(motif.dots.length).toBeLessThanOrEqual(CARD_MOTIF.limits.dots)
    expect(crowdWeek.newArtistCount).toBe(5000)
  })

  it('caps connectors by SAMPLING them, not by truncating to the first N', () => {
    // Edges arrive in CSR order, i.e. ascending source index, which the backend
    // groups by community — so a `break` at the cap drew every connector in one
    // corner of the map. A stride spreads them the way the dots are spread.
    const nodes = Array.from({ length: 400 }, (_, i) => node(i + 1, i * 10, (i % 7) * 10, NEW))
    const edges: SceneMapEdge[] = nodes
      .slice(0, -1)
      .map((from, i) => ({
        source: from.id,
        target: nodes[i + 1].id,
        kind: 'similarity' as const,
        appear: NEW,
      }))
    const crowd = mapOf(nodes, edges)
    const crowdWeek = resolveGraphWeek(crowd)!
    const motif = buildGraphWeekMotif(crowd, crowdWeek, CARD_MOTIF)

    expect(motif.connectors.length).toBe(CARD_MOTIF.limits.connectors)
    const xs = motif.connectors.map(line => line.x1)
    const drawnSpan = Math.max(...xs) - Math.min(...xs)
    // Truncation would have covered the leftmost `limit/total` of the layout.
    const truncatedSpan = (CARD_MOTIF.limits.connectors / edges.length) * CARD_MOTIF.box.width
    expect(drawnSpan).toBeGreaterThan(truncatedSpan * 2)
  })

  it('samples dots across the layout rather than taking one corner of it', () => {
    const crowd = crowdedMap(CARD_MOTIF.limits.dots * 3)
    const crowdWeek = resolveGraphWeek(crowd)!
    const motif = buildGraphWeekMotif(crowd, crowdWeek, CARD_MOTIF)
    const xs = motif.dots.map(dot => dot.x)
    expect(Math.max(...xs) - Math.min(...xs)).toBeGreaterThan(CARD_MOTIF.box.width * 0.9)
  })

  it('drops a node with a non-finite coordinate instead of emitting NaN', () => {
    const broken = mapOf([node(1, Number.NaN, 0, NEW), node(2, 10, 10, NEW), node(3, -10, -10, OLD)])
    const brokenWeek = resolveGraphWeek(broken)!
    const motif = buildGraphWeekMotif(broken, brokenWeek, CARD_MOTIF)

    for (const dot of [...motif.dots, ...motif.newDots]) {
      expect(Number.isFinite(dot.x)).toBe(true)
      expect(Number.isFinite(dot.y)).toBe(true)
    }
    expect(motif.newDots).toHaveLength(1)
  })

  it('survives a degenerate layout where every node shares one point', () => {
    const stacked = mapOf([node(1, 5, 5, NEW), node(2, 5, 5, OLD)])
    const stackedWeek = resolveGraphWeek(stacked)!
    const motif = buildGraphWeekMotif(stacked, stackedWeek, CARD_MOTIF)
    expect(motif.newDots).toHaveLength(1)
    expect(Number.isFinite(motif.newDots[0].x)).toBe(true)
  })

  it('draws nothing at all for an empty map', () => {
    expect(buildGraphWeekMotif(mapOf([]), week, CARD_MOTIF)).toEqual({
      dots: [],
      newDots: [],
      connectors: [],
    })
  })
})

describe('the two motif specs', () => {
  it('keeps the card motif overhanging the canvas on the right', () => {
    // The composition depends on it: fitted inside 1200×630 the map would be a
    // postage stamp behind the headline instead of a window beside it.
    expect(CARD_MOTIF.box.x + CARD_MOTIF.box.width).toBeGreaterThan(OG_SIZE.width)
    expect(CARD_MOTIF.box.x).toBeGreaterThan(TEXT_WIDTH * 0.6)
  })

  it('ships the page far fewer elements than the card rasterises', () => {
    // The card's elements are rasterised away; the teaser's are SERIALISED into
    // the HTML and the RSC payload of a page someone bounces off. At the card's
    // caps that measured ~68KB + ~67KB and ~1,300 DOM nodes.
    expect(total(TEASER_MOTIF)).toBeLessThan(total(CARD_MOTIF) / 2)
  })
})

function total(spec: MotifSpec): number {
  return spec.limits.dots + spec.limits.newDots + spec.limits.connectors
}

/** `count` artists in a wide left-to-right band, every third one an arrival. */
function crowdedMap(count: number): SceneMap {
  return mapOf(
    Array.from({ length: count }, (_, i) =>
      node(i + 1, (i % 97) - 48, Math.floor(i / 97) - 10, i % 3 === 0 ? NEW : OLD)
    )
  )
}
