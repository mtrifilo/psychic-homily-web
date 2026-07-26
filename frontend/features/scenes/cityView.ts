/**
 * Pure logic for the Atlas city view (PSY-1539) — which city the camera is
 * looking at, where each venue pins, and what the rail rows say.
 *
 * Kept free of React and MapLibre (like globeScale.ts) so the rules that
 * decide "is this venue street-mapped or centroid-mapped" and "does this venue
 * survive the filters" unit-test without WebGL. GlobeCanvas and VenueRail bind
 * these; the decisions live here, in one tunable place.
 */

import { haversineDistanceKm } from '@/lib/haversine'
import type { VenueWithShowCount } from '@/features/venues/types'
import type { PlaceableScene } from './components/globeTypes'
import { GENRE_FAMILIES, type GenreFamily } from './genreFamilies'

// ── Engagement thresholds ─────────────────────────────────────────────────
// City view is the STREET half of the Atlas: it engages once the camera is
// close enough that individual venues, not metros, are the unit of interest.
//
// CITY_VIEW_MIN_ZOOM sits well above PSY-1543's globe→street crossfade (done
// by z7) so the rail never appears while the Black Marble raster is still
// dissolving. At z11 a desktop map pane spans roughly 70 km of ground at
// mid-latitudes — one metro and its suburbs — which is what makes the claim
// radius below a sane "the camera is looking AT this city" test.
export const CITY_VIEW_MIN_ZOOM = 11

// How far the camera center may sit from a scene's centroid and still be
// "looking at" it. Deliberately close to the z11 viewport span: beyond it the
// camera is over open country between metros and no city should claim the
// rail. Also comfortably larger than the ~3 km center shift the rail itself
// causes when it opens and narrows the map pane, so opening the rail can
// never hand the camera to a DIFFERENT city (a feedback loop).
export const CITY_VIEW_CLAIM_RADIUS_KM = 60

// The rail's fixed width, from the mock. Exported because GlobeCanvas must
// shrink the map pane by exactly this much — the rail sits BESIDE the map,
// never over it, so it cannot cover the map's bottom-left attribution
// control, which OpenStreetMap's ODbL makes a licensing requirement
// (PSY-1543's adversarial review found exactly that regression).
export const CITY_RAIL_WIDTH_PX = 360

// Below this viewport width the rail would leave a uselessly narrow map, so
// city view stays map-only (pins + status chip, no rail). The <640px case
// never reaches here at all — AtlasGlobe swaps in MobileSceneList.
export const CITY_VIEW_MIN_VIEWPORT_PX = 900

// One page of venues is enough for a city rail; the endpoint caps limit at 100.
export const CITY_VENUE_FETCH_LIMIT = 100

// ── Which city owns the camera ────────────────────────────────────────────

export interface CameraPosition {
  lng: number
  lat: number
  zoom: number
}

/**
 * The scene the camera is looking at, or null when it isn't looking at one.
 *
 * Null below CITY_VIEW_MIN_ZOOM (the globe view owns the screen) and null when
 * the nearest scene is further than CITY_VIEW_CLAIM_RADIUS_KM (the camera is
 * between metros). Pure and cheap — the caller runs it on camera SETTLE, never
 * per frame.
 */
export function resolveCityScene(
  scenes: readonly PlaceableScene[],
  camera: CameraPosition,
): PlaceableScene | null {
  if (!Number.isFinite(camera.zoom) || camera.zoom < CITY_VIEW_MIN_ZOOM) {
    return null
  }
  let best: PlaceableScene | null = null
  let bestKm = Infinity
  for (const scene of scenes) {
    const km = haversineDistanceKm(
      camera.lat,
      camera.lng,
      scene.latitude,
      scene.longitude,
    )
    if (km < bestKm) {
      best = scene
      bestKm = km
    }
  }
  return bestKm <= CITY_VIEW_CLAIM_RADIUS_KM ? best : null
}

// ── Where a venue pins ────────────────────────────────────────────────────

/** Which coordinate source a venue's pin came from. */
export type VenuePinPrecision = 'street' | 'centroid'

export interface VenuePinPosition {
  lng: number
  lat: number
  precision: VenuePinPrecision
}

/**
 * A venue's map position, and how precise it is.
 *
 * PRIVACY GATE (PSY-1536, a locked user decision): street coordinates exist
 * only for verified venues whose geocode still matches their current address.
 * The API enforces that — it omits street_latitude/street_longitude for
 * everyone else — and this function's ONLY job is to honor the omission by
 * falling back to the venue's city centroid. It must never reconstruct a
 * street position from any other field (address, zipcode), because that would
 * street-map the DIY and house venues the gate exists to protect.
 *
 * Returns null when the venue has no usable coordinates at all; such a venue
 * still lists in the rail, it just doesn't pin.
 */
export function venuePinPosition(
  venue: Pick<
    VenueWithShowCount,
    'latitude' | 'longitude' | 'street_latitude' | 'street_longitude'
  >,
): VenuePinPosition | null {
  if (
    Number.isFinite(venue.street_latitude) &&
    Number.isFinite(venue.street_longitude)
  ) {
    return {
      lat: venue.street_latitude as number,
      lng: venue.street_longitude as number,
      precision: 'street',
    }
  }
  if (Number.isFinite(venue.latitude) && Number.isFinite(venue.longitude)) {
    return {
      lat: venue.latitude as number,
      lng: venue.longitude as number,
      precision: 'centroid',
    }
  }
  return null
}

