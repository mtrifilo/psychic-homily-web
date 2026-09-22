'use client'

import type { AuthStatus } from '@/lib/context/AuthContext'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import type { CityState, CityWithCount } from './CityFilters'
import type { GeoLocation } from '@/lib/geo-default'
import { GEO_CACHE_KEY, matchByGeo, toGeoLocation } from '@/lib/geo-client'
import { citiesEqual } from './cityParams'

/**
 * Shared IP-geo default-city hook.
 *
 * The hook RETURNS the derived geo default; it never writes it anywhere (no URL
 * seeding, no setState into the caller). Callers fold the value into their own
 * render-derived selection (URL value ?? favorites ?? geo). The only effect
 * inside is the `/api/geo` fetch itself, a genuine external-system sync.
 *
 * Two geo SOURCES, one hook (see the two-read-paths note in
 * `lib/geo-default.ts`):
 *   - a caller on an already-dynamic route passes `geoFromServer` (read
 *     server-side via `next/headers`) and sets `enableClientFetch: false`, so
 *     that route makes no extra client request.
 *   - a caller on an ISR or static route sets `enableClientFetch: true` (and no
 *     `geoFromServer`), and the hook fetches the `/api/geo` route handler on
 *     mount, cached in sessionStorage so cross-page navigation does not
 *     re-fetch.
 *
 * Resolution order:
 *   1. an authed user with `favoriteCities` wins (the caller's concern; pass
 *      `favoriteCities` so the hook stands down and never overrides them),
 *   2. anon + a geo city present in `cities` -> the CANONICAL entry from
 *      `cities`, never the raw header, which is what makes it injection-safe,
 *   3. otherwise no default.
 *
 * `cities` is whatever the calling surface counts: the returned value is always
 * an entry from it, so the selection it produces matches that surface's own
 * filter exactly.
 */

/**
 * Shape of the `/api/geo` route-handler response. The cache key and the
 * `toGeoLocation` validator live in `@/lib/geo-client` so every client geo
 * consumer (this hook, the homepage scene-graph default) shares one cache.
 */
interface GeoApiResponse {
  geo: GeoLocation | null
}

/**
 * How long the `/api/geo` read may take before it counts as answering "no
 * city". Generous enough for a cold edge invocation, short enough that a
 * caller rendering a loading state on `isResolving` is not left in it.
 */
const GEO_FETCH_TIMEOUT_MS = 5_000

interface UseGeoDefaultCityParams {
  /** Cities that currently have shows (from `useShowCities`); the has-shows gate. */
  cities: CityWithCount[]
  /**
   * The settled-auth signal. Geo applies to a SETTLED anonymous visitor only.
   *
   * `authStatus`, not an `isAuthenticated` / `isLoading` pair: `isLoading` is
   * false both before the profile fetch starts and after it fails without
   * answering, and in either window a signed-in viewer reads as anonymous with
   * no favourites, so the hook would seed the IP-geo city over favourites that
   * simply have not arrived. See AuthStatus in lib/context/AuthContext.
   */
  authStatus: AuthStatus
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
  /**
   * Also resolve a geo default for a SETTLED AUTHENTICATED viewer who has no
   * favorite cities (PSY-2103's signed-in home, which names the resolved city
   * in its copy and so must have one).
   *
   * Off by default, which keeps /shows and /explore on the anonymous-only rule.
   * Safe where it is on: the gate below still requires a settled viewer, and
   * `favoriteCities.length === 0` on a SETTLED authenticated viewer means they
   * have none, not that theirs have yet to arrive — which is the case the
   * anonymous-only gate exists to prevent.
   */
  allowAuthenticated?: boolean
}

