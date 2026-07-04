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
// Every scene at/above this count renders the SAME max dot ON PURPOSE: past it
// the dot would balloon over its neighbours (the bug this fixes). Finer
// magnitude among the dense tier is conveyed by the hover tooltip + which
// cities stay labelled when zoomed out — not by dot size.
const DOT_CAP_COUNT = 49
// Caps the variable (sqrt) part → max radius DOT_BASE_RADIUS + DOT_VARIABLE_MAX (0.5).
//
// PSY-1324 recalibration: the original 0.78 cap (variable max 0.5) still covered
// ~130 km of geography at the catalog's densest scene, enough for Chicago's
// capped disc to swallow Milwaukee (dot AND label) outright. The dense-tier
// boundary is a calibration decision independent of the cap's geometry, so the
// divisor is DERIVED from DOT_CAP_COUNT — shrinking the cap again can't
// silently move the boundary.
const DOT_VARIABLE_MAX = 0.22
const DOT_SQRT_DIVISOR = Math.sqrt(DOT_CAP_COUNT) / DOT_VARIABLE_MAX

export function sceneDotRadius(upcomingShowCount: number): number {
  // Non-finite guard: the type says `number`, but sibling fields (latitude/longitude)
  // are `number | null` from the API, so don't trust it blindly. A NaN radius would
  // poison the MERGED three.js point geometry (NaN bounding sphere) for the whole layer.
  const count = Number.isFinite(upcomingShowCount) ? Math.max(0, upcomingShowCount) : 0
  return DOT_BASE_RADIUS + Math.min(Math.sqrt(count) / DOT_SQRT_DIVISOR, DOT_VARIABLE_MAX)
}

// ── Dot altitude: smaller dots stack ABOVE larger ones (PSY-1324) ─────────
// react-globe.gl points are depth-tested 3D cylinders, so pointsData order does
// NOT decide which overlapping dot is visible — equal-height cylinders leave the
// larger disc covering the smaller one entirely (Chicago swallowing Milwaukee).
// Making cylinder height a strictly DECREASING function of count guarantees a
// less-dense neighbor's top face renders above a denser dot's, so it always
// peeks out of the overlap instead of disappearing.
//
// The offset is keyed to the RAW count, not the capped radius: radius saturates
// at DOT_CAP_COUNT, and a radius-derived offset would hand every capped pair
// (two co-dense adjacent metros — the likeliest overlaps as the catalog goes
// global) identical altitudes, resurrecting the z-fight exactly where it
// matters. With the count curve, ties need EXACTLY equal counts — and an
// equal-count pair is also equal-size, so neither dot dominates the other
// (the pre-fix status quo, not a regression).
//
// Range: base + (0, DOT_STACK_MAX] = (0.008, 0.016] — every dot stays above the
// PSY-1309 pulse rings (RING_ALTITUDE) and the whole band is far too small to
// read as "floating". The sqrt curve keeps neighboring dense counts ~1e-4
// globe-units apart, comfortably outside depth-buffer precision.
const DOT_BASE_ALTITUDE = 0.008
const DOT_STACK_MAX = 0.008
// The pulse rings' altitude (GlobeCanvas binds ringAltitude to this) — exported
// so the dots-above-rings invariant is structural, not comment-enforced.
export const RING_ALTITUDE = 0.006

export function sceneDotAltitude(upcomingShowCount: number): number {
  // Non-finite guard — see sceneDotRadius.
  const count = Number.isFinite(upcomingShowCount) ? Math.max(0, upcomingShowCount) : 0
  return DOT_BASE_ALTITUDE + DOT_STACK_MAX / (1 + Math.sqrt(count) / DOT_SQRT_DIVISOR)
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
// Followed scenes (PSY-1340 "My Scenes"): a distinct hue — not a brightness
// step of the base orange — so the followed set reads at continental zoom
// without colliding with the hover/selected affordance ramp.
export const DOT_COLOR_FOLLOWED = '#7ee8fa'
// Radius bump on hover — kept small so the merged point geometry rebuild (the
// accessors re-evaluate on hover change) never visibly pops neighbours.
export const DOT_HOVER_RADIUS_SCALE = 1.2

// Precedence: selected > hovered > followed > base — the transient affordance
// states stay legible on top of the persistent "my scene" marking.
export function sceneDotColor(
  slug: string,
  hoveredSlug: string | null,
  selectedSlug: string | null,
  followedSlugs?: ReadonlySet<string> | null,
): string {
  if (selectedSlug !== null && slug === selectedSlug) return DOT_COLOR_SELECTED
  if (hoveredSlug !== null && slug === hoveredSlug) return DOT_COLOR_HOVERED
  if (followedSlugs != null && followedSlugs.has(slug)) return DOT_COLOR_FOLLOWED
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
 * The FE-owned "liveliest first" ordering rule. The scenes API deliberately has
 * no ordering contract, so the frontend owns this — and owns it ONCE: search
 * (AtlasSearch), label prominence (visibleLabelScenes), and the mobile list all
 * sort with this comparator, so an evolution of the rule (e.g. a
 * shows_this_week tie-break) can't leave the surfaces disagreeing. Non-finite
 * counts sort last (a total order even on malformed data — the same defense the
 * size-scale guards apply).
 */
export function compareScenesByActivity<
  T extends { upcoming_show_count: number },
>(a: T, b: T): number {
  const av = Number.isFinite(a.upcoming_show_count)
    ? a.upcoming_show_count
    : -Infinity
  const bv = Number.isFinite(b.upcoming_show_count)
    ? b.upcoming_show_count
    : -Infinity
  if (av === bv) return 0
  return bv - av
}

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
    .sort(compareScenesByActivity)
    .slice(0, LABEL_TOP_K_FLOOR)
}
