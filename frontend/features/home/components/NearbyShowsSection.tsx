'use client'

import { useMemo } from 'react'
import Link from 'next/link'
// Concrete module paths, not the `@/features/shows` barrel — see the note in
// SavedShowsModule and features/sharedChunkBarrelGuard.test.ts.
import {
  HomeShowListView,
  type HomeShowExclusions,
} from '@/features/shows/components/HomeShowListView'
import { useHomeShowCitySelection } from '@/features/shows/hooks/useHomeShowCitySelection'
import {
  SAVED_SHOWS_HOME_READ_LIMIT,
  useSavedShows,
} from '@/features/shows/hooks/useSavedShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { buildCitiesParam, cityLabel } from '@/components/filters/cityParams'

/**
 * "Shows near you this week" — the signed-in home's discovery list (PSY-2103).
 *
 * Renders for EVERY signed-in viewer, saves or none, because the general
 * "Upcoming shows" section the anonymous page carries is gone for them: this is
 * how a show gets saved in the first place.
 *
 * The rows the saved module above is showing are excluded here, so the two
 * lists do not repeat one. The exclusion set is the same saved-shows read that
 * module renders — one request, shared through the query cache — and it is read
 * at the API's page size rather than the four rows painted, because the saved
 * list is ordered by date across ALL cities: reading only the four would
 * exclude nothing at all for a viewer whose soonest saves are elsewhere. A save
 * or unsave invalidates that query, which is what moves a row between the two
 * lists with no reload.
 *
 * The list does not paint until that read settles. That is a deliberate gate,
 * not a parallelism claim: a row shown and then removed is worse than a row
 * shown a beat later.
 *
 * The header names the city the rows were actually fetched for: the selection
 * is owned here and handed to the list, so the two cannot drift. This surface
 * asks the selection to resolve a city for its COPY (favorites, then IP-geo,
 * then the liveliest city with shows), because the approved header and link
 * both name one.
 */
export function NearbyShowsSection({ id }: { id: string }) {
  const { user, authStatus } = useAuthContext()
  const isAuthenticated = authStatus === 'authenticated'
  const selection = useHomeShowCitySelection({ resolveCityForCopy: true })
  const { effectiveCities } = selection

  // Same key as the saved-shows module's read, so this shares that one request
  // rather than making a second.
  const {
    data: savedShows,
    isPending: isSavedPending,
    error: savedError,
  } = useSavedShows({
    timeFilter: 'upcoming',
    limit: SAVED_SHOWS_HOME_READ_LIMIT,
    userId: user?.id,
    enabled: isAuthenticated,
  })

  const excludeShowIds: HomeShowExclusions = useMemo(() => {
    // A failed read is an answer for this purpose: exclude nothing rather than
    // hold the list at 'pending' forever, since a repeated row is recoverable
    // and a permanent spinner is not. The module above reports the failure.
    if (savedError) return []
    if (isAuthenticated && isSavedPending) return 'pending'
    return savedShows?.shows.map(show => show.id) ?? []
  }, [savedError, isAuthenticated, isSavedPending, savedShows?.shows])

  const cityCount = effectiveCities.length
  const cityNames = effectiveCities.map(cityLabel).join(', ')
  const allShowsHref =
    cityCount > 0
      ? `/shows?cities=${encodeURIComponent(buildCitiesParam(effectiveCities))}`
      : '/shows'
  // One city is the common case and the only one the approved copy names; a
  // multi-city selection links to all of them under the unqualified label
  // rather than picking one of the viewer's cities to speak for the rest.
  const allShowsLabel =
    cityCount === 1
      ? `All upcoming shows in ${cityNames} →`
      : 'All upcoming shows →'

  return (
    <section
      id={id}
      aria-labelledby="home-nearby-shows-heading"
      className="flex w-full flex-col gap-4"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <div className="min-w-0">
          <h2
            id="home-nearby-shows-heading"
            className="text-2xl font-semibold tracking-tight text-foreground"
          >
            Shows near you this week
          </h2>
          <p className="mt-0.5 text-sm text-muted-foreground">
            {cityCount > 0 ? `${cityNames} · ` : ''}tap ♡ to save
          </p>
        </div>
        <Link
          href={allShowsHref}
          className="text-sm font-medium text-primary transition-colors hover:underline underline-offset-4"
        >
          {allShowsLabel}
        </Link>
      </div>

      <HomeShowListView
        selection={selection}
        excludeShowIds={excludeShowIds}
        excludedLabel={cityNames}
      />
    </section>
  )
}
