'use client'

/**
 * UpcomingShowsList (PSY-837, city filter PSY-840)
 *
 * Vertical list of upcoming-show rows for the /explore landing, with a
 * city filter (A1) so a traveler can pick a city and see what's on there
 * — the same shipped `CityFilters` component the /shows·/venues·/artists
 * lists use, wired here over the discovery teaser.
 *
 * Default city selection (resolution order):
 *   1. authed user with favorite cities → seeded from `favorite_cities`,
 *   2. anon / no favorites → IP-geo city (PSY-926), but ONLY when that city
 *      has upcoming shows in PH's data; surfaced as an overridable chip with
 *      a "(from your location) — change" affordance,
 *   3. geo unavailable / VPN / city-without-shows → "All cities"
 *      (chronological). NOT a hard Phoenix default.
 * User override always wins and persists via `?cities=` (PSY-840).
 *
 * The unfiltered list reads from the page-level SSR prefetch (seeded
 * cache) so first paint is synchronous; applying a city refetches.
 */

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useTransition,
} from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import Link from 'next/link'
import dynamic from 'next/dynamic'
import { useExploreUpcomingShows } from '../hooks'
import { formatShowDateBadge } from '@/lib/utils/showDateBadge'
import { cn } from '@/lib/utils'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/features/auth'
import { useShowCities } from '@/features/shows'
import { SaveDefaultsButton } from '@/components/filters/SaveDefaultsButton'
import {
  parseCitiesParam,
  buildCitiesParam,
  citiesEqual,
} from '@/components/filters/cityParams'
import {
  useGeoDefaultCity,
  shouldShowGeoAffordance,
} from '@/components/filters/useGeoDefaultCity'
import { GeoDefaultAffordance } from '@/components/filters/GeoDefaultAffordance'
import type { CityState, CityWithCount } from '@/components/filters/CityFilters'
import type { GeoLocation } from '@/lib/geo-default'

// CityFilters pulls in cmdk (Command) + Radix Popover. Those bytes are
// already in the global bundle (CommandPalette in AppShell), but
// MOUNTING the combobox eagerly on /explore hydrates that interactive
// tree on the critical path, which pushed the TTI gate (error-level,
// PSY-868) over budget. dynamic(ssr:false) defers the filter's hydration
// off the initial /explore hydration path; since the bar only appears
// after the useShowCities fetch resolves (cities.length > 0), the
// deferral is effectively invisible. See PSY-840.
const CityFilters = dynamic(
  () =>
    import('@/components/filters/CityFilters').then(m => ({
      default: m.CityFilters,
    })),
  {
    ssr: false,
    // Combobox-shaped placeholder — same border/padding/height as the
    // real trigger so the bar doesn't shift when it hydrates (CLS).
    loading: () => (
      <div className="flex w-fit items-center gap-2 rounded-md border border-border/50 bg-muted/50 px-3 py-1.5 text-sm text-muted-foreground">
        <span className="opacity-60">Filter by city…</span>
      </div>
    ),
  },
)

interface UpcomingShowsListProps {
  limit?: number
  /**
   * IP-geo soft default (PSY-926), resolved server-side from the Vercel edge
   * geo headers in `app/explore/page.tsx`. Carries `{city,state}` plus optional
   * visitor lat/long (PSY-981). A suggestion only — applied as the default
   * selection only for anon visitors with no `?cities=` and no favorites, AND
   * only when the city has upcoming shows (exact match, else the nearest
   * has-shows city by the visitor's coords). `null` → no geo default
   * (→ "All cities").
   */
  geoDefaultCity?: GeoLocation | null
}

