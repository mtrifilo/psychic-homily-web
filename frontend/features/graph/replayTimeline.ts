/**
 * The clock the growth replay runs on (PSY-1737).
 *
 * The map's history is not evenly distributed: the catalog starts as a trickle
 * of a few shows a year and ends as a nightly ingest, so a replay driven by the
 * CALENDAR would sit frozen for twenty seconds and then flash the last two years
 * past in one frame. Every other design here follows from refusing that.
 *
 * So playback is EVENT-PACED. The history is cut into ~250 quantile bins — equal
 * numbers of arrivals per bin, not equal spans of calendar time — and each bin
 * gets the same slice of wall clock. The sparse early era therefore fast-forwards
 * through years per beat while the dense recent era crawls through weeks, which
 * is the only pacing under which both eras are watchable. Nothing about the
 * CAMERA changes to achieve that: the map is drawn at its fitted whole-scene
 * zoom for the entire run, so the early era is already "pulled back".
 *
 * Everything downstream reads PROGRESS (0..1 along the run), never a raw
 * timestamp. That is what makes a seek and a playthrough interchangeable: a
 * node's reveal is `smoothstep((progress - node.revealAt) / FADE)`, a pure
 * function of one number, so the state at T cannot depend on how T was reached.
 * The calendar date is derived FROM progress for display only.
 *
 * Pure and framework-free — the pacing and the seek algebra are unit-tested
 * rather than inferred from watching an animation.
 */

import type { SceneMap } from './sceneMap'

/**
 * Bins the run aims for. Chosen with the run length below: 250 bins at 100ms
 * each is the ~25s full-history run and the "densest era ≈ 1 bin per 100ms"
 * cadence from the approved design. It is a TARGET, not a guarantee — a catalog
 * with fewer distinct arrival times than this yields fewer bins (see
 * `buildReplayTimeline`), and the run shortens to match rather than stretching
 * a handful of bins over 25 seconds of dead air.
 */
export const REPLAY_TARGET_BINS = 250

/** Wall-clock milliseconds each bin holds the screen. */
export const REPLAY_MS_PER_BIN = 100

/**
 * How much of the run a single node spends fading in, in PROGRESS units.
 *
 * Expressed against the bin count rather than in calendar seconds on purpose: a
 * calendar-width fade would be instant in the dense era (bins there span hours)
 * and last for years in the sparse one. Anchored to bins, an arrival always
 * takes the same ~1.5 beats to land no matter which era it lands in — and the
 * maths can never divide by a zero-width bin, which a tie-heavy ingest history
 * produces in quantity.
 */
export const REPLAY_FADE_BINS = 1.5

/**
 * How long a fresh arrival wears the pulse colour, in the same units. Slightly
 * longer than the fade so a dot finishes arriving before it settles to its
 * community colour, rather than doing both at once.
 */
export const REPLAY_PULSE_BINS = 2.5

/** A replay needs at least this many distinct arrival times to be worth offering. */
export const REPLAY_MIN_BINS = 8

/** One quantile bin: an equal-arrivals slice of the history. */
export interface ReplayBin {
  /** First appear-second in the bin. */
  start: number
  /** Last appear-second in the bin. Equal to `start` for a bin of pure ties. */
  end: number
  /** Nodes whose appear time falls in the bin — the histogram bar's height. */
  count: number
}

export interface ReplayTimeline {
  bins: ReplayBin[]
  /** Total wall-clock length of an uninterrupted run, in milliseconds. */
  durationMs: number
  /** Fade width in progress units, precomputed so the paint path never divides. */
  fadeProgress: number
  /** Pulse width in progress units. */
  pulseProgress: number
  /** Origin the appear seconds count from. */
  epoch: Date
  /**
   * Every node's reveal position in progress units, ASCENDING. The reveal maths
   * reads it by node; the "how many are on the map" readout binary-searches it.
   */
  sortedNodeReveals: Float64Array
  /** Reveal position in progress units, by node id. */
  revealByNodeId: Map<number, number>
  /** The tallest bar, so the histogram can normalise without a second pass. */
  maxBinCount: number
}

