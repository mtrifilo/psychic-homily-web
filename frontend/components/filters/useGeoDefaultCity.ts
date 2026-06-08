'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import type { CityState, CityWithCount } from './CityFilters'
import type { GeoLocation } from '@/lib/geo-default'
import { haversineDistanceKm } from '@/lib/haversine'
import { citiesEqual } from './cityParams'

/**
 * Shared IP-geo default-city hook (PSY-946).
 *
 * Extracted from /explore's `UpcomingShowsList` reconciliation logic (PSY-926)
 * so the SAME resolution-order, has-shows gate, canonical-city match, and
 * "from your location — change" affordance run on all three city-filter
 * surfaces: /explore, /shows, and home. The seeding MECHANISM differs per
 * surface (URL `?cities=` vs local state), so the hook decides WHAT to seed
 * and the caller wires its own `onSeed`.
 *
 * Two geo SOURCES, one hook (see the two-read-paths note in
 * `lib/geo-default.ts`):
 *   - /explore passes `geoFromServer` (already read server-side via
 *     `next/headers` at its dynamic boundary) and sets `enableClientFetch:
 *     false` → zero extra client requests there.
 *   - /shows + home set `enableClientFetch: true` (and no `geoFromServer`) →
 *     the hook fetches the `/api/geo` edge route handler on mount, cached in
 *     sessionStorage so cross-page navigation doesn't re-fetch.
 *
 * Resolution order (mirrors PSY-926, unchanged):
 *   1. authed user with `favoriteCities` → seed favorites (caller's concern;
 *      pass `favoriteCities` so the hook stands down — it never overrides
 *      favorites),
 *   2. anon + geo city that HAS upcoming shows in PH's data → seed the
 *      CANONICAL city from `cities` (never the raw header — injection-safe),
 *   3. otherwise → no default ("All cities").
 *
 * The seeded value is always the canonical PH `{city,state}` from `cities`, so
 * the `?cities=` it produces matches the backend filter exactly.
 */

/** sessionStorage key for the cached `/api/geo` response (per-session reuse). */
const GEO_CACHE_KEY = 'ph-geo-default-city'

/** Shape of the `/api/geo` route-handler response. */
interface GeoApiResponse {
  geo: GeoLocation | null
}

/**
 * Narrow an arbitrary parsed value to a `GeoLocation` (two non-empty string
 * halves + optional finite coords), else null. Defensive: the only writer of
 * the cache / the route handler always emits this shape, but a malformed value
 * (corrupted sessionStorage, a future route-handler change) must not reach
 * `geoCityWithShows`, where `.trim()` on a non-string would throw mid-render or
 * a non-finite coord would corrupt the haversine pick.
 *
 * The coords are attached only when BOTH are finite numbers; a partial/garbage
 * pair is dropped so the nearest-city path never sees a half-coordinate.
 */