interface UseGeoDefaultCityResult {
  /** The canonical geo city from `cities` to use as the anon fallback default,
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
  /**
   * True while `appliedGeoDefault`'s null is "not yet known" rather than "no
   * default": auth has not settled, or an eligible visitor's `/api/geo` read
   * is still in flight.
   *
   * It separates the two nulls for a surface whose CONTENT depends on the
   * derived city rather than merely defaulting a filter. Such a surface must
   * render a loading state while this is true; treating the pending null as
   * "no city" would flash a no-city state at every visitor who has one. A
   * surface that merely defaults a FILTER has no use for it: an unfiltered list
   * is a truthful thing to show for the window, and narrowing it afterwards is
   * not a correction.
   */
  isResolving: boolean
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
): { geo: GeoLocation | null; settled: boolean } {
  // Seed synchronously from sessionStorage so a cached value is available on
  // first render (no flash, no redundant fetch). Server render + first
  // hydration return null (sessionStorage is client-only) — the value arrives
  // post-mount, same beat as today's authed-favorites seeding.
  const [fetched, setFetched] = useState<GeoLocation | null>(null)
  // Whether the read has ANSWERED, which is not the same as having produced a
  // city: a cache hit, a successful fetch and a failed one all settle, and two
  // of the three can settle on null.
  const [settled, setSettled] = useState(false)
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
          // `settled` is recorded whether or not this run was cleaned up: the
          // re-entry latch below is a ref that survives the cleanup, so a run
          // that bailed here would leave nothing to settle it.
          setSettled(true)
          if (cacheCancelled) return
          setFetched(cachedCity)
        })
        return () => {
          cacheCancelled = true
        }
      }
    } catch {
      // Corrupted cache / sessionStorage unavailable — fall through to fetch.
    }

    let cancelled = false
    // A DEADLINE, not just an error path. `fetch` rejects on a network error and
    // not on a connection that hangs, and a caller gating its content on
    // `isResolving` would wait on that hang for the life of the page.
    fetch('/api/geo', { signal: AbortSignal.timeout(GEO_FETCH_TIMEOUT_MS) })
      .then(res => (res.ok ? (res.json() as Promise<GeoApiResponse>) : null))
      .then(body => {
        setSettled(true)
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
        // A failed read is an answer for the purposes of the caller's loading
        // state: there is no city coming.
        setSettled(true)
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
  return geoFromServer !== undefined
    ? { geo: geoFromServer ?? null, settled: true }
    : { geo: fetched, settled }
}

export function useGeoDefaultCity({
  cities,
  authStatus,
  favoriteCities,
  hasExistingSelection,
  geoFromServer,
  enableClientFetch = false,
  allowAuthenticated = false,
}: UseGeoDefaultCityParams): UseGeoDefaultCityResult {
  // Set once the user interacts with the filter. State (not a ref): the
  // derived default below must recompute — and the affordance drop — on the
  // very next render after the flip.
  const [userInteracted, setUserInteracted] = useState(false)

  // Eligibility: a SETTLED visitor with no favorites, no existing selection and
  // no prior interaction — anonymous always, authenticated only where the
  // caller opted in (see allowAuthenticated). Gates BOTH the client fetch
  // (efficiency: an authed / favorited visitor never hits the edge) and the
  // derived default. Waiting for the settle matters: deriving the anon geo
  // default while the viewer's identity is unknown shows geo to someone whose
  // favorites should have won.
  const eligible =
    (authStatus === 'anonymous' ||
      (allowAuthenticated && authStatus === 'authenticated')) &&
    favoriteCities.length === 0 &&
    !hasExistingSelection &&
    !userInteracted

  const { geo: rawGeo, settled: geoSettled } = useGeoSource(
    geoFromServer,
    enableClientFetch,
    eligible,
  )

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

  // Pending covers both ways the derivation can still be unknown: the viewer's
  // identity is unresolved (favourites may yet win), or an eligible anonymous
  // visitor's geo read has not answered. An ineligible settled visitor is not
  // pending: their null is final.
  const isResolving =
    authStatus === 'pending' ||
    (eligible && enableClientFetch && geoFromServer === undefined && !geoSettled)

  return { appliedGeoDefault, notifyUserInteracted, isResolving }
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