/**
 * Cut a snapshot's arrival times into the timeline the replay plays back.
 *
 * Returns null when there is nothing to watch: a snapshot with no `appear`
 * column at all (every node reads 0), or one whose whole history collapses into
 * fewer than `REPLAY_MIN_BINS` distinct moments. Both would render as a map that
 * blinks into existence in one frame, which is worse than not offering the
 * affordance — so the caller reads null as "no WATCH IT GROW chip".
 */
export function buildReplayTimeline(map: SceneMap): ReplayTimeline | null {
  if (map.nodes.length === 0) return null
  if (Number.isNaN(map.epoch.getTime())) return null

  const appears = new Float64Array(map.nodes.length)
  for (let i = 0; i < map.nodes.length; i += 1) {
    const value = map.nodes[i].appear
    appears[i] = Number.isFinite(value) ? value : 0
  }
  appears.sort()

  const bins = quantileBins(appears, REPLAY_TARGET_BINS)
  if (bins.length < REPLAY_MIN_BINS) return null

  const fadeProgress = REPLAY_FADE_BINS / bins.length
  const pulseProgress = REPLAY_PULSE_BINS / bins.length

  // Reveal positions are computed ONCE per snapshot. Doing it per frame would
  // mean a binary search per node per frame — the exact per-frame cost the
  // fixed-layout design exists to avoid.
  const revealByNodeId = new Map<number, number>()
  for (const node of map.nodes) {
    revealByNodeId.set(node.id, progressAtAppear(bins, node.appear))
  }
  const sortedNodeReveals = new Float64Array(appears.length)
  for (let i = 0; i < appears.length; i += 1) {
    sortedNodeReveals[i] = progressAtAppear(bins, appears[i])
  }
  // `appears` is already ascending and `progressAtAppear` is monotonic, so the
  // mapped array is ascending too — no second sort.

  let maxBinCount = 0
  for (const bin of bins) {
    if (bin.count > maxBinCount) maxBinCount = bin.count
  }

  return {
    bins,
    durationMs: bins.length * REPLAY_MS_PER_BIN,
    fadeProgress,
    pulseProgress,
    epoch: map.epoch,
    sortedNodeReveals,
    revealByNodeId,
    maxBinCount,
  }
}

/**
 * Cut an ASCENDING array of arrival times into at most `target` equal-count bins.
 *
 * Two rules do the real work, and both come from tied timestamps — which are not
 * an edge case here but the normal shape of the data (a catalog import stamps
 * thousands of rows with one `created_at`; a festival stamps a hundred artists
 * with one show date):
 *
 *  1. A BIN BOUNDARY NEVER SPLITS A TIE. Two nodes that arrived at the same
 *     instant must reveal on the same frame — splitting them would make the
 *     rendered state depend on which side of an arbitrary cut each landed on.
 *     So a boundary is pushed forward past the whole tie group.
 *  2. RULE 1 IS WHAT GIVES THE HISTOGRAM ITS SHAPE. Perfect quantile bins hold
 *     equal counts and would draw a flat bar chart that says nothing. Absorbing
 *     a tie group makes that bin genuinely taller, so the bars read as what they
 *     are: the bursts in the scene's history.
 *
 * Fewer bins than `target` therefore come back whenever the history has fewer
 * distinct moments than that, which is the honest answer rather than a padded one.
 */
export function quantileBins(ascendingAppears: ArrayLike<number>, target: number): ReplayBin[] {
  const total = ascendingAppears.length
  if (total === 0 || target <= 0) return []

  const bins: ReplayBin[] = []
  let start = 0
  while (start < total) {
    // Re-divided from what is LEFT rather than from a fixed stride. A bin that
    // absorbed a large tie group has eaten into later bins' share, and a fixed
    // stride would then emit single-item bins until the nominal cut caught back
    // up — turning one burst into a hundred degenerate bars. Re-dividing keeps
    // the remaining bins even and caps the total at `target`.
    const remainingBins = target - bins.length
    let end =
      remainingBins <= 1
        ? total
        : Math.min(total, start + Math.max(1, Math.round((total - start) / remainingBins)))
    // Pushed past any tie straddling the cut.
    const boundaryValue = ascendingAppears[end - 1]
    while (end < total && ascendingAppears[end] === boundaryValue) end += 1
    bins.push({
      start: ascendingAppears[start],
      end: ascendingAppears[end - 1],
      count: end - start,
    })
    start = end
  }
  return bins
}

