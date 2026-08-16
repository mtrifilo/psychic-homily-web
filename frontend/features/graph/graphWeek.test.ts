import { describe, expect, it } from 'vitest'

import {
  GRAPH_WEEK_DAYS,
  formatGraphWeekCounts,
  formatGraphWeekRange,
  formatLastMapped,
  graphWeekKey,
  graphWeekSummary,
  isGraphWeekShareworthy,
  isInGraphWeek,
  resolveGraphWeek,
} from './graphWeek'
import type { SceneMap, SceneMapEdge, SceneMapNode } from './sceneMap'

const EPOCH = new Date('2005-01-01T00:00:00Z')
/** The snapshot's build time in every fixture below. */
const LAST_MAPPED = new Date('2026-08-02T04:30:00Z')

const SECONDS_PER_DAY = 86_400

/** `appear` seconds for a wall-clock instant, the way the backend derives them. */
function appearAt(iso: string): number {
  return Math.floor((new Date(iso).getTime() - EPOCH.getTime()) / 1000)
}

function node(overrides: Partial<SceneMapNode> & { id: number; appear: number }): SceneMapNode {
  return {
    kind: 'artist',
    name: `Artist ${overrides.id}`,
    slug: `artist-${overrides.id}`,
    x: 0,
    y: 0,
    community: 0,
    degree: 1,
    rank: 0,
    hasUpcomingShow: false,
    hasPlayableAudio: false,
    homeCity: null,
    ...overrides,
  }
}

function edge(source: number, target: number, appear: number): SceneMapEdge {
  return { source, target, kind: 'similarity', appear }
}

function mapFixture(overrides: Partial<SceneMap> = {}): SceneMap {
  return {
    nodes: [],
    edges: [],
    regions: [],
    artistCount: 0,
    labelCount: 0,
    isolateCount: 0,
    lastMapped: LAST_MAPPED,
    epoch: EPOCH,
    ...overrides,
  }
}

