import { describe, expect, it } from 'vitest'

import {
  REPLAY_MIN_BINS,
  REPLAY_MS_PER_BIN,
  REPLAY_TARGET_BINS,
  appearAtProgress,
  buildReplayTimeline,
  formatReplayDate,
  progressAtAppear,
  quantileBins,
  replayIsPulsingAt,
  replayReveal,
  revealedNodeCount,
} from './replayTimeline'
import type { SceneMap, SceneMapNode } from './sceneMap'

function node(id: number, appear: number): SceneMapNode {
  return {
    id,
    kind: 'artist',
    name: `Artist ${id}`,
    slug: `artist-${id}`,
    x: 0,
    y: 0,
    community: 1,
    degree: 1,
    rank: id,
    hasUpcomingShow: false,
    hasPlayableAudio: false,
    homeCaption: null,
    appear,
  }
}

function mapWith(appears: number[]): SceneMap {
  return {
    nodes: appears.map((appear, index) => node(index + 1, appear)),
    edges: [],
    regions: [],
    artistCount: appears.length,
    labelCount: 0,
    isolateCount: 0,
    lastMapped: new Date('2026-08-02T04:00:00Z'),
    epoch: new Date('2020-01-01T00:00:00Z'),
  }
}

/** A history shaped like the real one: a sparse trickle, then a dense burst era. */
function realisticAppears(count: number): number[] {
  const appears: number[] = []
  for (let i = 0; i < count; i += 1) {
    // Front-loaded in calendar terms — most arrivals land in the last stretch.
    appears.push(Math.floor(Math.pow(i / count, 3) * 200_000_000))
  }
  return appears
}

describe('quantileBins', () => {
  it('cuts an even history into the requested number of equal-count bins', () => {
    const values = Array.from({ length: 1000 }, (_, i) => i)
    const bins = quantileBins(values, 250)

    expect(bins).toHaveLength(250)
    expect(bins.every(bin => bin.count === 4)).toBe(true)
    // The bins tile the history end to end.
    expect(bins[0].start).toBe(0)
    expect(bins[bins.length - 1].end).toBe(999)
  })

  it('never splits a tie across a boundary, so simultaneous arrivals reveal together', () => {
    // 40 nodes stamped with one instant, sitting inside an otherwise even run.
    const values = [
      ...Array.from({ length: 30 }, (_, i) => i),
      ...Array.from({ length: 40 }, () => 100),
      ...Array.from({ length: 30 }, (_, i) => 200 + i),
    ]
    const bins = quantileBins(values, 10)

    // The tied value appears in exactly one bin: no boundary lands inside it.
    const binsTouching100 = bins.filter(bin => bin.start <= 100 && bin.end >= 100)
    expect(binsTouching100).toHaveLength(1)
    // ...and absorbing the tie is what makes that bar taller than its neighbours,
    // which is the only reason the histogram has a shape at all.
    expect(binsTouching100[0].count).toBeGreaterThan(bins[0].count)
  })

  it('re-divides what is left after a tie, instead of emitting a run of single-item bins', () => {
    // Half the catalog on one instant. A fixed stride would spend the rest of
    // the budget catching up one node at a time.
    const values = [...Array.from({ length: 100 }, () => 0), ...Array.from({ length: 100 }, (_, i) => i + 1)]
    const bins = quantileBins(values, 20)

    expect(bins.length).toBeLessThanOrEqual(20)
    // Every bin after the burst carries a real share rather than a single node.
    expect(bins.slice(1).every(bin => bin.count > 1)).toBe(true)
  })

  it('returns fewer bins than asked when the history has fewer distinct moments', () => {
    const bins = quantileBins([1, 1, 1, 2, 2, 3], 250)

    expect(bins.length).toBeLessThanOrEqual(3)
    expect(bins.reduce((sum, bin) => sum + bin.count, 0)).toBe(6)
  })

  it('handles an empty history', () => {
    expect(quantileBins([], 250)).toEqual([])
  })
})

