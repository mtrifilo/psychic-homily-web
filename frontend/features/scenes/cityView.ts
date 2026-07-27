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
import type { Venue, VenueWithShowCount } from '@/features/venues/types'
import { LOCATION_UNKNOWN, formatLocation } from '@/lib/formatLocation'
import { resolveShowTimezone } from '@/lib/utils/formatters'
import type { PlaceableScene, VenuePin } from './components/globeTypes'
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

// ── Venue panel (PSY-1540) ────────────────────────────────────────────────
// Every "the mock" below means the approved board 02 "Venue panel" on the
// Atlas page of the Product Designs Figma file (node 1152:84) — same citation
// style as ArtistContextPanel. Numbers taken from it are marked as such so a
// later reader can tell a design constant from a guess.
//
// The panel's fixed width, from the mock. It floats over the map's RIGHT edge
// while the rail sits beside the map's left, so the two never fight for the
// same pixels — and neither goes near the bottom-left attribution control.
export const CITY_VENUE_PANEL_WIDTH_PX = 384

// How far the panel's bottom edge stays clear of the map frame, as a cap on
// its height rather than a fixed bottom anchor. The map's attribution control
// is docked bottom-LEFT and the ODbL makes it a licensing requirement to keep
// it visible (PSY-1543's adversarial review found a panel hiding it outright).
// A right-docked panel only reaches the credit when the map pane is narrow
// enough that 384px of panel spans into it — so rather than depend on the pane
// being wide, the panel simply cannot grow into the credit's strip. Same ~30px
// strip the "N more scenes" link clears with bottom-11.
export const CITY_VENUE_PANEL_BOTTOM_INSET_PX = 36

// How many show rows the panel lists before deferring to "view all N →". The
// mock draws five; past that the panel stops being a glance and the venue page
// is the better surface.
export const VENUE_PANEL_SHOW_ROWS = 5

// ── Artist drill-in (PSY-1541) ────────────────────────────────────────────
// How many upcoming shows the artist panel lists under NEXT SHOWS. The mock
// (board 03 "Artist drill-in", node 1154:6) draws two, and two is also the
// point past which the drill-in stops being a glance at "are they playing
// again" and starts duplicating the artist page's show table. Doubles as the
// `limit` on the artist-shows request, so the panel never fetches rows it
// won't draw.
export const ARTIST_PANEL_NEXT_SHOW_ROWS = 2

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

// ── Label declutter ───────────────────────────────────────────────────────
// The centroid fallback puts EVERY venue without a servable street geocode on
// the exact same point, so their name labels stamp on top of each other into
// an unreadable smear. Same problem the globe solved for adjacent metros, so
// same solution: drop the label of the less busy of any colliding pair, keyed
// on great-circle distance rather than screen projection (a per-frame
// projection pass would churn the marker set every frame — the reason
// globeScale's declutter is great-circle too).
//
// The radius is calibrated for the case that actually bites: coincident and
// near-coincident pins. It is deliberately NOT wide enough to thin out a dense
// but genuinely distinct downtown block — two venues 200 m apart are two marks
// worth naming, and a wider radius would silently hide real venues.
export const VENUE_LABEL_DECLUTTER_KM = 0.2