describe('resolveGraphWeek', () => {
  it('spans the seven calendar days ending on the snapshot, in UTC', () => {
    const week = resolveGraphWeek(
      mapFixture({ nodes: [node({ id: 1, appear: appearAt('2026-08-01T00:00:00Z') })] })
    )

    // The snapshot was built 2026-08-02T04:30Z, so the window's first instant is
    // midnight UTC six days earlier: JUL 27 through AUG 2 inclusive.
    expect(week?.start.toISOString()).toBe('2026-07-27T00:00:00.000Z')
    expect(week?.end.toISOString()).toBe(LAST_MAPPED.toISOString())
    expect(GRAPH_WEEK_DAYS).toBe(7)
  })

  // The boundary is INCLUSIVE at both ends, and these two cases are what pin it.
  // Get either wrong and the counts on the card disagree with the map by a dot.
  it('includes an arrival at the exact first instant of the window', () => {
    const week = resolveGraphWeek(
      mapFixture({ nodes: [node({ id: 1, appear: appearAt('2026-07-27T00:00:00Z') })] })
    )
    expect(week?.newArtistCount).toBe(1)
  })

  it('excludes an arrival one second before the window opens', () => {
    const week = resolveGraphWeek(
      mapFixture({
        nodes: [node({ id: 1, appear: appearAt('2026-07-27T00:00:00Z') - 1 })],
      })
    )
    expect(week?.newArtistCount).toBe(0)
    expect(week?.newNodeIds.size).toBe(0)
  })

  it('includes an arrival at the exact last_mapped instant', () => {
    const week = resolveGraphWeek(
      mapFixture({ nodes: [node({ id: 1, appear: appearAt('2026-08-02T04:30:00Z') })] })
    )
    expect(week?.newArtistCount).toBe(1)
  })

  it('excludes an arrival after last_mapped', () => {
    const week = resolveGraphWeek(
      mapFixture({
        nodes: [
          node({ id: 1, appear: appearAt('2026-08-02T04:30:01Z') }),
          node({ id: 2, appear: appearAt('2026-07-30T00:00:00Z') }),
        ],
      })
    )
    expect(week?.newArtistCount).toBe(1)
    expect(week?.newNodeIds.has(2)).toBe(true)
  })

  it('counts label hubs as new nodes but never as new artists', () => {
    const inWindow = appearAt('2026-07-30T12:00:00Z')
    const week = resolveGraphWeek(
      mapFixture({
        nodes: [
          node({ id: 1, appear: inWindow }),
          node({ id: 900001, appear: inWindow, kind: 'label' }),
        ],
      })
    )

    expect(week?.newArtistCount).toBe(1)
    // The hub still lights up on the card — it is a new part of the map.
    expect(week?.newNodeIds.has(900001)).toBe(true)
  })

  it('counts edges by the same window rule as nodes', () => {
    const week = resolveGraphWeek(
      mapFixture({
        nodes: [
          node({ id: 1, appear: appearAt('2019-03-01T00:00:00Z') }),
          node({ id: 2, appear: appearAt('2026-07-30T00:00:00Z') }),
        ],
        edges: [
          // Both endpoints are old: the edge predates the window.
          edge(1, 1, appearAt('2019-03-01T00:00:00Z')),
          // The later endpoint arrived this week.
          edge(1, 2, appearAt('2026-07-30T00:00:00Z')),
          edge(2, 2, appearAt('2026-08-01T09:00:00Z')),
        ],
      })
    )

    expect(week?.newConnectionCount).toBe(2)
    // The predicate the motif shares with the counts agrees with them.
    expect(isInGraphWeek(week!, appearAt('2026-07-30T00:00:00Z'))).toBe(true)
    expect(isInGraphWeek(week!, appearAt('2026-03-30T00:00:00Z'))).toBe(false)
  })

  it('refuses a snapshot with no dated arrivals at all', () => {
    // Every node reading 0 is how `buildSceneMap` surfaces a payload with no
    // `appear` column. There is no week to report from it.
    expect(
      resolveGraphWeek(mapFixture({ nodes: [node({ id: 1, appear: 0 }), node({ id: 2, appear: 0 })] }))
    ).toBeNull()
  })

  it('refuses a snapshot whose timestamps did not parse', () => {
    const dated = [node({ id: 1, appear: appearAt('2026-07-30T00:00:00Z') })]
    expect(resolveGraphWeek(mapFixture({ nodes: dated, lastMapped: new Date('nope') }))).toBeNull()
    expect(resolveGraphWeek(mapFixture({ nodes: dated, epoch: new Date('nope') }))).toBeNull()
  })

  it('refuses a snapshot whose window would start before the epoch', () => {
    // A window cannot straddle the origin every `appear` counts from: an arrival
    // before it is clamped to 0, so "inside" and "outside" stop being different.
    expect(
      resolveGraphWeek(
        mapFixture({
          epoch: new Date('2026-07-30T00:00:00Z'),
          lastMapped: new Date('2026-08-02T04:30:00Z'),
          nodes: [node({ id: 1, appear: 100 })],
        })
      )
    ).toBeNull()
  })

  it('is shareworthy only when something actually arrived', () => {
    const quiet = resolveGraphWeek(
      mapFixture({
        nodes: [
          node({ id: 1, appear: appearAt('2019-01-01T00:00:00Z') }),
          node({ id: 2, appear: appearAt('2020-01-01T00:00:00Z') }),
        ],
      })
    )
    // Dateable, so the page and the card still render — but nothing to offer.
    expect(quiet).not.toBeNull()
    expect(quiet?.newArtistCount).toBe(0)
    expect(isGraphWeekShareworthy(quiet!)).toBe(false)

    const busy = resolveGraphWeek(
      mapFixture({ nodes: [node({ id: 1, appear: appearAt('2026-07-29T00:00:00Z') })] })
    )
    expect(isGraphWeekShareworthy(busy!)).toBe(true)
  })

  it('reports a week whose only news is a connection', () => {
    const week = resolveGraphWeek(
      mapFixture({
        nodes: [node({ id: 1, appear: appearAt('2019-01-01T00:00:00Z') })],
        edges: [edge(1, 1, appearAt('2026-07-31T00:00:00Z'))],
      })
    )
    expect(week?.newArtistCount).toBe(0)
    expect(week?.newConnectionCount).toBe(1)
    expect(isGraphWeekShareworthy(week!)).toBe(true)
  })

  it('exposes the window as whole epoch-relative seconds', () => {
    const week = resolveGraphWeek(
      mapFixture({ nodes: [node({ id: 1, appear: appearAt('2026-07-30T00:00:00Z') })] })
    )
    expect(week?.appearRange.end).toBe(appearAt('2026-08-02T04:30:00Z'))
    expect(week!.appearRange.end - week!.appearRange.start).toBe(
      6 * SECONDS_PER_DAY + 4 * 3600 + 30 * 60
    )
  })
})

