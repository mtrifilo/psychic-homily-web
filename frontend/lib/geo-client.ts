/**
 * Client-side geo primitives shared by every browser consumer of the
 * `/api/geo` edge route (PSY-946 city filter, PSY-1346 homepage graph default).
 *
 * These live here — not inside a specific feature hook — so all client geo
 * consumers agree on ONE sessionStorage cache key and ONE response validator.
 * A homepage renders more than one geo consumer at once (the shows city filter
 * and the scene-graph default); sharing the cache means the visit makes at most
 * one `/api/geo` request regardless of how many consumers mount.
 *
 * Client-safe by construction: unlike `lib/geo-default.ts` (which imports
 * `next/headers` for the server read path), nothing here touches server-only
 * APIs, so it's importable from any 'use client' module.
 */

import type { GeoLocation } from '@/lib/geo-default'
import { haversineDistanceKm } from '@/lib/haversine'

/** sessionStorage key for the cached `/api/geo` response (per-session reuse). */
export const GEO_CACHE_KEY = 'ph-geo-default-city'

/**
 * Narrow an arbitrary parsed value to a `GeoLocation` (two non-empty string
 * halves + optional finite coords), else null. Defensive: the route handler /
 * cache always emits this shape, but a malformed value (corrupted
 * sessionStorage, a future route-handler change) must not reach the consumers,
 * where `.trim()` on a non-string would throw mid-render or a non-finite coord
 * would corrupt a haversine pick.
 *
 * The coords are attached only when BOTH are finite numbers; a partial/garbage
 * pair is dropped so the nearest-by-distance path never sees a half-coordinate.
 */
export function toGeoLocation(value: unknown): GeoLocation | null {
  if (typeof value !== 'object' || value === null) return null
  const { city, state, latitude, longitude } = value as Record<string, unknown>
  if (typeof city !== 'string' || typeof state !== 'string') return null
  if (city.trim() === '' || state.trim() === '') return null
  if (
    typeof latitude === 'number' &&
    Number.isFinite(latitude) &&
    typeof longitude === 'number' &&
    Number.isFinite(longitude)
  ) {
    return { city, state, latitude, longitude }
  }
  return { city, state }
}

/** Field accessors letting `matchByGeo` work over any item shape. */
interface GeoAccessors<T> {
  city: (item: T) => string
  state: (item: T) => string
  lat: (item: T) => number | null | undefined
  lng: (item: T) => number | null | undefined
}

/**
 * Reconcile a geo suggestion against a list of geo-bearing items, returning the
 * best-matching item or null. Two-tier resolution (PSY-981), shared by the
 * shows city filter (`useGeoDefaultCity`) and the homepage scene-graph default
 * (`pickDefaultScene`) so both agree on what "the visitor's place" means:
 *
 *   1. Exact city/state match (case/whitespace-insensitive — the Vercel header
 *      spelling can differ slightly from stored casing). Preferred so a visitor
 *      whose own place is in the list always lands on it.
 *   2. Nearest item by haversine, with NO distance cap — the fallback when the
 *      exact place isn't in the list (e.g. a suburb). Requires the visitor's
 *      coords AND at least one item with a geocoded centroid; items the
 *      geocoder couldn't place (null/undefined coords) are skipped as distance
 *      candidates but remain eligible for the exact tier. Returns null when
 *      coords are missing on either side — the "no default" outcome.
 */
export function matchByGeo<T>(
  items: readonly T[],
  geo: GeoLocation,
  get: GeoAccessors<T>,
): T | null {
  const norm = (s: string) => s.trim().toLowerCase()
  const wantCity = norm(geo.city)
  const wantState = norm(geo.state)
  const exact = items.find(
    item => norm(get.city(item)) === wantCity && norm(get.state(item)) === wantState,
  )
  if (exact) return exact

  const { latitude, longitude } = geo
  if (latitude == null || longitude == null) return null

  let nearest: T | null = null
  let nearestKm = Infinity
  for (const item of items) {
    const lat = get.lat(item)
    const lng = get.lng(item)
    if (lat == null || lng == null) continue
    const km = haversineDistanceKm(latitude, longitude, lat, lng)
    if (km < nearestKm) {
      nearestKm = km
      nearest = item
    }
  }
  return nearest
}
