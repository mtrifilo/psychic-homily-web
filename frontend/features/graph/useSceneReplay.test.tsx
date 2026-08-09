import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'

import { SETTLE_MS, replayStatusText, useSceneReplay } from './useSceneReplay'
import { buildReplayTimeline, replayReadoutText } from './replayTimeline'
import type { SceneMap, SceneMapNode } from './sceneMap'

/**
 * jsdom has no frame loop and no clock that advances on its own, so both are
 * driven by hand: `runFrames` calls whatever the hook scheduled, at the times we
 * say it ran. That is not a compromise — a replay whose behaviour depended on
 * the REAL frame rate could not be tested at all, and the whole point of the
 * progress-driven design is that it does not.
 */
let scheduled: Array<(now: number) => void> = []
let now = 0

function runFrames(count: number, msPerFrame = 16) {
  for (let i = 0; i < count; i += 1) {
    const callbacks = scheduled
    scheduled = []
    if (callbacks.length === 0) return
    now += msPerFrame
    act(() => {
      for (const callback of callbacks) callback(now)
    })
  }
}

beforeEach(() => {
  scheduled = []
  now = 1000
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
    scheduled.push(callback)
    return scheduled.length
  })
  // Cancellation has to be OBSERVABLE, or a test cannot tell a torn-down chain
  // from a live one. The transport keeps exactly one frame in flight, so
  // dropping the queue is a faithful stand-in for cancelling by handle.
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {
    scheduled = []
  })
  vi.spyOn(performance, 'now').mockImplementation(() => now)
})

afterEach(() => {
  vi.restoreAllMocks()
})

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
    homeCity: null,
    appear,
  }
}

