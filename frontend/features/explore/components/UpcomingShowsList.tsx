'use client'

/**
 * UpcomingShowsList (PSY-837, city filter PSY-840)
 *
 * Vertical list of upcoming-show rows for the /explore landing, with a
 * city filter (A1) so a traveler can pick a city and see what's on there
 * — the same shipped `CityFilters` component the /shows·/venues·/artists
 * lists use, wired here over the discovery teaser.
 *
 * Default city selection:
 *   - authed user with favorite cities → seeded from `favorite_cities`,
 *   - anon / no favorites → "All cities" (chronological).
 * The IP-geolocation smart default for anon visitors is the separate
 * PSY-926 follow-up; this ships the "All cities" baseline.
 *
 * The unfiltered list reads from the page-level SSR prefetch (seeded
 * cache) so first paint is synchronous; applying a city refetches.
 */

import { useCallback, useEffect, useMemo, useRef, useTransition } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { useExploreUpcomingShows } from '../hooks'
import { formatShowDateBadge } from '@/lib/utils/showDateBadge'
import { cn } from '@/lib/utils'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/features/auth'
import { useShowCities } from '@/features/shows'
import {
  CityFilters,
  SaveDefaultsButton,
  parseCitiesParam,
  buildCitiesParam,
  citiesEqual,
  type CityState,
  type CityWithCount,
} from '@/components/filters'

interface UpcomingShowsListProps {
  limit?: number
}

export function UpcomingShowsList({ limit = 5 }: UpcomingShowsListProps) {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated } = useAuthContext()
  const { data: profileData } = useProfile()
  const [, startTransition] = useTransition()
  const hasAppliedDefaults = useRef(false)

  const citiesParam = searchParams.get('cities')
  const selectedCities = useMemo(
    () => parseCitiesParam(citiesParam),
    [citiesParam],
  )

  const favoriteCities: CityState[] = useMemo(
    () => profileData?.user?.preferences?.favorite_cities ?? [],
    [profileData?.user?.preferences],
  )

  // Authed default: seed the filter from favorite cities on first load
  // when the URL carries none yet. Anon visitors get the "All cities"
  // baseline (no favorites). The IP-geo smart default is PSY-926.
  useEffect(() => {
    if (
      !hasAppliedDefaults.current &&
      favoriteCities.length > 0 &&
      !citiesParam
    ) {
      hasAppliedDefaults.current = true
      const params = new URLSearchParams()
      params.set('cities', buildCitiesParam(favoriteCities))
      startTransition(() => {
        router.replace(`/explore?${params.toString()}`, { scroll: false })
      })
    }
  }, [favoriteCities, citiesParam, router])

  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const { data: citiesData } = useShowCities({ timezone })

  const { data, isLoading, isFetching, error } = useExploreUpcomingShows({
    limit,
    cities: selectedCities.length > 0 ? selectedCities : undefined,
  })

  const handleFilterChange = useCallback(
    (cities: CityState[]) => {
      const params = new URLSearchParams()
      if (cities.length > 0) params.set('cities', buildCitiesParam(cities))
      const qs = params.toString()
      startTransition(() => {
        router.push(qs ? `/explore?${qs}` : '/explore', { scroll: false })
      })
    },
    [router],
  )

  // Map ShowCity → CityWithCount; only render the filter when there's
  // more than one city to choose between (matches the list pages).
  const cities: CityWithCount[] = useMemo(
    () =>
      citiesData?.cities?.map(c => ({
        city: c.city,
        state: c.state,
        count: c.show_count,
      })) ?? [],
    [citiesData],
  )
  const selectionDiffersFromFavorites = !citiesEqual(
    selectedCities,
    favoriteCities,
  )

  const filterBar =
    cities.length > 1 ? (
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
