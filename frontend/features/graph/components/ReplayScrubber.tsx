'use client'

/**
 * The growth replay's transport (PSY-1737) — a histogram brush that replaces the
 * freshness strip for as long as a run is in flight, and only then.
 *
 * The bars are the scene's own history: one per quantile bin, as tall as the
 * number of artists that arrived in it. Because a bin boundary is never allowed
 * to split a tie (see `quantileBins`), the bursts — a catalog import, a festival
 * bill — come through as the tall bars they were, and the trickle years read as
 * the flat run they were.
 *
 * NOTHING HERE RE-RENDERS DURING PLAYBACK. The bars are built once per snapshot;
 * the played region, the playhead and the date readout are written straight into
 * the DOM from the transport's frame subscription. A 250-bar chart re-rendered
 * sixty times a second is the same mistake as a React-state playhead, one layer
 * out.
 *
 * Drawn with DOM boxes rather than a canvas or a charting library on purpose: at
 * 250 static bars the DOM is cheaper than a second canvas context, it inherits
 * the theme tokens for free, and the whole thing stays keyboard-operable as an
 * ordinary slider.
 */

import { memo, useCallback, useEffect, useMemo, useRef } from 'react'
import { Pause, Play, X } from 'lucide-react'

import {
  replayBinIndexAt,
  replayBinStep,
  replayReadoutText,
  type ReplayTimeline,
} from '../replayTimeline'
import type { SceneReplayController } from '../useSceneReplay'

/** Shortest bar drawn, as a fraction of the track height — an empty-looking bin still reads as a bin. */
const MIN_BAR_FRACTION = 0.08

/**
 * The mono primary-tinted chip the map card's small controls wear: the
 * `WATCH IT GROW` entry point and the `THIS WEEK` share link on the freshness
 * footer, and this transport's pause control.
 *
 * One constant for the same reason `SHUFFLE_PILL_CLASS` is one: a hover or
 * focus-ring spec copied into two files drifts (the first two had already
 * disagreed on their ring offset before they landed). Add `shrink-0` at a call
 * site inside a flex row.
 *
 * Named for the ROW rather than for one feature. It started life as
 * `REPLAY_CHIP_CLASS` and was renamed when the share link became the third
 * wearer: a share link reaching for a constant named after the replay reads as
 * a mistake every time someone finds it.
 */
export const MAP_CARD_CHIP_CLASS =
  'inline-flex items-center gap-1.5 rounded-full border border-primary/40 bg-primary/10 px-2.5 py-1 font-mono text-[10px] uppercase tracking-wider text-primary transition-colors hover:border-primary/60 hover:bg-primary/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background'

export interface ReplayScrubberProps {
  replay: SceneReplayController
  timeline: ReplayTimeline
}

