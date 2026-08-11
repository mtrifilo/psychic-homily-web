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

import {
  anchorFromCatalogTarget,
  type CatalogArtistTarget,
  type GraphAnchor,
} from './graphAnchor'

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
 * offerable anchors from `pool`, in a seed-determined order.
 *
 * EVERY ENTRY IN THE POOL IS A VALID ANSWER — the server ranked them all by the
 * same centrality that decides which names the map draws largest — so the draw
 * is a UNIFORM SHUFFLE over the pool rather than a window into it. A window
 * would be three lines shorter and would satisfy "varies across visits" just as
 * well, but it can only ever offer names that are adjacent in the ranking; the
 * shuffle can put the map's first and eleventh hub in the same sentence, which
 * is the point of ranking twelve and showing three.
 *
 * Duplicate ids are collapsed. The server does not emit them, but two identical
 * names in one sentence is the kind of thing worth being unable to render
 * rather than worth trusting an upstream about.
 */
export function pickRotationSuggestions(
  pool: readonly CatalogArtistTarget[],
  seed: number,
): GraphAnchor[] {
  const seen = new Set<number>()
  const candidates: GraphAnchor[] = []
  for (const candidate of pool) {
    // ONE narrowing rule for every source the graph centres on — see
    // graphAnchor.ts. The server already drops rows the catalog cannot honour;
    // this is the boundary check on top of that, because the alternative to an
    // unusable entry is a button announcing "Search for undefined".
    const anchor = anchorFromCatalogTarget(candidate)
    if (!anchor || seen.has(anchor.id)) continue
    seen.add(anchor.id)
    candidates.push(anchor)
  }
  if (candidates.length === 0) return []

  // Partial Fisher-Yates: only the prefix that is actually taken is shuffled.
  const random = createRandom(seed)
  const take = Math.min(SUGGESTION_ROTATION_SIZE, candidates.length)
  for (let i = 0; i < take; i += 1) {
    const j = i + Math.floor(random() * (candidates.length - i))
    ;[candidates[i], candidates[j]] = [candidates[j], candidates[i]]
  }
  return candidates.slice(0, take)
}
