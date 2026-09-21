'use client'

import Link from 'next/link'
// Concrete module paths, not the `@/features/shows` barrel — see the note in
// SavedShowsModule and features/sharedChunkBarrelGuard.test.ts.
import { HomeShowListView } from '@/features/shows/components/HomeShowListView'
import { useHomeShowCitySelection } from '@/features/shows/components/useHomeShowCitySelection'
import { buildCitiesParam, cityLabel } from '@/components/filters/cityParams'

/**
 * "Shows near you this week" — the signed-in home's discovery list (PSY-2103).
 *
 * Renders for EVERY signed-in viewer, saves or none, because the general
 * "Upcoming shows" section the anonymous page carries is gone for them: this is
 * how a show gets saved in the first place. Rows the viewer has already saved
 * are dropped, so this list and the saved module above never repeat a row.
 *
 * The header names the city the rows were actually fetched for: the selection
 * is owned here and handed to the list, so the two cannot drift. There may be
 * NO city — `useGeoDefaultCity` resolves an IP-geo default for anonymous
 * visitors only, so a signed-in viewer with no favorite cities has none — and
 * the header then claims none.
 */
export function NearbyShowsSection({ id }: { id: string }) {
  const selection = useHomeShowCitySelection()
  const { effectiveCities } = selection

  const cityNames = effectiveCities.map(cityLabel).join(', ')
  const allShowsHref =
    effectiveCities.length > 0
      ? `/shows?cities=${encodeURIComponent(buildCitiesParam(effectiveCities))}`
      : '/shows'
  // One city is the common case and the only one the approved copy names; a
  // multi-city selection links to all of them under the unqualified label
  // rather than picking one of the viewer's cities to speak for the rest.
  const allShowsLabel =
    effectiveCities.length === 1
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
            {cityNames ? `${cityNames} · ` : ''}tap ♡ to save
          </p>
        </div>
        <Link
          href={allShowsHref}
          className="text-sm font-medium text-primary transition-colors hover:underline underline-offset-4"
        >
          {allShowsLabel}
        </Link>
      </div>

      <HomeShowListView selection={selection} excludeSavedShows />
    </section>
  )
}
