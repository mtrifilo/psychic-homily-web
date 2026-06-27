/**
 * Pure dot-size, label-size, and label-visibility scaling for the Atlas globe
 * (PSY-1223).
 *
 * Kept free of react-globe.gl (like globeTypes.ts) so it unit-tests without
 * loading three.js. GlobeCanvas binds these to the <Globe> props; the scaling
 * decisions live here, in one tunable place, instead of inline in the JSX.
 *
 * Two problems this solves on the shipped globe (PSY-1213):
 *   1. Dot/label size scaled by `sqrt(count)` UNCAPPED, so very dense scenes
 *      (Chicago ~283) ballooned over their neighbours.
 *   2. Every scene carried an always-on label, so adjacent dense cities
 *      (Minneapolis / St. Paul, ~10 mi apart) overlapped at the default zoom.
 */

// ── Dot radius ────────────────────────────────────────────────────────────
// sqrt scale (so dense scenes don't dwarf small ones) with the high end CAPPED
// so a 283-show scene doesn't render a giant dot that swallows its neighbours.
// `count` is upcoming_show_count; radius is in react-globe.gl globe-radius units.
const DOT_BASE_RADIUS = 0.28
const DOT_SQRT_DIVISOR = 14
// Caps the variable (sqrt) part → max radius DOT_BASE_RADIUS + DOT_VARIABLE_MAX (0.78).
// The cap is reached at count ≈ (DOT_VARIABLE_MAX*DOT_SQRT_DIVISOR)^2 ≈ 49, so every
// scene above ~49 upcoming shows renders the SAME max dot ON PURPOSE: past that the dot
// would balloon over its neighbours (the bug this fixes). Finer magnitude among the
// dense tier is conveyed by the hover tooltip + which cities stay labelled when zoomed
// out — not by dot size.
const DOT_VARIABLE_MAX = 0.5

export function sceneDotRadius(upcomingShowCount: number): number {
  // Non-finite guard: the type says `number`, but sibling fields (latitude/longitude)
  // are `number | null` from the API, so don't trust it blindly. A NaN radius would
  // poison the MERGED three.js point geometry (NaN bounding sphere) for the whole layer.
  const count = Number.isFinite(upcomingShowCount) ? Math.max(0, upcomingShowCount) : 0
  return DOT_BASE_RADIUS + Math.min(Math.sqrt(count) / DOT_SQRT_DIVISOR, DOT_VARIABLE_MAX)
}

// ── Label text size ───────────────────────────────────────────────────────
// Same shape as the dot, capped so dense-scene labels stay readable without
// dominating the map.
const LABEL_BASE_SIZE = 0.5
const LABEL_SQRT_DIVISOR = 32
const LABEL_VARIABLE_MAX = 0.35

export function sceneLabelSize(upcomingShowCount: number): number {
  // Non-finite guard — see sceneDotRadius.
  const count = Number.isFinite(upcomingShowCount) ? Math.max(0, upcomingShowCount) : 0
  return LABEL_BASE_SIZE + Math.min(Math.sqrt(count) / LABEL_SQRT_DIVISOR, LABEL_VARIABLE_MAX)
}

// ── Zoom-gated labels ─────────────────────────────────────────────────────
// The dots are always shown; the always-on LABELS are gated by camera altitude.
// The further out the camera (higher altitude), the higher the upcoming-show
// count a scene must clear to keep its label — so a continental view labels only
// the densest cities (and an adjacent pair like Minneapolis/St. Paul shows just
// the bigger one), while zooming in progressively reveals the rest.
//
// Thresholds are step functions, not a continuous curve, so `labelsData` only
// changes — and react-globe.gl only rebuilds label geometry — when the camera
// crosses a threshold, not on every micro-zoom (PSY-1213 memoized pointsData by
// reference for the same reason).
//
// Both entry POVs land in the continental (>=1.5) bucket: the default 1.8 and the
// geo-resolved 1.6 (AtlasGlobe). Calibrated against the 2026-06 catalog, where
// Chicago (~283) and Minneapolis (~187) clear the continental 120 but the adjacent
// St. Paul (~95) does not — that's the decluttered pair the AC names. Two known
// limits, both deferred to PSY-1229: counts are SEASONAL so a fixed absolute
// threshold can leave the continental view sparse in a quiet stretch (wants a top-K
// floor); and the gate is by COUNT, not proximity, so two co-dense adjacent cities
// both above the threshold would still overlap.
export function labelMinCountForAltitude(altitude: number): number {
  if (altitude >= 1.5) return 120 // continental — only the very densest scenes
  if (altitude >= 1.0) return 40 // multi-region
  if (altitude >= 0.6) return 10 // metro cluster
  return 0 // zoomed in — label everything
}

/**
 * The subset of scenes that carry an always-on label at the given threshold
 * (from `labelMinCountForAltitude`). Scenes below it still render their dot and
 * hover tooltip — only the persistent label is withheld until you zoom in.
 *
 * Takes the discrete `minCount` (not raw altitude) so the caller can memoize on
 * the threshold and keep the array identity stable between threshold crossings —
 * react-globe.gl diffs labelsData by reference.
 */
export function visibleLabelScenes<T extends { upcoming_show_count: number }>(
  scenes: readonly T[],
  minCount: number,
): T[] {
  if (minCount <= 0) return scenes as T[]
  return scenes.filter((s) => s.upcoming_show_count >= minCount)
}
