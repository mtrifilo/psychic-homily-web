'use client'

/**
 * The growth replay's transport (PSY-1737): one clock, advanced in a rAF loop,
 * read by everything that draws.
 *
 * THE PLAYHEAD IS NEVER REACT STATE. A replay writes a new position sixty times
 * a second across a canvas of thousands of nodes, a 250-bar histogram and two
 * text readouts; routing that through `setState` would re-render the whole map
 * card every frame. So the position lives in a mutable frame object behind a
 * ref, and anything that needs it either reads it inside its own paint callback
 * (the canvas) or subscribes for a callback that writes the DOM directly (the
 * scrubber, the status line).
 *
 * React state here changes only when the MODE does — entering, pausing,
 * settling, resting — which is a handful of times per run.
 *
 * The frame is derived from ONE number (`progress`, 0..1 along the run) by a
 * pure function, which is what makes a drag and a playthrough interchangeable:
 * seeking to T and playing to T produce the identical frame, so they produce the
 * identical picture. That equivalence is the feature's contract, and it is
 * tested rather than asserted.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useReducedMotion } from '@/features/artists/hooks/useReducedMotion'

import {
  buildReplayTimeline,
  replayReadoutText,
  type ReplayTimeline,
} from './replayTimeline'
import type { SceneMap } from './sceneMap'

/** How long the decorations take to clear on entry. */
const ENTER_MS = 400
/** How long the map takes to come back on settle — the approved ~600ms. */
export const SETTLE_MS = 600

/**
 * What the replay is doing. `settling` covers BOTH ways a run ends — reaching
 * now, and an Esc that abandons it early — because they are the same transition:
 * fill in whatever is left and fade the map's furniture back in.
 */
export type SceneReplayPhase = 'rest' | 'playing' | 'paused' | 'settling'

/** The per-frame state, mutated in place. Never spread into React. */
export interface SceneReplayFrame {
  /** Position along the run, 0..1. */
  progress: number
  /**
   * Opacity of the at-rest furniture — hulls, label hubs, region captions.
   * 1 at rest, 0 mid-replay, animating across both transitions.
   */
  decorationAlpha: number
  /** True while the reveal engine owns the paint. */
  active: boolean
}

export interface SceneReplayController {
  /** Null when this snapshot has no watchable history — then there is no affordance. */
  timeline: ReplayTimeline | null
  phase: SceneReplayPhase
  /**
   * Whether this transport ever moves on its own (PSY-1743).
   *
   * False under `prefers-reduced-motion`, where a run is a SCRUBBER rather than
   * a movie: it opens settled at now and the playhead moves only where the
   * visitor puts it. Consumers read this rather than the media query so the
   * transport and its controls cannot disagree about which kind of run is on
   * screen.
   */
  autoplays: boolean
  /** True whenever the replay layer is on screen (playing, paused, or settling). */
  isActive: boolean
  /**
   * The clock's current reading. Call INSIDE a paint callback or an event
   * handler — never during render, where the value would be a lie by the time
   * anything painted it.
   *
   * A getter rather than the ref itself, deliberately. Consumers do not need to
   * know a ref exists, and handing one across a component boundary defeats the
   * React Compiler: it infers `ref.current` as a dependency of any callback that
   * reads one it cannot prove is a ref, and then declines to optimise the whole
   * component. The returned object is MUTATED in place between calls, so it must
   * not be stored — read it again next frame.
   */
  readFrame: () => SceneReplayFrame
  /** Called after every advance, and after every seek. Returns an unsubscribe. */
  subscribe: (listener: (frame: SceneReplayFrame) => void) => () => void
  start: () => void
  /** Esc, the × chip, or reaching the end — all settle back to the at-rest map. */
  exit: () => void
  togglePause: () => void
  /** Jump to a position without changing whether the clock is running. */
  seek: (progress: number) => void
  /** Hold the clock while a drag is in progress, then release it. */
  setScrubbing: (scrubbing: boolean) => void
}

function clamp01(value: number): number {
  return value < 0 ? 0 : value > 1 ? 1 : value
}

/**
 * The replay transport for a snapshot, or an inert controller when there is
 * nothing to replay.
 *
 * Always called — the hook rules require it — so the "no timeline" case is a
 * controller whose `start` does nothing rather than a conditional hook.
 */
