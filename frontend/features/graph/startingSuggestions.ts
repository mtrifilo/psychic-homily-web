/**
 * Picking which starting suggestions the /graph zero state rotates through
 * (PSY-1749).
 *
 * The endpoint returns a POOL — the map's dozen most central artists — and this
 * module draws the small set one visit actually sees. Split out from the
 * component because it is the part with a rule worth asserting: the pool is
 * network input, the draw has to vary across visits, and neither is observable
 * through the rendered sentence.
 */

import type { GraphStartingPoint } from './hooks/useGraphStartingPoints'

/**
 * How many suggestions rotate in one visit.
 *
 * THREE, matching the trio the hardcoded list showed, because the sentence's
 * layout depends on it: the names share one grid cell so the line crossfades in
 * place, which means the slot reserves the WIDEST offered name's width. Rotating
 * the whole pool would size that slot to the longest of twelve band names and
 * leave a gap in the sentence for the other eleven.
 *
 * Variation comes from redrawing which three, not from showing more.
 */
export const SUGGESTION_ROTATION_SIZE = 3

/**
 * A suggestion the client is willing to render.
 *
 * The server already drops rows the catalog cannot honor; this is the boundary
 * check on top of that, because the alternative to an unusable entry here is a
 * button whose accessible name is `Search for undefined`.
 */
function isOfferable(candidate: GraphStartingPoint | null | undefined): candidate is GraphStartingPoint {
  return (
    !!candidate &&
    typeof candidate.artist_id === 'number' &&
    candidate.artist_id > 0 &&
    typeof candidate.artist_name === 'string' &&
    candidate.artist_name.length > 0 &&
    typeof candidate.artist_slug === 'string' &&
    candidate.artist_slug.length > 0
  )
}

/**
 * mulberry32 — a 32-bit PRNG seeded from one integer.
 *
 * SEEDED, not `Math.random()` called inline. The draw has to be a pure function
 * of its inputs so the component can hold it in a `useMemo` without the names
 * reshuffling under the reader on an unrelated re-render, and so a test can
 * assert a specific draw instead of asserting "some three of them". The
 * randomness the visitor experiences comes from the seed being drawn once per
 * mount.
 */
function createRandom(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let t = state
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/**
 * Draws the suggestions for one visit: up to {@link SUGGESTION_ROTATION_SIZE}
 * offerable entries from `pool`, in a seed-determined order.
 *
 * EVERY ENTRY IN THE POOL IS A VALID ANSWER — the server ranked them all by the
 * same centrality that decides which names the map draws largest — so the draw
 * is a uniform shuffle rather than a weighted one. Preferring the top of the
 * ranking is what produced the bug this replaces: a fixed order shows a fixed
 * first name, and the pool exists to stop that.
 *
 * Duplicate ids are collapsed. The server does not emit them, but two identical
 * names in one sentence is the kind of thing worth being unable to render
 * rather than worth trusting an upstream about.
 */
export function pickRotationSuggestions(
  pool: readonly GraphStartingPoint[],
  seed: number,
  size: number = SUGGESTION_ROTATION_SIZE,
): GraphStartingPoint[] {
  const seen = new Set<number>()
  const candidates: GraphStartingPoint[] = []
  for (const candidate of pool) {
    if (!isOfferable(candidate) || seen.has(candidate.artist_id)) continue
    seen.add(candidate.artist_id)
    candidates.push(candidate)
  }
  if (candidates.length === 0 || size <= 0) return []

  // Partial Fisher-Yates: only the prefix that is actually taken is shuffled.
  const random = createRandom(seed)
  const take = Math.min(size, candidates.length)
  for (let i = 0; i < take; i += 1) {
    const j = i + Math.floor(random() * (candidates.length - i))
    ;[candidates[i], candidates[j]] = [candidates[j], candidates[i]]
  }
  return candidates.slice(0, take)
}
