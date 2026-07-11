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

import { useCallback, useMemo, useTransition } from 'react'
import { useQueryState } from 'nuqs'
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
  citiesEqual,
  citiesParser,
  ALL_CITIES,
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
// Combobox-shaped placeholder — same border/padding/height as the real
// CityFilters trigger. Used BOTH as the dynamic-import loading fallback AND as
// the pre-cities-fetch reservation, so the filter row occupies an identical
// height across all three states (empty → placeholder → real) and the shows
// list below never shifts (CLS — PSY-1091).
function CityFilterPlaceholder() {
  return (
    <div className="flex w-fit items-center gap-2 rounded-md border border-border/50 bg-muted/50 px-3 py-1.5 text-sm text-muted-foreground">
      <span className="opacity-60">Filter by city…</span>
    </div>
  )
}

const CityFilters = dynamic(
  () =>
    import('@/components/filters/CityFilters').then(m => ({
      default: m.CityFilters,
    })),
  {
    ssr: false,
    // MOUNTING the combobox eagerly on /explore hydrates the cmdk + Radix
    // Popover tree on the critical path, which pushed the TTI gate over budget
    // (PSY-868). dynamic(ssr:false) defers that hydration; the placeholder
    // keeps the row height stable. See PSY-840.
    loading: () => <CityFilterPlaceholder />,
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
  const { isAuthenticated, isLoading: authLoading } = useAuthContext()
  const { data: profileData } = useProfile()
  const [, startTransition] = useTransition()

  const favoriteCities: CityState[] = useMemo(
    () => profileData?.user?.preferences?.favorite_cities ?? [],
    [profileData?.user?.preferences],
  )

  // `?cities=` via nuqs: null (absent → apply the default) | ALL_CITIES
  // (explicit all) | an explicit selection. Filter changes push a history
  // entry so the back button steps through them.
  const [citiesState, setCities] = useQueryState(
    'cities',
    citiesParser.withOptions({ history: 'push', startTransition }),
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

  // IP-geo soft default for anon visitors (PSY-926, shared hook PSY-946). The
  // hook reconciles the server-read `geoDefaultCity` against the has-shows
  // data and RETURNS the derived canonical city — folded into the derived
  // selection below, never written to the URL. /explore reads geo server-side,
  // so no client fetch here (enableClientFetch defaults false).
  const { appliedGeoDefault, notifyUserInteracted } = useGeoDefaultCity({
    cities,
    isAuthenticated,
    authLoading,
    favoriteCities,
    hasExistingSelection: citiesState !== null,
    geoFromServer: geoDefaultCity,
  })

  // The effective selection, DERIVED during render — a bare /explore resolves
  // to the authed user's favorites (or, for anon visitors, the geo default);
  // an explicit ?cities= wins. Deriving (instead of seeding the default into
  // the URL from a mount effect) is what makes the default survive
  // client-side navigation.
  const selectedCities: CityState[] = useMemo(() => {
    if (citiesState === ALL_CITIES) return []
    if (citiesState) return citiesState
    if (favoriteCities.length > 0) return favoriteCities
    return appliedGeoDefault ? [appliedGeoDefault] : []
  }, [citiesState, favoriteCities, appliedGeoDefault])

  const { data, isLoading, isFetching, error } = useExploreUpcomingShows({
    limit,
    cities: selectedCities.length > 0 ? selectedCities : undefined,
  })

  // City changes write `?cities=` via nuqs. An empty selection becomes the
  // explicit ALL_CITIES sentinel (?cities=all), NOT a bare URL — a bare URL
  // means "apply my default" (favorites/geo).
  const handleFilterChange = useCallback(
    (cities: CityState[]) => {
      // Any manual filter change is an override — the geo affordance must
      // not linger over a user-chosen selection.
      notifyUserInteracted()
      void setCities(cities.length > 0 ? cities : ALL_CITIES)
    },
    [setCities, notifyUserInteracted],
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
      // cities not yet resolved (client fetch) → reserve the exact filter-row
      // height with the same placeholder, so the shows list doesn't shift down
      // when the bar fills in.
      <div className="mb-5" aria-hidden>
        <CityFilterPlaceholder />
      </div>
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