function sceneMap(appears: number[]): SceneMap {
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

const HISTORY = Array.from({ length: 1000 }, (_, i) => Math.floor((i / 1000) ** 3 * 200_000_000))

// ONE stable instance, as the host supplies (the map is `useMemo`d off the
// query cache). Rebuilding it per render would be a different snapshot every
// render, which is a state this hook deliberately treats as "the map changed".
const REPLAYABLE_MAP = sceneMap(HISTORY)
const FLAT_MAP = sceneMap(new Array(500).fill(0))

describe('useSceneReplay', () => {
  it('offers no transport for a snapshot with no watchable history', () => {
    const { result } = renderHook(() => useSceneReplay(FLAT_MAP))

    expect(result.current.timeline).toBeNull()
    expect(result.current.isActive).toBe(false)

    act(() => result.current.start())

    // Starting is a no-op rather than an error: the affordance is not rendered
    // in this state, so this is only ever reached by a caller that got it wrong.
    expect(result.current.isActive).toBe(false)
  })

  it('offers no transport at all when there is no map', () => {
    const { result } = renderHook(() => useSceneReplay(null))
    expect(result.current.timeline).toBeNull()
  })

  describe('entering', () => {
    it('starts at the beginning of the history and plays immediately', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))

      expect(result.current.phase).toBe('rest')
      expect(result.current.readFrame().active).toBe(false)

      act(() => result.current.start())

      expect(result.current.phase).toBe('playing')
      expect(result.current.readFrame().active).toBe(true)
      // The map empties to t0 — nothing has arrived on the first frame.
      expect(result.current.readFrame().progress).toBe(0)
      // ...and the furniture is still up, so it can be seen to leave.
      expect(result.current.readFrame().decorationAlpha).toBe(1)
    })

    it('clears the map furniture over the entry transition', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      act(() => result.current.start())

      runFrames(4)
      const midway = result.current.readFrame().decorationAlpha
      expect(midway).toBeGreaterThan(0)
      expect(midway).toBeLessThan(1)

      runFrames(30)
      expect(result.current.readFrame().decorationAlpha).toBe(0)
    })

    it('advances the playhead against wall time, not frame count', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      const durationMs = result.current.timeline!.durationMs
      act(() => result.current.start())

      runFrames(10, 100)

      // Ten 100ms frames is 1000ms of a 25s run.
      expect(result.current.readFrame().progress).toBeCloseTo(1000 / durationMs, 4)
    })

    it('refuses to start a run that is already running', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      act(() => result.current.start())
      runFrames(20, 100)
      const progress = result.current.readFrame().progress

      act(() => result.current.start())

      expect(result.current.readFrame().progress).toBe(progress)
    })
  })

  describe('pausing', () => {
    it('holds the playhead, and releases it again', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      act(() => result.current.start())
      runFrames(5, 100)

      act(() => result.current.togglePause())
      expect(result.current.phase).toBe('paused')
      const held = result.current.readFrame().progress
      runFrames(20, 100)
      expect(result.current.readFrame().progress).toBe(held)

      act(() => result.current.togglePause())
      expect(result.current.phase).toBe('playing')
      runFrames(5, 100)
      expect(result.current.readFrame().progress).toBeGreaterThan(held)
    })
  })

  describe('seeking', () => {
    it('jumps the playhead and clamps to the run', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      act(() => result.current.start())

      act(() => result.current.seek(0.42))
      expect(result.current.readFrame().progress).toBe(0.42)

      act(() => result.current.seek(-3))
      expect(result.current.readFrame().progress).toBe(0)

      act(() => result.current.seek(9))
      expect(result.current.readFrame().progress).toBe(1)
    })

    it('holds the clock while a drag is in progress, so it cannot fight the pointer', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      act(() => result.current.start())

      act(() => result.current.setScrubbing(true))
      act(() => result.current.seek(0.3))
      runFrames(10, 100)
      expect(result.current.readFrame().progress).toBe(0.3)

      act(() => result.current.setScrubbing(false))
      runFrames(5, 100)
      expect(result.current.readFrame().progress).toBeGreaterThan(0.3)
    })

    // A drag whose pointerup never arrives (capture lost, or the strip removed
    // between down and up) must not strand the hold across runs — that would
    // leave the next run parked at zero with nothing on screen to explain it.
    it('does not carry a stranded drag hold into the next run', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      act(() => result.current.start())
      act(() => result.current.setScrubbing(true))
      act(() => result.current.exit())
      runFrames(5, SETTLE_MS)
      expect(result.current.phase).toBe('rest')

      act(() => result.current.start())
      runFrames(5, 100)

      expect(result.current.readFrame().progress).toBeGreaterThan(0)
    })

    it('does nothing before a run has started', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))

      act(() => result.current.seek(0.5))

      expect(result.current.readFrame().progress).toBe(0)
    })
  })

  describe('settling', () => {
    it('settles into the at-rest map on reaching now', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      act(() => result.current.start())
      act(() => result.current.seek(0.99))

      runFrames(3, 100)
      expect(result.current.phase).toBe('settling')

      // The furniture comes back over the settle window, not instantly.
      runFrames(2, 100)
      const midway = result.current.readFrame().decorationAlpha
      expect(midway).toBeGreaterThan(0)
      expect(midway).toBeLessThan(1)

      runFrames(10, SETTLE_MS)
      expect(result.current.phase).toBe('rest')
      expect(result.current.isActive).toBe(false)
      expect(result.current.readFrame().active).toBe(false)
      expect(result.current.readFrame().progress).toBe(1)
      expect(result.current.readFrame().decorationAlpha).toBe(1)
    })

    it('restores the at-rest map from wherever an early exit leaves it', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      act(() => result.current.start())
      act(() => result.current.seek(0.25))

      act(() => result.current.exit())
      expect(result.current.phase).toBe('settling')
      // Still mid-history at the moment of the exit — the rest of the map fills
      // in across the settle rather than snapping.
      expect(result.current.readFrame().progress).toBe(0.25)

      runFrames(1, SETTLE_MS / 2)
      const midway = result.current.readFrame().progress
      expect(midway).toBeGreaterThan(0.25)
      expect(midway).toBeLessThan(1)

      runFrames(5, SETTLE_MS)
      expect(result.current.phase).toBe('rest')
      expect(result.current.readFrame().progress).toBe(1)
      expect(result.current.readFrame().decorationAlpha).toBe(1)
    })

    it('exits from a paused run too', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      act(() => result.current.start())
      runFrames(3, 100)
      act(() => result.current.togglePause())

      act(() => result.current.exit())
      runFrames(5, SETTLE_MS)

      expect(result.current.phase).toBe('rest')
    })

    it('ignores an exit when there is no run to exit', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))

      act(() => result.current.exit())

      expect(result.current.phase).toBe('rest')
    })

    it('can be replayed again after settling', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      act(() => result.current.start())
      act(() => result.current.exit())
      runFrames(5, SETTLE_MS)
      expect(result.current.phase).toBe('rest')

      act(() => result.current.start())

      expect(result.current.phase).toBe('playing')
      expect(result.current.readFrame().progress).toBe(0)
    })
  })

  describe('subscribers', () => {
    it('publishes the current frame on subscribe, and on every advance', () => {
      const { result } = renderHook(() => useSceneReplay(REPLAYABLE_MAP))
      const seen: number[] = []

      let unsubscribe = () => {}
      act(() => {
        unsubscribe = result.current.subscribe(frame => seen.push(frame.progress))
      })
      // Fired immediately, so a scrubber mounting mid-run is never blank.
      expect(seen).toEqual([0])

      act(() => result.current.start())
      runFrames(3, 100)
      expect(seen.length).toBeGreaterThan(3)

      const countBefore = seen.length
      act(() => unsubscribe())
      runFrames(3, 100)
      expect(seen).toHaveLength(countBefore)
    })
  })

  describe('a snapshot swapped in under a live run', () => {
    // The nightly refetch can land mid-replay. Because every drawn value is
    // derived from a NORMALISED position rather than from a timestamp or a node
    // count, the run keeps its place against the new snapshot — the transport
    // needs no special case, and interrupting the visitor to apply one would be
    // solving a problem they do not have.
    it('keeps playing from where it was, against the new snapshot', () => {
      const { result, rerender } = renderHook(({ map }) => useSceneReplay(map), {
        initialProps: { map: REPLAYABLE_MAP },
      })
      act(() => result.current.start())
      runFrames(5, 100)
      const progress = result.current.readFrame().progress
      expect(progress).toBeGreaterThan(0)

      rerender({ map: sceneMap(HISTORY.map(appear => appear + 1)) })

      expect(result.current.phase).toBe('playing')
      expect(result.current.readFrame().progress).toBe(progress)
      expect(result.current.readFrame().active).toBe(true)

      runFrames(5, 100)
      expect(result.current.readFrame().progress).toBeGreaterThan(progress)
    })

    // Losing the map entirely is the case that MUST end the run. Its owner only
    // hands this hook a map while the drawn map is the surface on screen, so a
    // null arrival means the visitor re-rooted on an artist or narrowed the
    // window past the canvas breakpoint. The scrubber and its Escape handler
    // unmount with that surface, so a run that kept going would be a mode with
    // no way out of it.
    it('ends the run when the map goes away, and stops the frame loop', () => {
      const { result, rerender } = renderHook(({ map }) => useSceneReplay(map), {
        initialProps: { map: REPLAYABLE_MAP as SceneMap | null },
      })
      act(() => result.current.start())
      runFrames(5, 100)
      expect(result.current.isActive).toBe(true)

      rerender({ map: null })

      expect(result.current.phase).toBe('rest')
      expect(result.current.isActive).toBe(false)
      expect(result.current.readFrame().active).toBe(false)
      expect(result.current.readFrame().progress).toBe(0)
      // Nothing left scheduled: the chain is cancelled rather than left
      // publishing frames at a surface nobody can see.
      expect(scheduled).toHaveLength(0)
    })

    it('can start a fresh run once the map comes back', () => {
      const { result, rerender } = renderHook(({ map }) => useSceneReplay(map), {
        initialProps: { map: REPLAYABLE_MAP as SceneMap | null },
      })
      act(() => result.current.start())
      runFrames(5, 100)
      rerender({ map: null })
      rerender({ map: REPLAYABLE_MAP })

      expect(result.current.phase).toBe('rest')
      act(() => result.current.start())

      expect(result.current.phase).toBe('playing')
      expect(result.current.readFrame().progress).toBe(0)
    })

    it('survives an unmemoised map without spinning', () => {
      // A caller rebuilding the map every render must not put this hook into a
      // render loop, whatever else it costs them.
      const { result, rerender } = renderHook(() => useSceneReplay(sceneMap(HISTORY)))

      act(() => result.current.start())
      rerender()
      rerender()

      expect(result.current.phase).toBe('playing')
    })
  })
})

describe('replayStatusText', () => {
  // Lower case at the source: both hosts carry `uppercase` in their class list,
  // so the display casing is CSS's job and the same string is what an assistive
  // technology should read out of the scrubber's `aria-valuetext`.
  it('reads Replay, the date, and what is on the map', () => {
    const timeline = buildReplayTimeline(REPLAYABLE_MAP)!

    expect(replayStatusText(timeline, 0)).toMatch(/^Replay · \w+ 20\d\d · [\d,]+ on the map$/)
    expect(replayStatusText(timeline, 1)).toContain('1,000 on the map')
  })

  it('shares its sentence with the scrubber readout, so the two cannot drift', () => {
    const timeline = buildReplayTimeline(REPLAYABLE_MAP)!

    expect(replayStatusText(timeline, 0.4)).toBe(
      `Replay · ${replayReadoutText(timeline, 0.4)}`,
    )
  })

  it('carries no em dashes, per the project copy rule', () => {
    const timeline = buildReplayTimeline(REPLAYABLE_MAP)!
    expect(replayStatusText(timeline, 0.5)).not.toContain('—')
  })
})
