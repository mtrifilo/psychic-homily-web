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
  useState,
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
import type { CityState, CityWithCount } from '@/components/filters/CityFilters'

// CityFilters pulls in cmdk (Command) + Radix Popover. Those bytes are
// already in the global bundle (CommandPalette in SidebarLayout), but
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
   * geo headers in `app/explore/page.tsx`. A suggestion only — applied as the
   * default selection only for anon visitors with no `?cities=` and no
   * favorites, AND only when the city has upcoming shows. `null` → no geo
   * default (→ "All cities").
   */
  geoDefaultCity?: CityState | null
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
  const hasAppliedDefaults = useRef(false)
  // Tracks that the CURRENT selection was auto-applied from the IP-geo
  // suggestion (vs. a user/favorites/URL choice). Drives the "(from your
  // location) — change" affordance. Cleared the moment the user touches the
  // filter, so the affordance never lingers over a manual selection.
  const [appliedGeoDefault, setAppliedGeoDefault] = useState<CityState | null>(
    null,
  )

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
      })) ?? [],
    [citiesData],
  )

  // The IP-geo suggestion reconciled against PH's has-shows data: returns the
  // CANONICAL `{city, state}` from the cities list (so the seeded `?cities=`
  // matches the backend filter exactly) when the detected city has upcoming
  // shows, else null. Match is case/whitespace-insensitive because Vercel's
  // city spelling may differ slightly from PH's stored name.
  const geoCityWithShows: CityState | null = useMemo(() => {
    if (!geoDefaultCity || cities.length === 0) return null
    const norm = (s: string) => s.trim().toLowerCase()
    const wantCity = norm(geoDefaultCity.city)
    const wantState = norm(geoDefaultCity.state)
    const match = cities.find(
      c => norm(c.city) === wantCity && norm(c.state) === wantState,
    )
    return match ? { city: match.city, state: match.state } : null
  }, [geoDefaultCity, cities])

  // Default selection on first load when the URL carries no `?cities=`:
  //   1. authed + favorite cities → seed from favorites (unchanged),
  //   2. anon + geo city that has shows → seed from geo (PSY-926),
  //   3. otherwise → leave empty ("All cities").
  // Runs once (guarded by hasAppliedDefaults) and only after the dependent
  // data (favorites / cities-with-shows) has had a chance to load.
  useEffect(() => {
    if (hasAppliedDefaults.current || citiesParam) return
    // Wait for auth to settle. Applying the anon geo default while auth is
    // still resolving could wrongly seed geo for a user who turns out to be
    // authenticated (whose favorites should win, or who gets "All cities").
    if (authLoading) return

    let seed: CityState[] | null = null
    let fromGeo = false
    if (favoriteCities.length > 0) {
      seed = favoriteCities
    } else if (!isAuthenticated && geoCityWithShows) {
      seed = [geoCityWithShows]
      fromGeo = true
    }
    if (!seed) return

    hasAppliedDefaults.current = true
    if (fromGeo) setAppliedGeoDefault(geoCityWithShows)
    const params = new URLSearchParams()
    params.set('cities', buildCitiesParam(seed))
    startTransition(() => {
      router.replace(`/explore?${params.toString()}`, { scroll: false })
    })
  }, [
    favoriteCities,
    geoCityWithShows,
    isAuthenticated,
    authLoading,
    citiesParam,
    router,
  ])

  const { data, isLoading, isFetching, error } = useExploreUpcomingShows({
    limit,
    cities: selectedCities.length > 0 ? selectedCities : undefined,
  })

  const handleFilterChange = useCallback(
    (cities: CityState[]) => {
      // Any manual filter change is an override — the geo affordance must
      // not linger over a user-chosen selection.
      setAppliedGeoDefault(null)
      const params = new URLSearchParams()
      if (cities.length > 0) params.set('cities', buildCitiesParam(cities))
      const qs = params.toString()
      startTransition(() => {
        router.push(qs ? `/explore?${qs}` : '/explore', { scroll: false })
      })
    },
    [router],
  )

  // True when the geo default is still the active selection (exactly the one
  // detected city, unchanged by the user) — drives the "(from your location)"
  // chip.
  const showGeoAffordance =
    appliedGeoDefault !== null &&
    selectedCities.length === 1 &&
    citiesEqual(selectedCities, [appliedGeoDefault])
  const selectionDiffersFromFavorites = !citiesEqual(
    selectedCities,
    favoriteCities,
  )

  const filterBar =
    // Show the filter whenever ≥1 city has shows (PSY-932) — consistent with
    // /venues and /artists; hidden only when there are no cities.
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
        {showGeoAffordance && appliedGeoDefault && (
          <p
            className="mt-1.5 text-xs text-muted-foreground"
            data-testid="geo-default-affordance"
          >
            Showing {appliedGeoDefault.city}, {appliedGeoDefault.state} (from
            your location) —{' '}
            <button
              type="button"
              onClick={() => handleFilterChange([])}
              className="text-primary hover:underline underline-offset-4"
              data-testid="geo-default-change"
            >
              change
            </button>
          </p>
        )}
      </div>
    ) : null

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
