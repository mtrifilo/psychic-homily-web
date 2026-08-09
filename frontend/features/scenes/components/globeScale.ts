/**
 * Pure dot-size, label-size, label-visibility scaling AND tooltip copy for the
 * Atlas globe (PSY-1223).
 *
 * Copy lives here too, not just geometry (PSY-1732): the same argument that
 * keeps the scaling out of the canvas keeps the wording out of it, and this is
 * the only globe module a unit test can import. The venue tooltip's meta line
 * is still composed inline in GlobeCanvas — it predates this and is not a
 * counter-precedent; new globe copy belongs here.
 *
 * Kept free of the rendering library (like globeTypes.ts) so it unit-tests
 * without loading WebGL. GlobeCanvas binds these to its MapLibre layers; the
 * scaling decisions live here, in one tunable place, instead of inline in the
 * canvas. Sizes were calibrated in the original react-globe.gl units; the
 * MapLibre port converts at the edge (zoomForAltitude / *_Px below) rather
 * than re-deriving the calibration.
 *
 * Two problems this solves on the shipped globe (PSY-1213):
 *   1. Dot/label size scaled by `sqrt(count)` UNCAPPED, so very dense scenes
 *      (Chicago ~283) ballooned over their neighbours.
 *   2. Every scene carried an always-on label, so adjacent dense cities
 *      (Minneapolis / St. Paul, ~10 mi apart) overlapped at the default zoom.
 */

// Pure great-circle helper (no three.js), so globeScale stays unit-testable
// without loading the globe. Reused by the PSY-1330 proximity declutter below.
import { haversineDistanceKm } from '@/lib/haversine'

// ── Dot radius ────────────────────────────────────────────────────────────
// sqrt scale (so dense scenes don't dwarf small ones) with the high end CAPPED
// so a 283-show scene doesn't render a giant dot that swallows its neighbours.
// `count` is upcoming_show_count; radius is in legacy globe-radius units —
// the canvas consumes the px conversion (sceneDotRadiusPx) below.
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
  // poison the rendered layer (historically the merged three.js point geometry;
  // today a NaN circle-radius feature in the MapLibre dots layer).
  const count = Number.isFinite(upcomingShowCount) ? Math.max(0, upcomingShowCount) : 0
  return DOT_BASE_RADIUS + Math.min(Math.sqrt(count) / DOT_SQRT_DIVISOR, DOT_VARIABLE_MAX)
}

// ── MapLibre zoom ⇄ legacy globe altitude (PSY-1538) ──────────────────────
// The label gate (labelMinCountForAltitude), the declutter calibration keyed on
// its thresholds, and the POV contract (GlobePov.altitude) were all calibrated
// in react-globe.gl camera-altitude units (globe radii above the surface). The
// MapLibre port keeps that calibration by converting between the two scales
// instead of re-deriving the bands.
//
// Derivation (ground span per screen height, the quantity labels care about):
//   react-globe.gl: fov 50° ⇒ span ≈ altitude · 2·tan(25°) · R_earth
//                                  ≈ altitude · 5941 km
//   MapLibre:       span ≈ H_px · C_equator · cos(lat) / (512 · 2^zoom)
// Calibrated at the surface the bands were tuned on — a ~900 px-tall desktop
// viewport centered on North America (lat ≈ 39°) — the two are equal when
//   2^zoom = 9.214 / altitude
// i.e. altitude 1.8 (default POV) ⇔ z≈2.36, 1.6 (geo POV) ⇔ z≈2.53,
// 1.5 (continental band edge) ⇔ z≈2.62, 1.0 (multi-region edge / fly-to
// landing) ⇔ z≈3.20, 0.6 (metro-cluster edge) ⇔ z≈3.94.
//
// The constant bakes in that calibration viewport on purpose: making it
// viewport/latitude-dependent would move the label bands per resize/pan and
// defeat the memoize-on-discrete-threshold churn avoidance (see the label-gate
// doc below). Approximate correspondence is all the bands need.
const ALTITUDE_AT_ZOOM_ZERO = 9.214

