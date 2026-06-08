/**
 * Great-circle distance between two lat/long points (PSY-981).
 *
 * Used to pick the geographically NEAREST has-shows city for a new visitor
 * whose exact city has no PH shows (e.g. Paradise Valley, AZ — a Phoenix
 * suburb): we compare the visitor's IP-derived coords (from Vercel's
 * x-vercel-ip-latitude/-longitude) against each show-city's geocoded centroid
 * (from `/shows/cities`) and seed the closest one — with NO distance cap (an
 * explicit product choice over a radius / same-state rule).
 *
 * Haversine is the standard, numerically stable spherical-distance formula. It
 * handles the antimeridian (e.g. -179° vs +179°) correctly because it works on
 * the sphere via the longitude DELTA's sine/cosine, not on raw degree
 * subtraction — so two points straddling ±180° come out close, as they should.
 * The Earth-as-sphere approximation is well within tolerance for "which metro
 * is closest" (sub-1% vs the ellipsoid at city scale); we only ever compare
 * distances, never report them, so the small absolute error is irrelevant.
 */

/** Mean Earth radius in kilometers (IUGG). Distances are relative-only here. */
const EARTH_RADIUS_KM = 6371

function toRadians(degrees: number): number {
  return (degrees * Math.PI) / 180
}

/**
 * Distance in kilometers between `(lat1, lon1)` and `(lat2, lon2)`, all in
 * decimal degrees. Returns `0` for identical points (the `asin` argument is
 * clamped, so floating-point noise can't push it past 1 and produce `NaN`).
 */
export function haversineDistanceKm(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number,
): number {
  const dLat = toRadians(lat2 - lat1)
  const dLon = toRadians(lon2 - lon1)
  const radLat1 = toRadians(lat1)
  const radLat2 = toRadians(lat2)

  const sinDLat = Math.sin(dLat / 2)
  const sinDLon = Math.sin(dLon / 2)
  const a =
    sinDLat * sinDLat + Math.cos(radLat1) * Math.cos(radLat2) * sinDLon * sinDLon
  // Clamp `a` to [0, 1] so `sqrt`/`asin` can't see a value > 1 from rounding
  // when the two points coincide (would otherwise yield NaN).
  const c = 2 * Math.asin(Math.sqrt(Math.min(1, a)))
  return EARTH_RADIUS_KM * c
}
