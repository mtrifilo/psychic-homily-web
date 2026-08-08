import { describe, expect, it } from 'vitest'

import { OG_SIZE } from '@/lib/og/brand'
import { measureMono } from '@/lib/og/textFit'

import {
  CONTENT_WIDTH,
  COUNTS_MAX_WIDTH,
  COUNTS_SIZE_MAX,
  COUNTS_SIZE_MIN,
  COUNTS_TRACKING,
  EYEBROW_SIZE,
  MOTIF_BOX,
  MOTIF_CONNECTOR_LIMIT,
  MOTIF_DOT_LIMIT,
  MOTIF_FADE_CLEAR_STOP,
  MOTIF_NEW_DOT_LIMIT,
  PAD_X,
  RANGE_SIZE,
  TEXT_WIDTH,
  buildGraphWeekMotif,
  eyebrowWidth,
  fitCountsSize,
  headlineLongestWordWidth,
} from './graphWeekOgLayout'
import { resolveGraphWeek } from './graphWeek'
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

/**
 * The family's rule: design at full size, verify at 300px — a link renders about
 * that wide in a group chat — and treat anything under ~8px effective as
 * decoration that must not carry meaning. Every line on this card carries
 * meaning, so every line has to clear the floor.
 */
const SHARE_DOWNSCALE = 4
const LEGIBILITY_FLOOR_PX = 8

describe('card type budgets', () => {
  it('keeps every line above the 300px legibility floor', () => {
    for (const [label, size] of [
      ['eyebrow', EYEBROW_SIZE],
      ['range', RANGE_SIZE],
      ['counts at their floor', COUNTS_SIZE_MIN],
    ] as const) {
      expect(size / SHARE_DOWNSCALE, label).toBeGreaterThanOrEqual(LEGIBILITY_FLOOR_PX)
    }
  })

  it('fits the eyebrow inside the content box at its fixed size', () => {
    // It is deliberately given the FULL content width rather than the text
    // column's, which is what lets it hold 34px. If this ever fails, the copy or
    // the width changed — not the size.
    expect(eyebrowWidth()).toBeLessThanOrEqual(CONTENT_WIDTH)
  })

  it('fits the headline inside the text column without breaking a word', () => {
    // The headline has no fit function because its copy is a constant. This is
    // the measurement that stands in for one: the longest single word must fit,
    // or the wrap would clip mid-word instead of at a space.
    expect(headlineLongestWordWidth()).toBeLessThanOrEqual(TEXT_WIDTH)
  })

  it('fits the range on one line', () => {
    expect(measureMono('DEC 28 2025 - JAN 3 2026', RANGE_SIZE, 2)).toBeLessThanOrEqual(TEXT_WIDTH)
  })
})