/** MapLibre zoom whose ground scale matches the legacy globe altitude. */
export function zoomForAltitude(altitude: number): number {
  // Guard: altitude ≤ 0 (or non-finite) has no zoom equivalent — clamp to the
  // smallest calibrated camera height rather than returning Infinity/NaN.
  const a = Number.isFinite(altitude) && altitude > 0 ? altitude : 0.1
  return Math.log2(ALTITUDE_AT_ZOOM_ZERO / a)
}

/** Inverse of zoomForAltitude — feeds labelMinCountForAltitude from map zoom. */
export function altitudeForZoom(zoom: number): number {
  const z = Number.isFinite(zoom) ? zoom : 0
  return ALTITUDE_AT_ZOOM_ZERO / 2 ** z
}

// ── Globe-unit → CSS-pixel scales (PSY-1538) ──────────────────────────────
// react-globe.gl sized dots/labels in angular globe units; MapLibre circle and
// DOM-marker labels take CSS pixels. One knob each, calibrated so the default
// continental POV renders the same visual weight as the shipped globe
// (0.28–0.5 unit dots ⇔ ~3.4–6 px radius; 0.5–0.85 unit labels ⇔ ~11–19 px
// text). The sqrt/cap COUNT curve is shared with the unit functions above,
// so it can't drift between layers.
//
// DELIBERATE semantic change vs the shipped globe: angular sizing grew
// on-screen as the camera descended; these px sizes are ZOOM-CONSTANT, so
// the calibration match is exact only at the default POV and dots read
// relatively smaller (but never blurrier or overlapping) at fly-to/metro
// zooms. Chosen because constant px stays legible at the deeper zooms this
// port unlocks (z→9, PSY-1539 street level next); if zoom growth is wanted
// later, interpolate the radius on ['zoom'] in the canvas — don't bend
// these knobs.
const DOT_PX_PER_RADIUS_UNIT = 12
const LABEL_PX_PER_SIZE_UNIT = 22

/** Dot radius in CSS px for the MapLibre circle layer. */
export function sceneDotRadiusPx(upcomingShowCount: number): number {
  return sceneDotRadius(upcomingShowCount) * DOT_PX_PER_RADIUS_UNIT
}

/** Label text size in CSS px for the DOM label markers. */
export function sceneLabelSizePx(upcomingShowCount: number): number {
  return sceneLabelSize(upcomingShowCount) * LABEL_PX_PER_SIZE_UNIT
}

// ── Dot stacking: smaller dots draw ABOVE larger ones (PSY-1324) ──────────
// Formerly sceneDotAltitude, a 3D cylinder-height trick against depth-tested
// three.js cylinders. In MapLibre the same invariant is a circle-sort-key:
// higher key renders on top, so the key must strictly DECREASE with count —
// Chicago's capped disc can never swallow Milwaukee's dot. Keyed to the RAW
// count, not the capped radius, so two co-dense capped metros still order
// (only an exact-count tie draws unordered — an equal-size pair, so neither
// dominates). GlobeCanvas uses the same key for hit-test tie-breaks, so the
// dot you SEE on top is the dot a click selects.
export function sceneDotSortKey(upcomingShowCount: number): number {
  // Non-finite guard — see sceneDotRadius; 0 sorts a malformed count with
  // count-0 scenes rather than poisoning the layer sort.
  return Number.isFinite(upcomingShowCount) ? -upcomingShowCount : 0
}

// The label gate composed for MapLibre: translate map zoom back to the legacy
// altitude the bands were calibrated in, then apply the gate. Lives here so
// the canvas never needs to know about the unit translation.
export function labelMinCountForZoom(zoom: number): number {
  return labelMinCountForAltitude(altitudeForZoom(zoom))
}

// The globe's screen radius in CSS px at a MapLibre zoom: the equator maps to
// worldSize = TILE_WORLD_SIZE_PX·2^zoom px of circumference. Same projection
// constant the ALTITUDE_AT_ZOOM_ZERO derivation above is calibrated against.
const TILE_WORLD_SIZE_PX = 512