/** The subset of pins that keeps a name label. Busier venue wins a collision. */
export function labelledVenuePinIds(
  pins: readonly VenuePin[],
  radiusKm: number = VENUE_LABEL_DECLUTTER_KM,
): ReadonlySet<number> {
  const kept: VenuePin[] = []
  // Busiest first, so the venue a traveller most wants named is the one that
  // survives a pile-up at the centroid.
  const byActivity = [...pins].sort(
    (a, b) => b.upcomingShowCount - a.upcomingShowCount,
  )
  for (const pin of byActivity) {
    const collides = kept.some(
      (k) => haversineDistanceKm(pin.lat, pin.lng, k.lat, k.lng) <= radiusKm,
    )
    if (!collides) kept.push(pin)
  }
  return new Set(kept.map((p) => p.id))
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

// ── Venue panel copy (PSY-1540) ───────────────────────────────────────────

/**
 * "FRI 7/31" — the panel's mono date column, in the VENUE's timezone.
 *
 * A FOURTH show-date format is a real cost, so it needs a reason: the mock
 * (Product Designs → Atlas → board 02, node 1152:84) draws the panel's dates
 * as a fixed-width mono GUTTER beside the bill, not as prose. A gutter has to
 * be scannable at a glance and the same width on every row, which "Tue, Jul
 * 28" (`formatShowDate`, used in the venue page's table) and "Tue Jul 28"
 * (`formatNextShowDate`, used in the rail's running meta line) are not — both
 * vary in width with the month name and both read as sentence fragments. The
 * uppercase numeric form is the one that lines up. Kept out of
 * `formatters.ts` deliberately: it is Atlas panel chrome, not a general
 * show-date rendering the rest of the app should reach for.
 *
 * Unlike `formatNextShowDate` above, the input here is a full timestamp from
 * `GET /venues/{id}/shows` rather than a pre-resolved date string, so the zone
 * conversion has to happen here: a 9pm Friday show in Austin read in a
 * European viewer's zone is Saturday, and the panel is describing the venue's
 * calendar, not the reader's. `resolveShowTimezone` is the app-wide chain —
 * the venue's IANA zone when known, the US state map as the pre-backfill
 * fallback (PSY-985/986).
 *
 * Returns '' for a missing or unparseable timestamp so the caller renders an
 * empty column rather than "Invalid Date".
 */
export function formatPanelShowDate(
  eventDate: string | null | undefined,
  state: string | null | undefined,
  timezone: string | null | undefined,
): string {
  if (!eventDate) return ''
  const parsed = new Date(eventDate)
  if (Number.isNaN(parsed.getTime())) return ''
  return parsed
    .toLocaleString('en-US', {
      timeZone: resolveShowTimezone(state, timezone),
      weekday: 'short',
      month: 'numeric',
      day: 'numeric',
    })
    .replace(',', '')
    .toUpperCase()
}

/**
 * The panel's mono identity line, e.g. "1502 E 6th St · cap ~250 · Austin, TX".
 *
 * PRIVACY GATE (PSY-1536): `address` is one of the fields the API redacts for
 * unverified venues — `buildVenueResponse` serves it only when `verified` — so
 * an unverified venue arrives here with `address` already null and this line
 * simply omits that segment. This function must never reconstruct a street
 * address from another field for the same reason `venuePinPosition` must never
 * reconstruct a street coordinate: the redaction is the product decision, and
 * a panel that leaked what the map withholds would defeat it.
 *
 * Capacity is deliberately NOT gated — the backend serves it for unverified
 * venues too, on the stated grounds that a room size is not sensitive.
 *
 * The mock also draws a neighborhood ("East Austin") and an age policy
 * ("21+ varies"). Neither is in the venue model, and inventing them is exactly
 * the fabrication the rail's disabled "All ages" chip already refuses to do,
 * so both segments are absent rather than guessed.
 */
export function venuePanelIdentityLine(
  venue: Pick<Venue, 'address' | 'capacity' | 'city' | 'state'>,
): string {
  const parts: string[] = []
  const address = venue.address?.trim()
  if (address) parts.push(address)
  if (typeof venue.capacity === 'number' && venue.capacity > 0) {
    parts.push(`cap ~${venue.capacity}`)
  }
  // The shared PSY-558/780 rule, not a local `${city}, ${state}` template —
  // that bare template is precisely what PSY-780 consolidated away because it
  // left a trailing comma on a stateless venue. Its "Location Unknown"
  // placeholder is right for a location FIELD and wrong for one segment of a
  // run-on line, so an unplaceable venue drops the segment instead.
  const place = formatLocation(venue)
  if (place !== LOCATION_UNKNOWN) parts.push(place)
  return parts.join(' · ')
}

/**
 * How many upcoming shows to CLAIM the venue has.
 *
 * `total` is the endpoint's unfiltered `COUNT(*)` and `listed` is what
 * survived client-side de-duplication of the fetched page (the venue page's
 * shared `dedupVenueShows`). When rows exist beyond the page, `total` is the
 * only honest number available. When they don't, the deduped length is the
 * more honest one — `total` would double-count the duplicate rows the reader
 * can plainly see are gone.
 *
 * The "are there more rows" test is `total > fetched`, deliberately NOT
 * "did the page come back full". The two agree almost everywhere and diverge
 * exactly at the boundary where a venue has precisely the fetch limit's worth
 * of shows: a full-page heuristic calls that complete page truncated, and the
 * panel then claims the raw `total` over a list whose duplicates it had just
 * collapsed — "50 shows" above 48 rows, with no "view all" link to explain the
 * gap. `total` is a limit-independent COUNT(*), so ask it directly. This is
 * also why the fetch limit isn't a parameter: it carries no information the
 * total doesn't already carry better.
 */
export function venuePanelShowCount(args: {
  total: number | undefined
  listed: number
  fetched: number
}): number {
  const { total, listed, fetched } = args
  if (typeof total !== 'number') return listed
  const moreExistBeyondThePage = total > fetched
  return moreExistBeyondThePage && total > listed ? total : listed
}