describe('formatGraphWeekRange', () => {
  it('names both days and the shared year, with a hyphen', () => {
    expect(
      formatGraphWeekRange(new Date('2026-07-27T00:00:00Z'), new Date('2026-08-02T04:30:00Z'))
    ).toBe('JUL 27 - AUG 2 2026')
  })

  it('carries a year on each end when the range straddles New Year', () => {
    expect(
      formatGraphWeekRange(new Date('2025-12-28T00:00:00Z'), new Date('2026-01-03T04:00:00Z'))
    ).toBe('DEC 28 2025 - JAN 3 2026')
  })

  it('formats in UTC regardless of the runtime timezone', () => {
    // A late-UTC instant is the previous day in the Americas. The card is one
    // cached PNG served worldwide, so the label must not move with the renderer.
    expect(
      formatGraphWeekRange(new Date('2026-07-27T00:00:00Z'), new Date('2026-08-02T23:30:00Z'))
    ).toBe('JUL 27 - AUG 2 2026')
  })

  it('is empty rather than "Invalid Date" for an unparsed end', () => {
    expect(formatGraphWeekRange(new Date('2026-07-27T00:00:00Z'), new Date('nope'))).toBe('')
  })
})

describe('count copy', () => {
  const week = (newArtistCount: number, newConnectionCount: number) => ({
    start: new Date('2026-07-27T00:00:00Z'),
    end: new Date('2026-08-02T04:30:00Z'),
    newArtistCount,
    newConnectionCount,
    newNodeIds: new Set<number>(),
    appearRange: { start: 0, end: 1 },
  })

  it('sets the card line in upper case, grouped, with a middle dot', () => {
    expect(formatGraphWeekCounts(week(1234, 56))).toBe('+1,234 ARTISTS · +56 CONNECTIONS')
  })

  it('singularises both halves independently', () => {
    expect(formatGraphWeekCounts(week(1, 1))).toBe('+1 ARTIST · +1 CONNECTION')
    expect(formatGraphWeekCounts(week(0, 2))).toBe('+0 ARTISTS · +2 CONNECTIONS')
  })

  it('reads as a sentence for the snippet and the screen reader', () => {
    expect(graphWeekSummary(week(12, 30))).toBe(
      '12 new artists and 30 new connections joined the map, JUL 27 - AUG 2 2026.'
    )
  })

  it('keys the share image URL on the window end date', () => {
    expect(graphWeekKey(week(1, 1))).toBe('2026-08-02')
  })
})

describe('formatLastMapped', () => {
  // The two ends of one UTC day, and its middle. In ANY timezone but UTC
  // itself, at least one of the outer two falls on a different LOCAL calendar
  // day than the middle one — so a formatter that quietly slipped back to the
  // reader's zone cannot satisfy both assertions, whatever zone (or locale)
  // the runner happens to sit in. This is the boundary the bug lived on: the
  // nightly job lands in the small hours UTC, which is still yesterday across
  // the Americas.
  const startOfDay = new Date('2026-08-02T00:30:00Z')
  const middleOfDay = new Date('2026-08-02T12:00:00Z')
  const endOfDay = new Date('2026-08-02T23:30:00Z')

  it('names the UTC day for every instant in it, not the reader-local one', () => {
    expect(formatLastMapped(startOfDay)).toBe(formatLastMapped(middleOfDay))
    expect(formatLastMapped(endOfDay)).toBe(formatLastMapped(middleOfDay))
  })

  it('agrees with the week the same snapshot ends', () => {
    // The footer and the share affordance sit on one line of chrome, reading
    // the same `last_mapped`. Word boundaries rather than a literal date so
    // this holds under any runner locale: `Aug 2, 2026` and `2. Aug. 2026`
    // both carry a standalone 2 and neither carries a 1 or a 3.
    const snapshot = mapFixture({
      lastMapped: startOfDay,
      nodes: [node({ id: 1, appear: appearAt('2026-08-01T00:00:00Z') })],
    })

    expect(graphWeekKey(resolveGraphWeek(snapshot)!)).toBe('2026-08-02')
    expect(formatLastMapped(startOfDay)).toMatch(/\b2\b/)
    expect(formatLastMapped(startOfDay)).not.toMatch(/\b1\b/)
    expect(formatLastMapped(endOfDay)).not.toMatch(/\b3\b/)
  })

  it('drops the clause for an instant that did not parse', () => {
    expect(formatLastMapped(new Date('nope'))).toBe('')
  })
})