export function globeScreenRadiusPx(zoom: number): number {
  const z = Number.isFinite(zoom) ? zoom : 0
  return (TILE_WORLD_SIZE_PX * 2 ** z) / (2 * Math.PI)
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
// Radius bump on hover — kept small so the highlighted dot never visibly
// pops over its neighbours (applied via a feature-state paint expression).
export const DOT_HOVER_RADIUS_SCALE = 1.2

// Precedence: selected > hovered > followed > base — the transient affordance
// states stay legible on top of the persistent "my scene" marking. `baseColor` is
// the scene's dominant-genre-family tint (PSY-1315) when it has one; it replaces
// the default orange as the resting color, but the interaction/follow affordances
// still win so hover/select/follow stay legible over any tint.
export function sceneDotColor(
  slug: string,
  hoveredSlug: string | null,
  selectedSlug: string | null,
  followedSlugs?: ReadonlySet<string> | null,
  baseColor?: string,
): string {
  if (selectedSlug !== null && slug === selectedSlug) return DOT_COLOR_SELECTED
  if (hoveredSlug !== null && slug === hoveredSlug) return DOT_COLOR_HOVERED
  if (followedSlugs != null && followedSlugs.has(slug)) return DOT_COLOR_FOLLOWED
  return baseColor ?? DOT_COLOR_BASE
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
// St. Paul (~95) does not — at continental zoom that pair is separated by the COUNT
// gate; the PSY-1330 proximity pass (below) handles them at the lower multi-region
// band, where BOTH clear the gate.
//
// Two limits surfaced in the PSY-1223 review, both now fixed. PSY-1229 handles
// the first with the top-K floor below: counts are SEASONAL, so a fixed absolute
// threshold could leave the continental view with ZERO labels in a quiet stretch.
// The second (the gate is by COUNT, not proximity, so two co-dense adjacent cities
// both above the threshold could still overlap — Minneapolis/St. Paul at the
// multi-region band) is fixed by PSY-1330: a great-circle proximity pass in
// visibleLabelScenes (see declutterByProximity). It is great-circle, NOT a
// per-frame screen projection, precisely to preserve the memoize-on-threshold
// churn-avoidance this comment describes.
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

// ── Proximity declutter (PSY-1330) ────────────────────────────────────────
// The count gate alone can't stop two co-dense ADJACENT cities that BOTH clear
// the threshold from rendering overlapping, unreadable labels — the demonstrated
// case is Minneapolis (~187) and St. Paul (~95), ~15 km apart, both above the
// multi-region 40 at camera altitude 1.0. After the count gate, drop the
// less-dense of any pair whose labels would collide (see declutterByProximity),
// keyed on GREAT-CIRCLE distance, not screen projection: this runs only when the
// discrete label threshold changes (visibleLabelScenes is memoized on minCount),
// so a per-frame projection pass would churn labelsData every frame and defeat
// the PSY-1213/1223 churn-avoidance design. A screen-projection declutter is more
// pixel-accurate but was ruled out for exactly that reason.
//
// The "too close" distance widens with altitude: a fixed km spans fewer screen
// pixels the further out the camera, so labels collide at larger real distances
// when zoomed out. Keyed on the same discrete minCount the label gate uses.
// CALIBRATION (PSY-1330 derived → PSY-1369 stage confirm, 2026-07-18):
// Anchored on Minneapolis/St. Paul ~15 km decluttering at the band-40 view.
// Stage visual pass at camera altitudes ~0.6 / 1.0 / 1.5 (bypass-gated
// stage.psychichomily.com/atlas): labels at every gated band were readable with
// no garbled overlaps. Closest multi-region pair on today's stage catalog is
// Chicago↔Milwaukee (~134 km) — correctly kept (outside the 30 km reach). St.
// Paul is not currently in the stage scenes registry, so the original MSP pair
// could not be re-measured; derived values remain adequate until a co-dense
// adjacent pair returns. Choose each value with MARGIN below the nearest real
// non-colliding metro pair — the distance check is inclusive (<=), so a pair
// exactly at the value is suppressed.
//
// Tunable here — this map is the single knob. Bands not listed get no declutter;
// note minCount <= 0 (the zoomed-in "label everything" view) is short-circuited in
// visibleLabelScenes BEFORE this lookup, so adding a 0 entry would have no effect.
// CONTRACT: every non-zero threshold labelMinCountForAltitude can return MUST have
// a key here, or that band ships with no declutter — the globeScale.test
// "covers every gated band" guard pins this so the two maps can't silently drift.
export const LABEL_DECLUTTER_KM_BY_MIN_COUNT: Readonly<Record<number, number>> = {
  120: 60, // continental — only the very densest scenes label; wide overlap reach
  40: 30, //  multi-region — catches Minneapolis/St. Paul (~15 km) with margin
  10: 15, //  metro cluster — tight; only near-coincident cities collide here
}

// The declutter radius (km) for a label band, or 0 (no declutter) for any band
// not in the calibration map. minCount is the DISCRETE threshold from
// labelMinCountForAltitude, not raw altitude.
export function labelDeclutterRadiusKm(minCount: number): number {
  return LABEL_DECLUTTER_KM_BY_MIN_COUNT[minCount] ?? 0
}

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
 * The globe's hover tooltip line for a scene.
 *
 * Lives here rather than inline in GlobeCanvas for the same reason VenuePin
 * carries a preformatted `nextShowLabel`: the canvas draws what it is handed
 * and does not decide how a count is worded. It also puts the copy somewhere a
 * unit test can reach — GlobeCanvas imports maplibre at module scope, so its
 * strings are otherwise only verifiable by reading them.
 *
 * The trailing segment counts `shows_this_week`, so it says "next 7 days" —
 * see `SceneListItem.shows_this_week` for why (PSY-1732). The segment is
 * dropped entirely at zero: a quiet scene shows its upcoming total without a
 * "0" that reads as dead.
 */
export function sceneTooltipLabel(scene: {
  city: string
  state: string
  upcoming_show_count: number
  shows_this_week: number
}): string {
  const week =
    scene.shows_this_week > 0 ? ` · ${scene.shows_this_week} next 7 days` : ''
  return `${scene.city}, ${scene.state} · ${scene.upcoming_show_count} upcoming${week}`
}

// The shape visibleLabelScenes / declutterByProximity need: a show count for the
// gate + ordering, and coords for the proximity pass. Coords are optional (the
// scenes API's latitude/longitude are `number | null`); a scene without them is
// simply never a declutter collision (it isn't plotted on the globe anyway).
type LabelScene = {
  upcoming_show_count: number
  latitude?: number | null
  longitude?: number | null
}

// A scene we can actually measure: finite latitude AND longitude. This excludes
// null/undefined (the API's missing-coords case, so the scene isn't plotted on the
// globe anyway) AND NaN/Infinity (malformed) in one guard — a non-measurable scene
// is never a proximity-collision candidate. Type predicate so callers narrow to
// number coords without a cast.
function hasFiniteCoords<T extends LabelScene>(
  s: T,
): s is T & { latitude: number; longitude: number } {
  return Number.isFinite(s.latitude) && Number.isFinite(s.longitude)
}

/**
 * Drop the less-dense of any pair of labelled scenes whose great-circle distance
 * is within `radiusKm`, so co-dense adjacent cities (Minneapolis/St. Paul) don't
 * render overlapping labels (PSY-1330). The DENSER of a colliding pair is kept
 * (compareScenesByActivity — the shared liveliest-first rule). A scene without
 * finite coordinates can't be measured, so it's always kept and never suppresses
 * another. The result preserves the INPUT order minus the suppressed scenes, so a
 * caller's ordering contract is unchanged. `radiusKm <= 0` (a band with no
 * calibrated distance) is a no-op. An exact-count tie between two co-located
 * scenes resolves by input (API) order — rare enough to accept; the shared
 * compareScenesByActivity owns the ordering rule. Pure and O(n·k) — O(n²) in the
 * worst case (nothing suppressed) — paid at band crossings, never per frame.
 */
function declutterByProximity<T extends LabelScene>(
  labelled: readonly T[],
  radiusKm: number,
): T[] {
  if (radiusKm <= 0 || labelled.length < 2) return labelled.slice()
  // Decide keeps densest-first so the denser scene always wins a collision...
  const byDensity = [...labelled].sort(compareScenesByActivity)
  const kept: T[] = []
  const suppressed = new Set<T>()
  for (const s of byDensity) {
    if (!hasFiniteCoords(s)) {
      kept.push(s) // unmeasurable coords → can't collide, can't suppress others
      continue
    }
    const collides = kept.some(
      (k) =>
        hasFiniteCoords(k) &&
        haversineDistanceKm(s.latitude, s.longitude, k.latitude, k.longitude) <=
          radiusKm,
    )
    if (collides) suppressed.add(s)
    else kept.push(s)
  }
  // ...but emit in the ORIGINAL order (only the suppressed are removed).
  return labelled.filter((s) => !suppressed.has(s))
}

/**
 * The subset of scenes that carry an always-on label at the given threshold
 * (from `labelMinCountForAltitude`). Scenes below it still render their dot and
 * hover tooltip — only the persistent label is withheld until you zoom in.
 *
 * Normally this is the threshold gate (`count >= minCount`) followed by a
 * proximity declutter (PSY-1330): co-dense adjacent cities that both clear the
 * gate would overlap, so the less-dense of any colliding pair is dropped. The
 * PSY-1229 floor adds one safety net: if the threshold clears NOTHING (a seasonal
 * dip where no city reaches the continental count), fall back to the
 * `LABEL_TOP_K_FLOOR` densest so the zoomed-out view is never empty. That floor is
 * deliberately NOT decluttered — its job is "never fewer than K labels", which a
 * proximity pass could undercut. The fallback fires ONLY on an empty gate result,
 * so a normal season is untouched — the calibrated continental label set is preserved.
 *
 * @param minCount the DISCRETE threshold from `labelMinCountForAltitude` (a step
 *   value, NOT raw altitude). The caller memoizes on it to keep the returned
 *   array's identity stable between threshold crossings — react-globe.gl diffs
 *   labelsData by reference, so passing raw altitude would churn it every frame.
 *   The declutter reads the same discrete value, so it too runs only on crossings.
 */
export function visibleLabelScenes<T extends LabelScene>(
  scenes: readonly T[],
  minCount: number,
): T[] {
  // Every path returns a fresh array, so the result is always safe to mutate.
  if (minCount <= 0) return scenes.slice()
  const radiusKm = labelDeclutterRadiusKm(minCount)
  // NaN counts fail `>= minCount` (NaN comparisons are always false), so a
  // malformed scene is naturally excluded from the gate.
  const qualifiers = scenes.filter((s) => s.upcoming_show_count >= minCount)
  if (qualifiers.length > 0) return declutterByProximity(qualifiers, radiusKm)
  // The threshold cleared NOTHING (a seasonal dip) — fall back to the
  // LABEL_TOP_K_FLOOR densest so the zoomed-out view is never empty. Firing only
  // on empty keeps a normal season untouched (no re-clutter). Non-finite counts
  // are excluded outright (never floored in), matching the size-scale guards. This
  // floor is NOT proximity-decluttered: its guarantee is "never fewer than K
  // labels", and decluttering a co-dense cluster here could drop it toward 1 —
  // overlapping labels are the acceptable tradeoff in this rare safety-net view.
  return scenes
    .filter((s) => Number.isFinite(s.upcoming_show_count))
    .sort(compareScenesByActivity)
    .slice(0, LABEL_TOP_K_FLOOR)
}