function toGeoLocation(value: unknown): GeoLocation | null {
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

interface UseGeoDefaultCityParams {
  /** Cities that currently have shows (from `useShowCities`); the has-shows gate. */
  cities: CityWithCount[]
  /** Whether the visitor is authenticated. Geo applies to anon visitors only. */
  isAuthenticated: boolean
  /** Whether auth is still resolving. The hook waits — seeding geo while auth
   *  is in-flight could wrongly seed a user whose favorites should win. */
  authLoading: boolean
  /** The authed user's favorite cities. Non-empty → the hook stands down
   *  entirely (favorites win; the caller seeds them). */
  favoriteCities: CityState[]
  /** True when a selection is already present that geo must NOT override —
   *  e.g. /shows has a `?cities=` URL param, or the caller already seeded
   *  favorites. When true the hook never seeds and shows no affordance. */
  hasExistingSelection: boolean
  /** The geo value read server-side (/explore) — `{city,state}` plus optional
   *  visitor lat/long (PSY-981). When provided, the client fetch is skipped.
   *  `undefined` (not passed) → use the client fetch. */
  geoFromServer?: GeoLocation | null
  /** Fetch `/api/geo` client-side (/shows + home). Mutually exclusive in
   *  practice with `geoFromServer`. */
  enableClientFetch?: boolean
  /** Called once with the canonical city the hook decides to seed. The caller
   *  wires its surface's mechanism (router.replace for URL surfaces,
   *  setSelectedCities for state surfaces). */
  onSeed: (city: CityState) => void
}

interface UseGeoDefaultCityResult {
  /** The geo city currently seeded (and not yet overridden by the user), or
   *  null. Drives the "Showing {City}, {ST} (from your location)" affordance.
   *  Null whenever the selection isn't the geo default. */
  appliedGeoDefault: CityState | null
  /** Call from the surface's filter-change / "change" handler so the
   *  affordance never lingers over a user-chosen selection. Also permanently
   *  blocks a still-in-flight geo fetch from seeding AFTER the user has acted
   *  (the client-fetch surfaces resolve geo async, so this race is real;
   *  /explore's server prop resolves before mount, so it's a no-op there). */
  notifyUserInteracted: () => void
}

/**
 * Resolve the raw geo suggestion (`{city,state} | null`) for the client-fetch
 * surfaces, caching the `/api/geo` response in sessionStorage so cross-page
 * navigation within a session doesn't re-hit the edge.
 *
 * Returns `geoFromServer` verbatim when the caller already has a server-read
 * value (/explore) — no fetch, no cache.
 */
function useGeoSource(
  geoFromServer: GeoLocation | null | undefined,
  enableClientFetch: boolean,
  eligible: boolean,
): GeoLocation | null {
  // Seed synchronously from sessionStorage so a cached value is available on
  // first render (no flash, no redundant fetch). Server render + first
  // hydration return null (sessionStorage is client-only) — the value arrives
  // post-mount, same beat as today's authed-favorites seeding.
  const [fetched, setFetched] = useState<GeoLocation | null>(null)
  const hasFetched = useRef(false)

  useEffect(() => {
    if (!enableClientFetch || geoFromServer !== undefined) return
    // Only hit the edge route when the visitor could actually use the result
    // (anon, no favorites, auth settled, no existing selection). This is the
    // efficiency gate: an authed / favorited visitor NEVER triggers the geo
    // request. We re-check on each render until eligible, so a fetch fires the
    // moment auth settles to "anon, no favorites".
    if (!eligible) return
    if (hasFetched.current) return
    hasFetched.current = true

    // sessionStorage cache: a prior page in this session already resolved geo.
    // React 19.2: a synchronous setState in the effect body trips
    // set-state-in-effect (cascading render). The cache read itself is sync,
    // but applying it is deferred to a microtask so the state update lands
    // after the effect returns — same render-timing as the fetch path below.
    try {
      const cached = window.sessionStorage.getItem(GEO_CACHE_KEY)
      if (cached !== null) {
        const parsed = JSON.parse(cached) as GeoApiResponse
        const cachedCity = toGeoLocation(parsed?.geo)
        let cacheCancelled = false
        Promise.resolve().then(() => {
          if (!cacheCancelled) setFetched(cachedCity)
        })
        return () => {
          cacheCancelled = true
        }
      }
    } catch {
      // Corrupted cache / sessionStorage unavailable — fall through to fetch.
    }

    let cancelled = false
    fetch('/api/geo')
      .then(res => (res.ok ? (res.json() as Promise<GeoApiResponse>) : null))
      .then(body => {
        if (cancelled) return
        const geo = toGeoLocation(body?.geo)
        setFetched(geo)
        try {
          window.sessionStorage.setItem(GEO_CACHE_KEY, JSON.stringify({ geo }))
        } catch {
          // sessionStorage unavailable (private mode / quota) — degrade to
          // re-fetching on the next page; the geo default still works here.
        }
      })
      .catch(error => {
        if (cancelled) return
        // A geo-default failure is non-critical (the filter just defaults to
        // "All cities"), but capture it so a broken edge route is visible.
        Sentry.captureException(error, {
          level: 'warning',
          tags: { service: 'geo-default-city' },
        })
      })

    return () => {
      cancelled = true
    }
  }, [enableClientFetch, geoFromServer, eligible])

  // Server-prop path wins when provided; otherwise the client-fetched value.
  return geoFromServer !== undefined ? (geoFromServer ?? null) : fetched
}

export function useGeoDefaultCity({
  cities,
  isAuthenticated,
  authLoading,
  favoriteCities,
  hasExistingSelection,
  geoFromServer,
  enableClientFetch = false,
  onSeed,
}: UseGeoDefaultCityParams): UseGeoDefaultCityResult {
  const hasAppliedDefaults = useRef(false)
  // Set once the user interacts with the filter. Permanently blocks seeding —
  // guards the async race where the `/api/geo` fetch resolves AFTER the user
  // has already chosen (or cleared) a city on the client-fetch surfaces.
  const userInteracted = useRef(false)

  // Eligibility for the geo lookup: only anon visitors with no favorites, no
  // existing selection, and no prior interaction, once auth has settled. Gates
  // BOTH the client fetch (efficiency: an authed / favorited visitor never
  // hits the edge) and the seed effect below.
  //
  // Reading `userInteracted.current` during render is intentional and safe
  // here: the ONLY writer is `notifyUserInteracted`, which also calls
  // `setAppliedGeoDefault(null)`. That state update forces a re-render, so the
  // recomputed `eligible` (and the gated fetch) always observe the flip — the
  // ref never goes stale relative to render. It's a ref, not state, so the
  // async `/api/geo` resolution can't re-seed over a user choice that landed
  // first (see the seed effect's `userInteracted.current` guard below).
  //
  // The disable spans down through `useGeoSource` because the rule taints
  // every render value derived from the ref (here `eligible`), so the gated
  // hook call re-reports without it.
  /* eslint-disable react-hooks/refs */
  const eligible =
    !authLoading &&
    !isAuthenticated &&
    favoriteCities.length === 0 &&
    !hasExistingSelection &&
    !userInteracted.current

  const rawGeo = useGeoSource(geoFromServer, enableClientFetch, eligible)
  /* eslint-enable react-hooks/refs */
  // Tracks that the CURRENT selection was auto-applied from the IP-geo
  // suggestion (vs. a user / favorites / URL choice). Drives the affordance;
  // cleared the moment the user touches the filter.
  const [appliedGeoDefault, setAppliedGeoDefault] = useState<CityState | null>(
    null,
  )

  // The geo suggestion reconciled against PH's has-shows data: returns the
  // CANONICAL {city,state} from `cities` (so the seed matches the backend
  // filter exactly), else null. Two-tier resolution (PSY-981):
  //
  //   1. Exact (city, state) match — preferred when the visitor's own city
  //      has shows. Case/whitespace-insensitive (Vercel's spelling may differ
  //      slightly from PH's stored name). This is the pre-PSY-981 behavior,
  //      kept first so a visitor IN a show city always seeds their own city.
  //   2. NEAREST has-shows city by haversine, with NO distance cap (PSY-981) —
  //      the fallback when the exact city has no shows (e.g. Paradise Valley,
  //      AZ — a Phoenix suburb -> Phoenix). Requires the visitor's coords (from
  //      Vercel) AND at least one show-city with a geocoded centroid; when
  //      either is missing we return null, the pre-PSY-981 "no default"
  //      outcome — never worse than before.
  //
  // Either tier seeds the canonical PH casing from `cities`, never the raw
  // header (injection-safe).
  const geoCityWithShows: CityState | null = useMemo(() => {
    if (!rawGeo || cities.length === 0) return null

    // Tier 1: exact city-name match.
    const norm = (s: string) => s.trim().toLowerCase()
    const wantCity = norm(rawGeo.city)
    const wantState = norm(rawGeo.state)
    const exact = cities.find(
      c => norm(c.city) === wantCity && norm(c.state) === wantState,
    )
    if (exact) return { city: exact.city, state: exact.state }

    // Tier 2: nearest has-shows city by haversine (no cap). Only when the
    // visitor's coords are known; otherwise fall back to "no default".
    const { latitude, longitude } = rawGeo
    if (latitude === undefined || longitude === undefined) return null

    let nearest: CityWithCount | null = null
    let nearestKm = Infinity
    for (const c of cities) {
      // Skip cities the geocoder couldn't place — they can't be distance
      // candidates, but exact-match (tier 1) for them still works for a
      // visitor whose own city is that uncoded city.
      if (c.latitude === undefined || c.longitude === undefined) continue
      const km = haversineDistanceKm(latitude, longitude, c.latitude, c.longitude)
      if (km < nearestKm) {
        nearestKm = km
        nearest = c
      }
    }
    return nearest ? { city: nearest.city, state: nearest.state } : null
  }, [rawGeo, cities])

  // Seed once when: no existing selection, auth settled, anon, favorites empty,
  // and the geo city has shows. Guarded by hasAppliedDefaults so it runs once.
  useEffect(() => {
    if (hasAppliedDefaults.current) return
    // The user already acted (possibly before the async geo fetch resolved) —
    // never seed over their intent.
    if (userInteracted.current) return
    if (hasExistingSelection) return
    // Wait for auth to settle — seeding the anon geo default while auth is
    // still resolving could wrongly seed geo for a user who turns out to be
    // authenticated (whose favorites should win, or who gets "All cities").
    if (authLoading) return
    // Favorites win (caller seeds them) and authed-no-favorites gets no geo.
    if (isAuthenticated || favoriteCities.length > 0) return
    if (!geoCityWithShows) return

    hasAppliedDefaults.current = true
    setAppliedGeoDefault(geoCityWithShows)
    onSeed(geoCityWithShows)
  }, [
    hasExistingSelection,
    authLoading,
    isAuthenticated,
    favoriteCities,
    geoCityWithShows,
    onSeed,
  ])

  const notifyUserInteracted = useCallback(() => {
    userInteracted.current = true
    // Also block a later seed pass and drop the affordance immediately.
    hasAppliedDefaults.current = true
    setAppliedGeoDefault(null)
  }, [])

  return { appliedGeoDefault, notifyUserInteracted }
}

/**
 * True when the geo default is still the active selection (exactly the one
 * detected city, unchanged by the user) — drives whether the surface renders
 * the "(from your location) — change" chip. Extracted so all three surfaces
 * gate the chip identically.
 */
export function shouldShowGeoAffordance(
  appliedGeoDefault: CityState | null,
  selectedCities: CityState[],
): appliedGeoDefault is CityState {
  return (
    appliedGeoDefault !== null &&
    selectedCities.length === 1 &&
    citiesEqual(selectedCities, [appliedGeoDefault])
  )
}