describe('fitCountsSize', () => {
  it('uses the full size for an ordinary week', () => {
    const ordinary = '+12 ARTISTS · +34 CONNECTIONS'
    expect(fitCountsSize(ordinary)).toBe(COUNTS_SIZE_MAX)
    expect(measureMono(ordinary, COUNTS_SIZE_MAX, COUNTS_TRACKING)).toBeLessThanOrEqual(
      COUNTS_MAX_WIDTH
    )
  })

  it('seats the widest line this card can produce without overrunning', () => {
    // This is the case that fixes COUNTS_MAX_WIDTH. Five figures on both halves
    // is a mass-import week, not a normal one, but the budget has to hold it —
    // the count is the whole assertion of the card and must not clip.
    const widest = '+9,999 ARTISTS · +99,999 CONNECTIONS'
    const size = fitCountsSize(widest)
    expect(size).toBeLessThan(COUNTS_SIZE_MAX)
    expect(size).toBeGreaterThanOrEqual(COUNTS_SIZE_MIN)
    expect(measureMono(widest, size, COUNTS_TRACKING)).toBeLessThanOrEqual(COUNTS_MAX_WIDTH)
  })

  it('never returns below the legibility floor even for absurd input', () => {
    const absurd = '+1 ARTIST · +2 CONNECTIONS'.repeat(6)
    expect(fitCountsSize(absurd)).toBe(COUNTS_SIZE_MIN)
    // Past the floor the line is over its budget by design — but it must still
    // CLIP inside the canvas rather than bleed off it. `nowrap` plus a text
    // column narrower than the content box is what makes that true.
    expect(measureMono(absurd, COUNTS_SIZE_MIN, COUNTS_TRACKING)).toBeGreaterThan(
      COUNTS_MAX_WIDTH
    )
  })

  it('keeps the counts line inside the motif fade at every size it can pick', () => {
    // The line is set over the gradient, and past MOTIF_FADE_CLEAR_STOP there is
    // no gradient left to sit on. Checked against the widest realistic line at
    // the size the fit function actually returns for it.
    const clearStopPx = (OG_SIZE.width * MOTIF_FADE_CLEAR_STOP) / 100
    const widest = '+9,999 ARTISTS · +99,999 CONNECTIONS'
    const right = PAD_X + measureMono(widest, fitCountsSize(widest), COUNTS_TRACKING)
    expect(right).toBeLessThanOrEqual(clearStopPx)
  })
})

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
    const motif = buildGraphWeekMotif(map, week)
    expect(motif.newDots).toHaveLength(2)
    expect(motif.dots).toHaveLength(2)
  })

  it('draws only the connections the week counted', () => {
    const motif = buildGraphWeekMotif(map, week)
    expect(motif.connectors).toHaveLength(week.newConnectionCount)
    expect(week.newConnectionCount).toBe(1)
  })

  it('fits the projection inside the motif box', () => {
    const motif = buildGraphWeekMotif(map, week)
    for (const dot of [...motif.dots, ...motif.newDots]) {
      expect(dot.x).toBeGreaterThanOrEqual(MOTIF_BOX.x)
      expect(dot.x).toBeLessThanOrEqual(MOTIF_BOX.x + MOTIF_BOX.width)
      expect(dot.y).toBeGreaterThanOrEqual(MOTIF_BOX.y)
      expect(dot.y).toBeLessThanOrEqual(MOTIF_BOX.y + MOTIF_BOX.height)
    }
  })

  it('honours a caller-supplied box, for the share page teaser', () => {
    const box = { x: 0, y: 0, width: 900, height: 380 }
    const motif = buildGraphWeekMotif(map, week, box)
    for (const dot of [...motif.dots, ...motif.newDots]) {
      expect(dot.x).toBeGreaterThanOrEqual(0)
      expect(dot.x).toBeLessThanOrEqual(box.width)
      expect(dot.y).toBeGreaterThanOrEqual(0)
      expect(dot.y).toBeLessThanOrEqual(box.height)
    }
  })

  it('is deterministic — two renders of one snapshot draw the same dots', () => {
    // The card is cached in front of every reader and re-rendered on every cache
    // miss, so a sampled motif that moved between renders would be a different
    // picture of the same week.
    const crowd = mapOf(
      Array.from({ length: MOTIF_DOT_LIMIT * 2 }, (_, i) =>
        node(i + 1, (i % 97) - 48, Math.floor(i / 97) - 10, i % 3 === 0 ? NEW : OLD)
      )
    )
    const crowdWeek = resolveGraphWeek(crowd)!
    expect(buildGraphWeekMotif(crowd, crowdWeek)).toEqual(
      buildGraphWeekMotif(crowd, crowdWeek)
    )
  })

  it('caps what it draws without touching what the week counted', () => {
    // A production snapshot is thousands of nodes; the card draws a sample. The
    // COUNT must stay the real one — a capped number would be a wrong number.
    const crowd = mapOf(
      Array.from({ length: 5000 }, (_, i) => node(i + 1, (i % 71) - 35, Math.floor(i / 71), NEW))
    )
    const crowdWeek = resolveGraphWeek(crowd)!
    const motif = buildGraphWeekMotif(crowd, crowdWeek)

    expect(motif.newDots.length).toBeLessThanOrEqual(MOTIF_NEW_DOT_LIMIT)
    expect(motif.dots.length).toBeLessThanOrEqual(MOTIF_DOT_LIMIT)
    expect(motif.connectors.length).toBeLessThanOrEqual(MOTIF_CONNECTOR_LIMIT)
    expect(crowdWeek.newArtistCount).toBe(5000)
  })

  it('samples across the layout rather than taking one corner of it', () => {
    const crowd = mapOf(
      Array.from({ length: MOTIF_DOT_LIMIT * 3 }, (_, i) => node(i + 1, i, 0, OLD))
    )
    const crowdWeek = resolveGraphWeek(
      mapOf([
        ...crowd.nodes,
        node(999_999, 0, 0, NEW),
      ])
    )!
    const motif = buildGraphWeekMotif(crowd, crowdWeek)
    const xs = motif.dots.map(dot => dot.x)
    // A "first N" sample of a left-to-right layout would cover a third of the
    // box; a stride covers all of it.
    expect(Math.max(...xs) - Math.min(...xs)).toBeGreaterThan(MOTIF_BOX.width * 0.9)
  })

  it('drops a node with a non-finite coordinate instead of emitting NaN', () => {
    const broken = mapOf([node(1, Number.NaN, 0, NEW), node(2, 10, 10, NEW), node(3, -10, -10, OLD)])
    const brokenWeek = resolveGraphWeek(broken)!
    const motif = buildGraphWeekMotif(broken, brokenWeek)

    for (const dot of [...motif.dots, ...motif.newDots]) {
      expect(Number.isFinite(dot.x)).toBe(true)
      expect(Number.isFinite(dot.y)).toBe(true)
    }
    expect(motif.newDots).toHaveLength(1)
  })

  it('survives a degenerate layout where every node shares one point', () => {
    const stacked = mapOf([node(1, 5, 5, NEW), node(2, 5, 5, OLD)])
    const stackedWeek = resolveGraphWeek(stacked)!
    const motif = buildGraphWeekMotif(stacked, stackedWeek)
    expect(motif.newDots).toHaveLength(1)
    expect(Number.isFinite(motif.newDots[0].x)).toBe(true)
  })

  it('draws nothing at all for an empty map', () => {
    const empty = mapOf([node(1, 0, 0, NEW)])
    const emptyWeek = resolveGraphWeek(empty)!
    expect(buildGraphWeekMotif(mapOf([]), emptyWeek)).toEqual({
      dots: [],
      newDots: [],
      connectors: [],
    })
  })

  it('keeps the motif box overhanging the canvas on the right', () => {
    // The composition depends on it: fitted inside 1200×630 the map would be a
    // postage stamp behind the headline instead of a window beside it.
    expect(MOTIF_BOX.x + MOTIF_BOX.width).toBeGreaterThan(OG_SIZE.width)
    expect(MOTIF_BOX.x).toBeGreaterThan(TEXT_WIDTH * 0.6)
  })
})