/**
 * Where an arrival time sits along the run, in progress units (0..1).
 *
 * Monotonic non-decreasing in `appear`, which is what lets the caller map a
 * sorted array of arrival times without re-sorting the result. Inside a bin the
 * position interpolates linearly, so the clock reads continuously rather than
 * stepping 250 times; a zero-width bin (a pure tie) contributes its whole slice
 * at the bin's own boundary instead of dividing by zero.
 */
export function progressAtAppear(bins: readonly ReplayBin[], appear: number): number {
  if (bins.length === 0) return 0
  const value = Number.isFinite(appear) ? appear : 0
  if (value <= bins[0].start) return 0
  const last = bins[bins.length - 1]
  if (value >= last.end) return 1

  // Binary search for the bin whose [start, end] contains the value. Bins are
  // contiguous in rank but NOT in value (bin i's end and bin i+1's start are
  // two different arrivals), so a value can land in the gap between them — it
  // is then attributed to the start of the later bin, which keeps the mapping
  // monotonic.
  let low = 0
  let high = bins.length - 1
  while (low < high) {
    const mid = (low + high) >> 1
    if (value > bins[mid].end) low = mid + 1
    else high = mid
  }
  const bin = bins[low]
  const span = bin.end - bin.start
  const within = span > 0 ? Math.min(1, Math.max(0, (value - bin.start) / span)) : 0
  return (low + within) / bins.length
}

/** The appear-second the run is showing at `progress`. Inverse of `progressAtAppear`. */
export function appearAtProgress(timeline: ReplayTimeline, progress: number): number {
  const bins = timeline.bins
  if (bins.length === 0) return 0
  const clamped = Math.min(1, Math.max(0, progress))
  const scaled = clamped * bins.length
  const index = Math.min(bins.length - 1, Math.floor(scaled))
  const bin = bins[index]
  return bin.start + (bin.end - bin.start) * (scaled - index)
}

/**
 * How much of a node has arrived at `progress`: 0 before it, 1 after its fade.
 *
 * The ONE function the canvas, the tests and the readout all go through, so
 * "the state at T" has exactly one definition. Deliberately takes plain numbers
 * rather than a node: the paint path calls it once per node per frame.
 */
export function replayReveal(progress: number, revealAt: number, fadeProgress: number): number {
  const t = (progress - revealAt) / fadeProgress
  if (t <= 0) return 0
  if (t >= 1) return 1
  return t * t * (3 - 2 * t)
}

/** Whether a node arrived recently enough to still be wearing the pulse colour. */
export function replayIsPulsing(
  progress: number,
  revealAt: number,
  pulseProgress: number,
): boolean {
  const age = progress - revealAt
  return age >= 0 && age < pulseProgress
}

/**
 * How many nodes have started arriving at `progress`.
 *
 * Counts anything whose reveal has BEGUN rather than finished: the status line
 * should agree with what a visitor can see, and a dot mid-fade is on the map.
 */
export function revealedNodeCount(timeline: ReplayTimeline, progress: number): number {
  const values = timeline.sortedNodeReveals
  let low = 0
  let high = values.length
  while (low < high) {
    const mid = (low + high) >> 1
    if (values[mid] <= progress) low = mid + 1
    else high = mid
  }
  return low
}

/** The calendar date the run is showing at `progress`. */
export function replayDateAt(timeline: ReplayTimeline, progress: number): Date {
  return new Date(timeline.epoch.getTime() + appearAtProgress(timeline, progress) * 1000)
}

/**
 * The date readout beside the transport, and the one in the status line.
 *
 * Month + year, not a full date: the clock is a quantile of arrivals rather than
 * a calendar cursor, so a day-precision readout would claim an accuracy the
 * pacing does not have.
 */
export function formatReplayDate(date: Date): string {
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short' })
}