export function useSceneReplay(map: SceneMap | null): SceneReplayController {
  const timeline = useMemo(() => (map ? buildReplayTimeline(map) : null), [map])
  const [phase, setPhase] = useState<SceneReplayPhase>('rest')
  // The one reduced-motion decision the feature makes, taken here rather than at
  // each control, so `start` and `exit` and the scrubber's chrome all read the
  // same answer (PSY-1743).
  //
  // Read live rather than captured at entry, which means flipping the system
  // setting DURING a run leaves that run mid-flight in the other mode: an
  // already-playing run keeps playing, minus its pause chip. Deliberately not
  // handled — it needs an OS setting changed inside a 25-second window, Escape
  // and the close chip both still end the run, and the state machine that would
  // catch it costs more than the case is worth.
  const autoplays = !useReducedMotion()

  const frameRef = useRef<SceneReplayFrame>({ progress: 0, decorationAlpha: 1, active: false })
  const listenersRef = useRef(new Set<(frame: SceneReplayFrame) => void>())
  const rafRef = useRef<number | null>(null)
  const isScrubbingRef = useRef(false)
  // The phase the rAF loop reads. React state cannot be read from inside the
  // loop's closure without re-registering it every render, and re-registering a
  // rAF callback per render is how a replay ends up running two clocks.
  const phaseRef = useRef<SceneReplayPhase>('rest')
  // Wall-clock start of the current settle, so its 600ms is measured rather
  // than counted in frames (which would run at whatever rate the tab allows).
  const settleStartRef = useRef(0)
  const settleFromRef = useRef({ progress: 0, decorationAlpha: 0 })
  const enterStartRef = useRef(0)

  // A SNAPSHOT SWAPPED IN UNDER A LIVE RUN NEEDS NO HANDLING, and that is a
  // property of the design rather than an oversight. The nightly refetch can
  // replace the map mid-replay; because every drawn value is derived from
  // `progress` — a normalised position along the run, not a timestamp and not a
  // node count — the run simply continues against the new snapshot. The canvas
  // recomputes its reveal positions from the new timeline in the same memo it
  // rebuilds its render nodes in, and both eras of the history mean the same
  // thing at the same position. Ending the run instead would interrupt the
  // visitor to fix a problem they do not have.
  //
  // Losing the map ENTIRELY is the different case, and it is handled: see
  // `isActive` and the teardown effect below.

  const readFrame = useCallback(() => frameRef.current, [])

  const publish = useCallback(() => {
    for (const listener of listenersRef.current) listener(frameRef.current)
  }, [])

  const subscribe = useCallback((listener: (frame: SceneReplayFrame) => void) => {
    listenersRef.current.add(listener)
    // Fired immediately so a scrubber that mounts mid-run paints its current
    // position rather than a stale zero until the next frame.
    listener(frameRef.current)
    return () => {
      listenersRef.current.delete(listener)
    }
  }, [])

  const setPhaseBoth = useCallback((next: SceneReplayPhase) => {
    phaseRef.current = next
    setPhase(next)
  }, [])

  // ── The loop ───────────────────────────────────────────────────────────
  //
  // One rAF chain for the whole run, started on entry and torn down on rest.
  // It closes over refs only, so it never has to be rebuilt mid-run.
  const stopLoop = useCallback(() => {
    if (rafRef.current !== null) {
      window.cancelAnimationFrame(rafRef.current)
      rafRef.current = null
    }
  }, [])

  // The run's length, captured when the rAF chain starts. A snapshot that swaps
  // in mid-run therefore keeps pacing against the previous timeline's duration
  // for the rest of that run, which is deliberate rather than overlooked: the
  // duration is `bins.length * REPLAY_MS_PER_BIN` and the bin count saturates at
  // its target for any catalog large enough to be worth replaying, so two
  // consecutive nightly snapshots yield the same number. Re-reading it per frame
  // would buy nothing and would put the timeline back in the loop's closure.
  const durationMs = timeline?.durationMs ?? 0
  const runLoop = useCallback(() => {
    let previous = performance.now()

    const step = (now: number) => {
      rafRef.current = null
      // Clamped: a backgrounded tab hands back a delta of seconds, and an
      // unclamped one would skip most of the history in a single frame.
      const deltaMs = Math.min(100, Math.max(0, now - previous))
      previous = now
      const frame = frameRef.current
      const phaseNow = phaseRef.current

      // Nothing outside this loop moves the phase to `rest` today — the settle
      // below is the only path there, and it returns without rescheduling. This
      // is a self-destruct guard for the edit that adds a second one: without it
      // a chain would keep publishing frames over an at-rest map forever.
      if (phaseNow === 'rest') return

      if (phaseNow === 'settling') {
        const elapsed = now - settleStartRef.current
        const t = clamp01(elapsed / SETTLE_MS)
        const from = settleFromRef.current
        frame.progress = from.progress + (1 - from.progress) * t
        frame.decorationAlpha = from.decorationAlpha + (1 - from.decorationAlpha) * t
        if (t >= 1) {
          frame.progress = 1
          frame.decorationAlpha = 1
          frame.active = false
          publish()
          setPhaseBoth('rest')
          return
        }
      } else {
        // The decorations clear on their own clock, so entry does not have to
        // wait for them before the first arrivals land.
        frame.decorationAlpha = 1 - clamp01((now - enterStartRef.current) / ENTER_MS)
        if (phaseNow === 'playing' && !isScrubbingRef.current && durationMs > 0) {
          frame.progress = clamp01(frame.progress + deltaMs / durationMs)
          if (frame.progress >= 1) {
            settleStartRef.current = now
            settleFromRef.current = { progress: 1, decorationAlpha: frame.decorationAlpha }
            setPhaseBoth('settling')
          }
        }
      }

      publish()
      rafRef.current = window.requestAnimationFrame(step)
    }

    stopLoop()
    rafRef.current = window.requestAnimationFrame(step)
  }, [durationMs, publish, setPhaseBoth, stopLoop])

  const start = useCallback(() => {
    if (!timeline || phaseRef.current !== 'rest') return
    // Cleared defensively. A drag that never sees its `pointerup` — capture lost,
    // or the strip removed between down and up — would otherwise strand this
    // true, and the NEXT run would sit at zero forever with no clue why. A
    // silently dead feature is worth one assignment to rule out.
    isScrubbingRef.current = false
    const frame = frameRef.current
    // A REDUCED-MOTION RUN OPENS SETTLED AT NOW (PSY-1743). Same surface, same
    // seek, same way out — it simply starts at the end of the history instead of
    // the beginning, and paused instead of playing, so the visitor gets the
    // scrubber without being shown 25 seconds of unrequested motion. Standard
    // motion is untouched: it still opens at t0 and plays.
    frame.progress = autoplays ? 0 : 1
    frame.decorationAlpha = autoplays ? 1 : 0
    frame.active = true
    // Backdating the entry clock past its own duration makes the loop compute
    // the alpha the decoration fade would have ENDED on, so the clear happens in
    // no frames rather than in a second code path — the same
    // `motion-reduce:transition-none` the isolate band already applies,
    // expressed as a clock instead of a class.
    enterStartRef.current = performance.now() - (autoplays ? 0 : ENTER_MS)
    setPhaseBoth(autoplays ? 'playing' : 'paused')
    publish()
    // Started in BOTH modes, even though a paused run has nothing to advance.
    // The loop is what `exit` settles on, and a transport that only sometimes
    // has one is a transport that sometimes strands itself in `settling` when
    // the mode changes under it. The cost is a paused run's rAF chain — which a
    // standard run already pays for as long as the visitor leaves it paused.
    runLoop()
  }, [autoplays, publish, runLoop, setPhaseBoth, timeline])

  const exit = useCallback(() => {
    if (phaseRef.current === 'rest' || phaseRef.current === 'settling') return
    const frame = frameRef.current
    isScrubbingRef.current = false
    if (!autoplays) {
      // Reduced motion skips the settle for the reason it skips the autoplay:
      // the settle SWEEPS whatever history is left onto the map and crossfades
      // the furniture back over 600ms, which is the largest single piece of
      // motion this feature has. Land on the finished state in one frame
      // instead — the same picture, none of the sweep.
      stopLoop()
      frame.progress = 1
      frame.decorationAlpha = 1
      frame.active = false
      publish()
      setPhaseBoth('rest')
      return
    }
    settleStartRef.current = performance.now()
    settleFromRef.current = {
      progress: frame.progress,
      decorationAlpha: frame.decorationAlpha,
    }
    setPhaseBoth('settling')
    // The loop is already running in every phase this is reachable from, so it
    // picks the settle up on its next tick.
  }, [autoplays, publish, setPhaseBoth, stopLoop])

  const togglePause = useCallback(() => {
    if (phaseRef.current === 'playing') setPhaseBoth('paused')
    else if (phaseRef.current === 'paused') setPhaseBoth('playing')
  }, [setPhaseBoth])

  const seek = useCallback(
    (progress: number) => {
      if (phaseRef.current === 'rest' || phaseRef.current === 'settling') return
      frameRef.current.progress = clamp01(progress)
      publish()
    },
    [publish],
  )

  const setScrubbing = useCallback((scrubbing: boolean) => {
    isScrubbingRef.current = scrubbing
  }, [])

  // ── The run cannot outlive the map it plays ────────────────────────────
  //
  // The owner hands this hook a map only while the drawn map is the surface on
  // screen (see `GraphObservatory`), so a null timeline means the visitor has
  // navigated, re-rooted, or narrowed the window past the canvas breakpoint. The
  // controls that would stop a run have unmounted with that surface, so if the
  // transport did not notice, the rAF chain would keep publishing frames to
  // nothing and the header would keep claiming `REPLAY · …` with no way left to
  // stop it.
  //
  // Deliberately split in two halves, because this repo's lint rules forbid
  // both of the single-place options: `setState` inside an effect
  // (`react-hooks/set-state-in-effect`) and touching refs during a render
  // (`react-hooks/refs`). So the REACT half resets during render — the
  // documented "adjust state when a prop changes" pattern, and it terminates
  // because it can only fire on a timeline identity change — and the IMPERATIVE
  // half cancels the scheduled frame in an effect, which is where cancelling a
  // frame belongs anyway.
  // Tracks only WHETHER there is a map, never WHICH one. Holding the timeline
  // identity here would spin forever the moment a caller passed an unmemoised
  // map: each render would produce a new timeline, the comparison would fail, and
  // the state write would schedule the next render. A boolean cannot do that, and
  // "the map went away" is the only transition this needs to catch.
  const hasTimeline = timeline !== null
  const [hadTimeline, setHadTimeline] = useState(hasTimeline)
  if (hadTimeline !== hasTimeline) {
    setHadTimeline(hasTimeline)
    if (!hasTimeline && phase !== 'rest') setPhase('rest')
  }

  useEffect(() => {
    if (timeline) return
    stopLoop()
    isScrubbingRef.current = false
    phaseRef.current = 'rest'
    const frame = frameRef.current
    frame.progress = 0
    frame.decorationAlpha = 1
    frame.active = false
  }, [timeline, stopLoop])

  useEffect(() => stopLoop, [stopLoop])

  return useMemo(
    () => ({
      timeline,
      phase,
      autoplays,
      // Derived against the timeline as well as the phase, so the surface's
      // chrome cannot survive one render longer than the map does. The teardown
      // effect above lands on the same tick, but this is what makes the header
      // and the scrubber disappear in the SAME commit the map goes away in
      // rather than one frame later.
      isActive: timeline !== null && phase !== 'rest',
      readFrame,
      subscribe,
      start,
      exit,
      togglePause,
      seek,
      setScrubbing,
    }),
    [timeline, phase, autoplays, readFrame, subscribe, start, exit, togglePause, seek, setScrubbing],
  )
}

/**
 * The status text the header shows while a run is in flight, e.g.
 * `Replay · Mar 2019 · 412 on the map` — displayed upper case by the host's own
 * `uppercase` class, like every other mono status line on this page.
 *
 * A plain string builder rather than a component: it is written into the DOM by
 * a frame subscription, not rendered, so it must not depend on React at all.
 * The sentence after the prefix is `replayReadoutText`, shared with the
 * scrubber so the two cannot disagree.
 */
export function replayStatusText(timeline: ReplayTimeline, progress: number): string {
  return `Replay · ${replayReadoutText(timeline, progress)}`
}
