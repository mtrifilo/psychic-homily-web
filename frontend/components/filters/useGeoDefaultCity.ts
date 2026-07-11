'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import type { CityState, CityWithCount } from './CityFilters'
import type { GeoLocation } from '@/lib/geo-default'
import { GEO_CACHE_KEY, matchByGeo, toGeoLocation } from '@/lib/geo-client'
import { citiesEqual } from './cityParams'

/**
 * Shared IP-geo default-city hook (PSY-946).
 *
 * Extracted from /explore's `UpcomingShowsList` reconciliation logic (PSY-926)
 * so the SAME resolution-order, has-shows gate, canonical-city match, and
 * "from your location — change" affordance run on all three city-filter
 * surfaces: /explore, /shows, and home.
 *
 * The hook RETURNS the derived geo default; it never writes it anywhere (no
 * URL seeding, no setState into the caller). Callers fold the value into
 * their own render-derived selection (URL value ?? favorites ?? geo). The
 * only effect inside is the `/api/geo` fetch itself — a genuine
 * external-system sync.
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
 *   1. authed user with `favoriteCities` → favorites (caller's concern; pass
 *      `favoriteCities` so the hook stands down — it never overrides them),
 *   2. anon + geo city that HAS upcoming shows in PH's data → the CANONICAL
 *      city from `cities` (never the raw header — injection-safe),
 *   3. otherwise → no default ("All cities").
 *
 * The returned value is always the canonical PH `{city,state}` from `cities`,
 * so the selection it produces matches the backend filter exactly.
 */

/**
 * Shape of the `/api/geo` route-handler response. The cache key and the
 * `toGeoLocation` validator live in `@/lib/geo-client` so every client geo
 * consumer (this hook, the homepage scene-graph default) shares one cache.
 */
interface GeoApiResponse {
  geo: GeoLocation | null
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
}

interface UseGeoDefaultCityResult {
  /** The canonical has-shows geo city to use as the anon fallback default,
   *  DERIVED — the hook never writes it anywhere. Callers fold it into their
   *  own derived selection (URL value ?? favorites ?? this). Null whenever
   *  ineligible (authed / favorites present / existing selection / user has
   *  interacted / auth still resolving) or no has-shows match. Also drives the
   *  "Showing {City}, {ST} (from your location)" affordance. */
  appliedGeoDefault: CityState | null
  /** Call from the surface's filter-change / "change" handler. Flips the hook
   *  permanently ineligible so the affordance drops and a late-resolving
   *  `/api/geo` fetch can never surface a default over the user's choice —
   *  the old seed-effect race is gone structurally (nothing is written), but
   *  without this a slow fetch could still flash the derived default in. */
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
}: UseGeoDefaultCityParams): UseGeoDefaultCityResult {
  // Set once the user interacts with the filter. State (not a ref): the
  // derived default below must recompute — and the affordance drop — on the
  // very next render after the flip.
  const [userInteracted, setUserInteracted] = useState(false)

  // Eligibility: only anon visitors with no favorites, no existing selection,
  // and no prior interaction, once auth has settled. Gates BOTH the client
  // fetch (efficiency: an authed / favorited visitor never hits the edge) and
  // the derived default. Waiting on authLoading matters: deriving the anon
  // geo default while auth is still resolving could wrongly show geo for a
  // user who turns out to be authenticated (whose favorites should win).
  const eligible =
    !authLoading &&
    !isAuthenticated &&
    favoriteCities.length === 0 &&
    !hasExistingSelection &&
    !userInteracted

  const rawGeo = useGeoSource(geoFromServer, enableClientFetch, eligible)

  // The geo suggestion reconciled against PH's has-shows data via the shared
  // two-tier `matchByGeo` (exact city/state, else nearest has-shows city by
  // haversine — PSY-981; full contract on `matchByGeo`). Returns the CANONICAL
  // {city,state} from `cities` (so the value matches the backend filter
  // exactly) — never the raw header (injection-safe) — else null ("no
  // default").
  const geoCityWithShows: CityState | null = useMemo(() => {
    if (!rawGeo || cities.length === 0) return null
    const match = matchByGeo(cities, rawGeo, {
      city: c => c.city,
      state: c => c.state,
      lat: c => c.latitude,
      lng: c => c.longitude,
    })
    return match ? { city: match.city, state: match.state } : null
  }, [rawGeo, cities])

  // DERIVED, never written: the old seed effect (hasAppliedDefaults ref +
  // onSeed → router.replace / setState) reconstructed the default via a
  // side-effect, which raced auth/profile timing and client-side navigation.
  // Deriving it makes those races structurally impossible — ineligibility
  // (favorites arriving, a URL selection, user interaction) nulls the value
  // on the same render.
  const appliedGeoDefault = eligible ? geoCityWithShows : null

  const notifyUserInteracted = useCallback(() => setUserInteracted(true), [])

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