// ── Pin size ──────────────────────────────────────────────────────────────
// Same shape as the globe's dot scale (sqrt, capped) for the same reason: a
// 40-show venue must read as busier than a 4-show one without ballooning over
// its neighbours on a street map, where venues sit blocks apart. Bigger than
// the globe dots in absolute px because a city map has far fewer marks
// competing for the frame. Retune HERE, not inline in the canvas.
export const VENUE_PIN_BASE_RADIUS_PX = 5
export const VENUE_PIN_VARIABLE_MAX_PX = 6
// Every venue at or above this count draws the same max pin.
export const VENUE_PIN_CAP_COUNT = 20

/** Pin radius in CSS px for a venue's upcoming-show count. */
export function venuePinRadiusPx(upcomingShowCount: number): number {
  // Non-finite guard, matching sceneDotRadius: a NaN radius poisons the layer.
  const count = Number.isFinite(upcomingShowCount)
    ? Math.max(0, upcomingShowCount)
    : 0
  const variable =
    (Math.sqrt(count) / Math.sqrt(VENUE_PIN_CAP_COUNT)) *
    VENUE_PIN_VARIABLE_MAX_PX
  return (
    VENUE_PIN_BASE_RADIUS_PX + Math.min(variable, VENUE_PIN_VARIABLE_MAX_PX)
  )
}

// ── Filters ───────────────────────────────────────────────────────────────

export interface CityVenueFilters {
  /** "This week" chip: only venues with a show in the next 7 days. */
  thisWeekOnly: boolean
  /** Genre-family key from the "All genres" chip, or null for all. */
  genreFamily: string | null
}

export const NO_CITY_VENUE_FILTERS: CityVenueFilters = {
  thisWeekOnly: false,
  genreFamily: null,
}

/**
 * The venues that survive the rail's filter chips. ONE function, because the
 * rail rows and the map pins must narrow together — they render the same
 * array, so they cannot disagree.
 */
export function filterCityVenues(
  venues: readonly VenueWithShowCount[],
  filters: CityVenueFilters,
): VenueWithShowCount[] {
  return venues.filter((v) => {
    if (filters.thisWeekOnly && (v.shows_this_week ?? 0) <= 0) return false
    if (filters.genreFamily && v.dominant_genre !== filters.genreFamily) {
      return false
    }
    return true
  })
}

/**
 * The genre families actually represented in this city, in the canonical
 * legend order — so the "All genres" menu offers only families the user can
 * actually select their way to a non-empty list with.
 */
export function cityGenreFamilies(
  venues: readonly VenueWithShowCount[],
): GenreFamily[] {
  const present = new Set(
    venues.map((v) => v.dominant_genre).filter((key): key is string => !!key),
  )
  return GENRE_FAMILIES.filter((f) => present.has(f.key))
}

// ── Rail header stats ─────────────────────────────────────────────────────

export interface CityRailStats {
  venueCount: number
  upcomingCount: number
  thisWeekCount: number
}

/**
 * The header's counts, derived from the SAME venue rows the rail lists below
 * it. Deliberately not the scene's own venue/show counts: those are
 * metro-wide (a Tempe venue counts toward the Phoenix scene) while this list
 * is city-scoped, so a header sourced from the scene would contradict the
 * rows underneath it. "Local artists" is genuinely a scene-level stat and
 * comes from the scene detail endpoint instead.
 */
export function cityRailStats(
  venues: readonly VenueWithShowCount[],
): CityRailStats {
  let upcomingCount = 0
  let thisWeekCount = 0
  for (const v of venues) {
    upcomingCount += v.upcoming_show_count ?? 0
    thisWeekCount += v.shows_this_week ?? 0
  }
  return { venueCount: venues.length, upcomingCount, thisWeekCount }
}

/**
 * The most recent `updated_at` across the listed venues — the honest half of
 * the provenance footer ("DATA updated …"). Null when the list is empty.
 */
export function cityDataUpdatedAt(
  venues: readonly VenueWithShowCount[],
): string | null {
  let newest: string | null = null
  let newestMs = -Infinity
  for (const v of venues) {
    const ms = Date.parse(v.updated_at)
    if (Number.isFinite(ms) && ms > newestMs) {
      newest = v.updated_at
      newestMs = ms
    }
  }
  return newest
}

// ── Row copy ──────────────────────────────────────────────────────────────

/**
 * The bill line for a venue's next show: its title when it has one, else the
 * artists joined — the app-wide title-or-bill fallback (most shows carry no
 * title at all). Empty string when the venue has neither.
 */
export function nextShowBill(venue: VenueWithShowCount): string {
  if (venue.next_show_title) return venue.next_show_title
  return (venue.next_show_artists ?? []).join(' / ')
}

/**
 * "Tue Jul 28" from the API's `YYYY-MM-DD`.
 *
 * The parts are split and fed to a LOCAL Date on purpose. `new Date('2026-07-28')`
 * parses as UTC midnight, which renders as Jul 27 anywhere west of Greenwich —
 * and the backend already resolved this date in the VENUE's timezone, so any
 * further zone math here would corrupt a value that is already correct.
 * Returns '' for a missing or malformed date.
 */
export function formatNextShowDate(isoDate: string | undefined | null): string {
  if (!isoDate) return ''
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(isoDate)
  if (!match) return ''
  const [, year, month, day] = match
  const date = new Date(Number(year), Number(month) - 1, Number(day))
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  })
}