export function UpcomingShowsList({
  limit = 5,
  geoDefaultCity = null,
}: UpcomingShowsListProps) {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated, isLoading: authLoading } = useAuthContext()
  const { data: profileData } = useProfile()
  const [, startTransition] = useTransition()
  const hasAppliedFavorites = useRef(false)

  const citiesParam = searchParams.get('cities')
  const selectedCities = useMemo(
    () => parseCitiesParam(citiesParam),
    [citiesParam],
  )

  const favoriteCities: CityState[] = useMemo(
    () => profileData?.user?.preferences?.favorite_cities ?? [],
    [profileData?.user?.preferences],
  )

  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const { data: citiesData } = useShowCities({ timezone })

  // Map ShowCity → CityWithCount; only render the filter when there's
  // at least one city to choose between (matches the list pages).
  const cities: CityWithCount[] = useMemo(
    () =>
      citiesData?.cities?.map(c => ({
        city: c.city,
        state: c.state,
        count: c.show_count,
        // Geocoded centroid (PSY-981) — drives the nearest-has-shows-city geo
        // default when the visitor's exact city has no shows.
        latitude: c.latitude,
        longitude: c.longitude,
      })) ?? [],
    [citiesData],
  )

  // Seed authed favorites onto the URL on first load (when no `?cities=`).
  // Geo seeding for anon visitors is delegated to the shared hook below;
  // favorites win because the hook stands down whenever favorites are present.
  useEffect(() => {
    if (hasAppliedFavorites.current || citiesParam) return
    if (authLoading) return
    if (favoriteCities.length === 0) return

    hasAppliedFavorites.current = true
    const params = new URLSearchParams()
    params.set('cities', buildCitiesParam(favoriteCities))
    startTransition(() => {
      router.replace(`/explore?${params.toString()}`, { scroll: false })
    })
  }, [favoriteCities, authLoading, citiesParam, router])

  // IP-geo soft default for anon visitors (PSY-926, shared hook PSY-946). The
  // hook reconciles the server-read `geoDefaultCity` against the has-shows
  // data and seeds the canonical city via router.replace. /explore reads geo
  // server-side, so no client fetch here (enableClientFetch defaults false).
  const onSeedGeo = useCallback(
    (city: CityState) => {
      const params = new URLSearchParams()
      params.set('cities', buildCitiesParam([city]))
      startTransition(() => {
        router.replace(`/explore?${params.toString()}`, { scroll: false })
      })
    },
    [router],
  )
  const { appliedGeoDefault, notifyUserInteracted } = useGeoDefaultCity({
    cities,
    isAuthenticated,
    authLoading,
    favoriteCities,
    hasExistingSelection: !!citiesParam,
    geoFromServer: geoDefaultCity,
    onSeed: onSeedGeo,
  })

  const { data, isLoading, isFetching, error } = useExploreUpcomingShows({
    limit,
    cities: selectedCities.length > 0 ? selectedCities : undefined,
  })

  const handleFilterChange = useCallback(
    (cities: CityState[]) => {
      // Any manual filter change is an override — the geo affordance must
      // not linger over a user-chosen selection.
      notifyUserInteracted()
      const params = new URLSearchParams()
      if (cities.length > 0) params.set('cities', buildCitiesParam(cities))
      const qs = params.toString()
      startTransition(() => {
        router.push(qs ? `/explore?${qs}` : '/explore', { scroll: false })
      })
    },
    [router, notifyUserInteracted],
  )

  // True when the geo default is still the active selection (exactly the one
  // detected city, unchanged by the user) — drives the "(from your location)"
  // chip.
  const showGeoAffordance = shouldShowGeoAffordance(
    appliedGeoDefault,
    selectedCities,
  )
  const selectionDiffersFromFavorites = !citiesEqual(
    selectedCities,
    favoriteCities,
  )

  const filterBar =
    // CLS (PSY-1091): always reserve the filter row's height (min-h-9 = the
    // combobox trigger / its dynamic-import placeholder height) so the shows
    // list below doesn't shift down when the cities fetch resolves and the bar
    // fills in. The filter controls render once ≥1 city has shows (PSY-932);
    // the container holds their space from first paint.
    cities.length > 0 ? (
      <div className="mb-5">
        <CityFilters
          cities={cities}
          selectedCities={selectedCities}
          onFilterChange={handleFilterChange}
        >
          {isAuthenticated && selectionDiffersFromFavorites && (
            <SaveDefaultsButton
              selectedCities={selectedCities}
              favoriteCities={favoriteCities}
            />
          )}
        </CityFilters>
        {showGeoAffordance && (
          <GeoDefaultAffordance
            city={appliedGeoDefault}
            onChange={() => handleFilterChange([])}
          />
        )}
      </div>
    ) : (
      // cities not yet resolved (client fetch) → reserve the row height.
      <div className="mb-5 min-h-9" aria-hidden />
    )

  if (isLoading) {
    return (
      <>
        {filterBar}
        <div className="flex justify-center items-center py-8">
          <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-foreground"></div>
        </div>
      </>
    )
  }

  if (error) {
    return (
      <>
        {filterBar}
        <div className="text-center py-8 text-muted-foreground">
          <p>Unable to load shows.</p>
        </div>
      </>
    )
  }

  if (!data || data.shows.length === 0) {
    const filtered = selectedCities.length > 0
    return (
      <>
        {filterBar}
        <div className="text-center py-8 text-muted-foreground">
          <p>
            {filtered
              ? `No upcoming shows in the selected ${
                  selectedCities.length === 1 ? 'city' : 'cities'
                }.`
              : 'No upcoming shows at this time.'}
          </p>
          {filtered && (
            <button
              onClick={() => handleFilterChange([])}
              className="mt-3 text-sm text-primary hover:underline underline-offset-4"
            >
              Show all cities
            </button>
          )}
        </div>
      </>
    )
  }

  return (
    <>
      {filterBar}
      <ul
        className={cn(
          'flex flex-col divide-y divide-border/40 rounded-lg border border-border/50 bg-card/30 transition-opacity',
          isFetching && 'opacity-60',
        )}
      >
        {data.shows.map(show => {
          const state = show.state ?? show.venue_state ?? null
          const cityLabel = show.city ?? show.venue_city ?? ''
          const dateBadge = formatShowDateBadge(show.event_date, state)
          const detailsHref = `/shows/${show.slug || show.id}`

          return (
            <li key={show.id}>
              <Link
                href={detailsHref}
                className="flex items-center gap-3 px-3 py-2.5 hover:bg-muted/40 transition-colors"
                aria-label={show.title}
              >
                <span className="text-xs text-muted-foreground shrink-0 w-20 tabular-nums">
                  {dateBadge.dayOfWeek} {dateBadge.monthDay}
                </span>
                <span className="font-medium text-sm flex-1 truncate">
                  {show.headliner_name || show.title}
                </span>
                <span className="text-xs text-muted-foreground shrink-0 hidden sm:inline truncate max-w-[40%]">
                  {show.venue_name}
                  {(cityLabel || state) && (
                    <span className="text-muted-foreground/70">
                      {' '}&middot; {[cityLabel, state].filter(Boolean).join(', ')}
                    </span>
                  )}
                </span>
              </Link>
            </li>
          )
        })}
      </ul>
    </>
  )
}