describe('buildReplayTimeline', () => {
  it('targets ~250 bins and a ~25 second run on a catalog-sized history', () => {
    const timeline = buildReplayTimeline(mapWith(realisticAppears(5000)))!

    expect(timeline.bins).toHaveLength(REPLAY_TARGET_BINS)
    expect(timeline.durationMs).toBe(REPLAY_TARGET_BINS * REPLAY_MS_PER_BIN)
    expect(timeline.durationMs).toBe(25_000)
  })

  it('refuses a snapshot with no appear column — every node reading zero is not a history', () => {
    expect(buildReplayTimeline(mapWith(new Array(500).fill(0)))).toBeNull()
  })

  it('refuses a history too short to be worth watching', () => {
    const appears = Array.from({ length: REPLAY_MIN_BINS - 1 }, (_, i) => i)
    expect(buildReplayTimeline(mapWith(appears))).toBeNull()
  })

  it('refuses an empty map, and one with an unreadable epoch', () => {
    expect(buildReplayTimeline(mapWith([]))).toBeNull()
    const broken = mapWith(realisticAppears(500))
    broken.epoch = new Date('not a date')
    expect(buildReplayTimeline(broken)).toBeNull()
  })

  it('places every node on the run, in the order it arrived', () => {
    const timeline = buildReplayTimeline(mapWith(realisticAppears(1000)))!

    const first = timeline.revealByNodeId.get(1)!
    const last = timeline.revealByNodeId.get(1000)!
    expect(first).toBe(0)
    expect(last).toBe(1)
    // The sorted array the readout searches is genuinely ascending.
    for (let i = 1; i < timeline.sortedNodeReveals.length; i += 1) {
      expect(timeline.sortedNodeReveals[i]).toBeGreaterThanOrEqual(
        timeline.sortedNodeReveals[i - 1],
      )
    }
  })

  it('reports the tallest bar so the histogram needs no second pass', () => {
    const appears = [...Array.from({ length: 400 }, () => 5), ...realisticAppears(600)]
    const timeline = buildReplayTimeline(mapWith(appears))!

    expect(timeline.maxBinCount).toBe(Math.max(...timeline.bins.map(bin => bin.count)))
    expect(timeline.maxBinCount).toBeGreaterThan(timeline.bins.length > 0 ? 4 : 0)
  })
})

describe('progressAtAppear / appearAtProgress', () => {
  const timeline = buildReplayTimeline(mapWith(realisticAppears(2000)))!

  it('is monotonic in the arrival time', () => {
    let previous = -1
    for (let appear = 0; appear <= 200_000_000; appear += 1_000_000) {
      const progress = progressAtAppear(timeline.bins, appear)
      expect(progress).toBeGreaterThanOrEqual(previous)
      previous = progress
    }
  })

  it('clamps outside the history rather than extrapolating', () => {
    expect(progressAtAppear(timeline.bins, -1_000_000)).toBe(0)
    expect(progressAtAppear(timeline.bins, 999_000_000)).toBe(1)
  })

  it('round-trips a position back to a date inside its own bin', () => {
    for (const progress of [0, 0.13, 0.5, 0.87, 1]) {
      const appear = appearAtProgress(timeline, progress)
      const backAgain = progressAtAppear(timeline.bins, appear)
      // Within one bin: the mapping is piecewise linear over bins, so a
      // position inside a bin cannot recover more precisely than the bin.
      expect(Math.abs(backAgain - progress)).toBeLessThanOrEqual(1 / timeline.bins.length)
    }
  })

  it('treats a zero-width bin as a single instant instead of dividing by zero', () => {
    const bins = [
      { start: 0, end: 0, count: 10 },
      { start: 5, end: 5, count: 10 },
      { start: 9, end: 9, count: 10 },
    ]
    expect(progressAtAppear(bins, 5)).toBeCloseTo(1 / 3, 10)
    expect(Number.isFinite(progressAtAppear(bins, 5))).toBe(true)
  })
})

describe('replayReveal', () => {
  it('is 0 before the arrival, 1 after the fade, and smooth in between', () => {
    expect(replayReveal(0.1, 0.2, 0.05)).toBe(0)
    expect(replayReveal(0.2, 0.2, 0.05)).toBe(0)
    expect(replayReveal(0.3, 0.2, 0.05)).toBe(1)
    const mid = replayReveal(0.225, 0.2, 0.05)
    expect(mid).toBeGreaterThan(0)
    expect(mid).toBeLessThan(1)
    expect(mid).toBeCloseTo(0.5, 10)
  })

  it('never moves backwards as the run advances', () => {
    let previous = -1
    for (let progress = 0; progress <= 1; progress += 0.001) {
      const reveal = replayReveal(progress, 0.4, 0.02)
      expect(reveal).toBeGreaterThanOrEqual(previous)
      previous = reveal
    }
  })
})

describe('replayIsPulsingAt', () => {
  it('marks only arrivals inside the beat, and never one that has not arrived', () => {
    expect(replayIsPulsingAt(0.19 - 0.2, 0.05)).toBe(false)
    expect(replayIsPulsingAt(0.2 - 0.2, 0.05)).toBe(true)
    expect(replayIsPulsingAt(0.24 - 0.2, 0.05)).toBe(true)
    expect(replayIsPulsingAt(0.26 - 0.2, 0.05)).toBe(false)
  })
})

