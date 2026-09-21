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
  SAVED_SHOWS_COLLAPSED_COUNT,
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
 * lists never repeat one. The exclusion set comes from the same saved-shows
 * query that module runs — one request, shared through the query cache — and it
 * runs BESIDE the upcoming-shows fetch rather than after it, so neither list
 * waits on the other. A save or unsave invalidates that query, which is what
 * moves a row between the two lists with no reload.
 *
 * The header names the city the rows were actually fetched for: the selection
 * is owned here and handed to the list, so the two cannot drift. There may be
 * NO city — `useGeoDefaultCity` resolves an IP-geo default for anonymous
 * visitors only, so a signed-in viewer with no favorite cities has none — and
 * the header then claims none.
 */
export function NearbyShowsSection({ id }: { id: string }) {
  const { user, isAuthenticated } = useAuthContext()
  const selection = useHomeShowCitySelection()
  const { effectiveCities } = selection

  // Same key as the saved-shows module's read, so this shares that one request
  // rather than making a second.
  const { data: savedShows, isPending: isSavedPending } = useSavedShows({
    timeFilter: 'upcoming',
    limit: SAVED_SHOWS_COLLAPSED_COUNT,
    userId: user?.id,
    enabled: isAuthenticated,
  })

  const excludeShowIds: HomeShowExclusions = useMemo(
    () =>
      isAuthenticated && isSavedPending
        ? 'pending'
        : (savedShows?.shows.map(show => show.id) ?? []),
    [isAuthenticated, isSavedPending, savedShows?.shows]
  )

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

      <HomeShowListView selection={selection} excludeShowIds={excludeShowIds} />
    </section>
  )
}
