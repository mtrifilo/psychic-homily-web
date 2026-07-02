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

// ── Dot color + hover/selected affordance (PSY-1312) ─────────────────────
// Uniform orange gave the dots no affordance feedback: nothing signalled
// clickability until you clicked, and with the preview panel open you couldn't
// see WHICH dot you were reading about. Three states, one place:
//   selected → cream (matches the label color, persistent while the panel is open)
//   hovered  → brightened orange (+ pointer cursor + slight radius bump)
//   base     → the shipped dot orange
// Selected wins over hovered so re-hovering the open scene doesn't flicker.
export const DOT_COLOR_BASE = '#ff7a3c'
export const DOT_COLOR_HOVERED = '#ffb066'
export const DOT_COLOR_SELECTED = '#ffe6c2'
// Radius bump on hover — kept small so the merged point geometry rebuild (the
// accessors re-evaluate on hover change) never visibly pops neighbours.
export const DOT_HOVER_RADIUS_SCALE = 1.2

export function sceneDotColor(
  slug: string,
  hoveredSlug: string | null,
  selectedSlug: string | null,
): string {
  if (selectedSlug !== null && slug === selectedSlug) return DOT_COLOR_SELECTED
  if (hoveredSlug !== null && slug === hoveredSlug) return DOT_COLOR_HOVERED
  return DOT_COLOR_BASE
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
// St. Paul (~95) does not — that's the decluttered pair the AC names.
//
// Two limits surfaced in the PSY-1223 review. PSY-1229 fixes the first with the
// top-K floor below: counts are SEASONAL, so a fixed absolute threshold could
// leave the continental view with ZERO labels in a quiet stretch. The second
// (the gate is by COUNT, not proximity, so two co-dense adjacent cities both
// above the threshold could still overlap) is deferred — the count gate handles
// the real cases today; a per-frame projection-based declutter is the upgrade
// only if real data proves it insufficient (PSY-1229 open question).
export function labelMinCountForAltitude(altitude: number): number {
  if (altitude >= 1.5) return 120 // continental — only the very densest scenes
  if (altitude >= 1.0) return 40 // multi-region
  if (altitude >= 0.6) return 10 // metro cluster
  return 0 // zoomed in — label everything
}

// Quiet-season floor (PSY-1229): when the absolute threshold clears NOTHING,
// label this many of the densest scenes so a zoomed-out view is never empty.
// Only the empty case falls back — a normal season keeps its calibrated, sparser
// label set (so PSY-1223's 2-label continental view + the Minneapolis/St. Paul
// declutter are preserved), which is why K can be generous. This exported
// constant is the single tuning knob for the floor; fixed (not viewport-scaled).
export const LABEL_TOP_K_FLOOR = 5

/**
 * The subset of scenes that carry an always-on label at the given threshold
 * (from `labelMinCountForAltitude`). Scenes below it still render their dot and
 * hover tooltip — only the persistent label is withheld until you zoom in.
 *
 * Normally this is just the threshold gate (`count >= minCount`). The PSY-1229
 * floor adds one safety net: if the threshold clears NOTHING (a seasonal dip
 * where no city reaches the continental count), fall back to the
 * `LABEL_TOP_K_FLOOR` densest so the zoomed-out view is never empty. The
 * fallback fires ONLY on an empty result, so a normal season is untouched — the
 * calibrated continental label set (and the Minneapolis/St. Paul declutter) is
 * preserved.
 *
 * @param minCount the DISCRETE threshold from `labelMinCountForAltitude` (a step
 *   value, NOT raw altitude). The caller memoizes on it to keep the returned
 *   array's identity stable between threshold crossings — react-globe.gl diffs
 *   labelsData by reference, so passing raw altitude would churn it every frame.
 */
export function visibleLabelScenes<T extends { upcoming_show_count: number }>(
  scenes: readonly T[],
  minCount: number,
): T[] {
  // Every path returns a fresh array, so the result is always safe to mutate.
  if (minCount <= 0) return scenes.slice()
  // NaN counts fail `>= minCount` (NaN comparisons are always false), so a
  // malformed scene is naturally excluded from the gate.
  const qualifiers = scenes.filter((s) => s.upcoming_show_count >= minCount)
  if (qualifiers.length > 0) return qualifiers
  // The threshold cleared NOTHING (a seasonal dip) — fall back to the
  // LABEL_TOP_K_FLOOR densest so the zoomed-out view is never empty. Firing only
  // on empty keeps a normal season untouched (no re-clutter). Non-finite counts
  // are excluded outright (never floored in), matching the size-scale guards.
  return scenes
    .filter((s) => Number.isFinite(s.upcoming_show_count))
    .sort((a, b) => b.upcoming_show_count - a.upcoming_show_count)
    .slice(0, LABEL_TOP_K_FLOOR)
}