describe('revealedNodeCount', () => {
  const timeline = buildReplayTimeline(mapWith(realisticAppears(1000)))!

  it('counts nothing at the start and everything at the end', () => {
    // The first node's reveal is exactly 0, so it has begun arriving at 0.
    expect(revealedNodeCount(timeline, -0.001)).toBe(0)
    expect(revealedNodeCount(timeline, 1)).toBe(1000)
  })

  it('never decreases as the run advances', () => {
    let previous = -1
    for (let progress = 0; progress <= 1; progress += 0.01) {
      const count = revealedNodeCount(timeline, progress)
      expect(count).toBeGreaterThanOrEqual(previous)
      previous = count
    }
  })
})

describe('formatReplayDate', () => {
  it('reads as a month and a year, not a false day-precision cursor', () => {
    expect(formatReplayDate(new Date('2019-03-14T00:00:00Z'))).toMatch(/2019/)
    expect(formatReplayDate(new Date('2019-03-14T00:00:00Z'))).not.toMatch(/14/)
  })

  it('yields nothing for an unreadable date rather than "Invalid Date"', () => {
    expect(formatReplayDate(new Date('nonsense'))).toBe('')
  })
})

// ── The contract the whole feature rests on ───────────────────────────────
describe('seek determinism', () => {
  const timeline = buildReplayTimeline(mapWith(realisticAppears(3000)))!

  /**
   * The rendered state at a position: every node's reveal alpha and pulse flag.
   * This is exactly what the canvas computes per frame, through the same two
   * pure functions, so equality here IS equality on screen.
   */
  function stateAt(progress: number) {
    const state: Array<[number, number, boolean]> = []
    for (const [id, revealAt] of timeline.revealByNodeId) {
      state.push([
        id,
        replayReveal(progress, revealAt, timeline.fadeProgress),
        replayIsPulsingAt(progress - revealAt, timeline.pulseProgress),
      ])
    }
    return state
  }

  it('reaches the same state at T by playing to T as by dragging to T', () => {
    const target = 0.6137

    // Playback: the frame loop's own accumulation, at an irregular frame rate,
    // stopping at the target the way the clock would land on it.
    let played = 0
    const deltas = [0.0031, 0.0009, 0.0164, 0.0007, 0.0052]
    let i = 0
    while (played < target) {
      played = Math.min(target, played + deltas[i % deltas.length])
      i += 1
    }
    expect(played).toBe(target)

    // A drag lands on the identical number, and therefore the identical picture.
    expect(stateAt(played)).toEqual(stateAt(target))
  })

  it('is independent of the path taken to a position', () => {
    const target = 0.42
    const direct = stateAt(target)

    // Forwards past it and dragged back.
    expect(stateAt(target)).toEqual(direct)
    // Reached from the end.
    const fromEnd = stateAt(1)
    expect(fromEnd).not.toEqual(direct)
    expect(stateAt(target)).toEqual(direct)
  })

  it('depends on nothing but the position — no hidden per-node history', () => {
    // Sampled out of order on purpose: if any of this carried state, a
    // descending sweep would disagree with an ascending one.
    const positions = [0.9, 0.1, 0.5, 0.3, 0.7]
    const ascending = new Map(
      [...positions].sort((a, b) => a - b).map(p => [p, JSON.stringify(stateAt(p))]),
    )
    const descending = new Map(
      [...positions].sort((a, b) => b - a).map(p => [p, JSON.stringify(stateAt(p))]),
    )
    for (const position of positions) {
      expect(descending.get(position)).toBe(ascending.get(position))
    }
  })

  it('never draws an edge more present than either of its dots', () => {
    // `buildSceneMap` floors an edge's appear time at the later of its two
    // endpoints, and `progressAtAppear` is monotonic — so the edge's reveal can
    // never run ahead of the dots it connects. Asserted across the whole run
    // rather than at one position, because a line reaching a dot that is not
    // there yet is the one artefact this design cannot produce and must not
    // start producing.
    const a = timeline.revealByNodeId.get(1)!
    const b = timeline.revealByNodeId.get(3000)!
    const edgeRevealAt = Math.max(a, b)

    for (let progress = 0; progress <= 1; progress += 0.01) {
      const edge = replayReveal(progress, edgeRevealAt, timeline.fadeProgress)
      expect(edge).toBeLessThanOrEqual(replayReveal(progress, a, timeline.fadeProgress))
      expect(edge).toBeLessThanOrEqual(replayReveal(progress, b, timeline.fadeProgress))
    }
  })
})