export function ReplayScrubber({ replay, timeline }: ReplayScrubberProps) {
  const { subscribe, seek, setScrubbing, togglePause, exit, phase, readFrame, autoplays } = replay

  const trackRef = useRef<HTMLDivElement>(null)
  const playedRef = useRef<HTMLDivElement>(null)
  const playheadRef = useRef<HTMLDivElement>(null)
  const readoutRef = useRef<HTMLSpanElement>(null)
  // Last strings written, so a frame that does not change the readout does not
  // touch the DOM at all — 60 identical textContent writes a second is layout
  // work for nothing.
  const lastReadoutRef = useRef('')
  const lastValueNowRef = useRef(-1)
  // The bin the readout was last built for. Two `Intl` formatters (a date and a
  // thousands-separated count) are the most expensive thing on this path, and
  // the date only carries month precision — so they run once per BIN rather
  // than once per frame, which at 250 bins over a 25s run is ~10 times a second
  // instead of 60.
  const lastReadoutBinRef = useRef(-1)

  const bars = useMemo(() => {
    const max = Math.max(1, timeline.maxBinCount)
    return timeline.bins.map(
      bin => MIN_BAR_FRACTION + (1 - MIN_BAR_FRACTION) * (bin.count / max),
    )
  }, [timeline])

  // ── The per-frame writes ───────────────────────────────────────────────
  useEffect(
    () =>
      subscribe(frame => {
        const percent = frame.progress * 100
        // `clip-path` on an overlaid copy of the bars, so the played region is
        // the SAME geometry in orange rather than a second bar list to keep in
        // sync. Mutating it per frame does re-raster the played layer (only
        // CSS-animated clip-paths composite), but the invalidated area is a
        // ~600x24px strip, which is genuinely cheap.
        if (playedRef.current) {
          playedRef.current.style.clipPath = `inset(0 ${100 - percent}% 0 0)`
        }
        // A percentage `left` rather than a transform, knowingly: a transform
        // would need the track's width in px (percentage transforms resolve
        // against the ELEMENT, and this element is 1px wide), which means
        // measuring it and tracking resizes. The layout this invalidates is a
        // 1px box inside a strip the clip-path above already re-rasters, so the
        // measurement machinery would buy nothing real.
        if (playheadRef.current) {
          playheadRef.current.style.left = `${percent}%`
        }
        // The readout is rebuilt only when the BIN changes, not every frame. It
        // carries month precision and a count that moves in steps, so at 250
        // bins over a 25s run this is ~10 rebuilds a second instead of 60 — and
        // each one is two `Intl` formats, the most expensive thing on this path.
        const binIndex = replayBinIndexAt(timeline, frame.progress)
        if (binIndex !== lastReadoutBinRef.current) {
          lastReadoutBinRef.current = binIndex
          const text = replayReadoutText(timeline, frame.progress)
          if (text !== lastReadoutRef.current) {
            lastReadoutRef.current = text
            if (readoutRef.current) readoutRef.current.textContent = text
            if (trackRef.current) trackRef.current.setAttribute('aria-valuetext', text)
          }
        }
        // Whole percents only: assistive technology announces this, and a value
        // that changes sixty times a second announces nothing usefully.
        const valueNow = Math.round(percent)
        if (trackRef.current && valueNow !== lastValueNowRef.current) {
          lastValueNowRef.current = valueNow
          trackRef.current.setAttribute('aria-valuenow', String(valueNow))
        }
      }),
    [subscribe, timeline],
  )

  // ── Seeking ────────────────────────────────────────────────────────────
  const seekToClientX = useCallback(
    (clientX: number) => {
      const track = trackRef.current
      if (!track) return
      const rect = track.getBoundingClientRect()
      if (rect.width <= 0) return
      seek((clientX - rect.left) / rect.width)
    },
    [seek],
  )

  const handlePointerDown = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      // Pointer capture, not a window listener: the drag must survive the
      // pointer leaving a 24px-tall strip, which it does constantly.
      event.currentTarget.setPointerCapture(event.pointerId)
      // The clock is held for the duration of the drag. Without this the
      // playhead fights the pointer — every frame advances it away from where
      // the drag just put it.
      setScrubbing(true)
      seekToClientX(event.clientX)
    },
    [seekToClientX, setScrubbing],
  )

  const handlePointerMove = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      if (!event.currentTarget.hasPointerCapture(event.pointerId)) return
      seekToClientX(event.clientX)
    },
    [seekToClientX],
  )

  const handlePointerUp = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId)
      }
      // Playback resumes from wherever the drag left it, in whatever state it
      // was in before — a paused run stays paused.
      setScrubbing(false)
    },
    [setScrubbing],
  )

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      // The transport's own step, from the module that defines the pacing — not
      // re-derived here from bin internals.
      const step = replayBinStep(timeline)
      const current = readFrame().progress
      switch (event.key) {
        case 'ArrowLeft':
        case 'ArrowDown':
          seek(current - step)
          break
        case 'ArrowRight':
        case 'ArrowUp':
          seek(current + step)
          break
        case 'PageDown':
          seek(current - step * 10)
          break
        case 'PageUp':
          seek(current + step * 10)
          break
        case 'Home':
          seek(0)
          break
        case 'End':
          seek(1)
          break
        default:
          return
      }
      event.preventDefault()
    },
    [readFrame, seek, timeline],
  )

  const isPaused = phase === 'paused'

  const readout = (
    <span
      ref={readoutRef}
      // Reserved so the strip does not resize as the date and the count
      // grow digits under the playhead.
      className="min-w-[13ch] text-left tabular-nums"
    />
  )

  return (
    <div className="flex items-center gap-3 border-t border-border/50 px-4 py-2">
      {autoplays ? (
        <button
          type="button"
          onClick={togglePause}
          aria-label={isPaused ? 'Resume the replay' : 'Pause the replay'}
          className={`${MAP_CARD_CHIP_CLASS} shrink-0`}
        >
          {isPaused ? (
            <Play className="size-3" aria-hidden="true" />
          ) : (
            <Pause className="size-3" aria-hidden="true" />
          )}
          {readout}
        </button>
      ) : (
        // NO PLAY CONTROL UNDER REDUCED MOTION (PSY-1743). A transport that
        // never runs on its own has nothing to pause, and a play chip here
        // would be worse than absent either way: at the position this run opens
        // in — the end — pressing it advances nothing and closes the replay.
        // The readout stays, because it is the label for where the playhead is,
        // and it keeps the strip's geometry identical in both modes.
        <span className="inline-flex shrink-0 items-center px-2.5 py-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
          {readout}
        </span>
      )}

      <div
        ref={trackRef}
        role="slider"
        tabIndex={0}
        aria-label="Replay position"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={0}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
        onKeyDown={handleKeyDown}
        className="relative h-6 flex-1 cursor-pointer touch-none select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      >
        <BarRow bars={bars} className="text-muted-foreground/35" />
        {/* The same bars again, in the played colour, revealed by the clip. */}
        <div ref={playedRef} className="absolute inset-0" style={{ clipPath: 'inset(0 100% 0 0)' }}>
          <BarRow bars={bars} className="text-primary" />
        </div>
        <div
          ref={playheadRef}
          aria-hidden="true"
          className="pointer-events-none absolute inset-y-0 w-px -translate-x-1/2 bg-primary"
          style={{ left: '0%' }}
        />
      </div>

      <button
        type="button"
        onClick={exit}
        aria-label="Close the replay and return to the map"
        className="inline-flex shrink-0 items-center gap-1 rounded-full border border-border/60 px-2 py-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground transition-colors hover:border-border hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      >
        Esc
        <X className="size-3" aria-hidden="true" />
      </button>
    </div>
  )
}

/**
 * One pass of bars. Rendered twice — once muted, once in the played colour —
 * from the same array, so the two rows cannot drift apart. `currentColor` is
 * what lets the caller set the colour with a class instead of duplicating the
 * geometry per theme.
 *
 * Memoised because `bars` is stable per snapshot and the class is a literal, so
 * with 250 bars per row this skips 500 element reconciliations (and 500 fresh
 * style objects) on every re-render of the strip — which happens on each phase
 * change and on any hover or hub selection elsewhere in the card during a run.
 */
const BarRow = memo(function BarRow({
  bars,
  className,
}: {
  bars: number[]
  className: string
}) {
  return (
    <div
      aria-hidden="true"
      className={`absolute inset-0 flex items-end gap-px overflow-hidden ${className}`}
    >
      {bars.map((height, index) => (
        <div
          // Bars are a fixed-length list derived from an immutable snapshot and
          // are never reordered, inserted, or removed, so the index IS the
          // stable identity here.
          key={index}
          className="min-w-px flex-1 bg-current"
          style={{ height: `${height * 100}%` }}
        />
      ))}
    </div>
  )
})
