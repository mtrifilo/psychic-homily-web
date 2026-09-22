'use client'

// Concrete module paths, not the `@/features/shows` barrel — see the note in
// SavedShowsModule and features/sharedChunkBarrelGuard.test.ts.
import { HomeShowListView } from '@/features/shows/components/HomeShowListView'
import { useHomeShowCitySelection } from '@/features/shows/hooks/useHomeShowCitySelection'
import { NEXT_7_DAYS } from '@/features/shows/quickWindows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { cityLabel } from '@/components/filters/cityParams'
import { HomeCityShowsLink, useHomeCityLinkSlot } from './HomeCityShowsLink'
import { HomeDiscoverLinks } from './HomeDiscoverLinks'

/** Rows the section paints, the count on the approved board. */
const NEARBY_ROWS = 4

/**
 * "Shows near you this week" — the signed-in home's discovery list (PSY-2103).
 *
 * Renders for EVERY signed-in viewer, saves or none, because the general
 * "Upcoming shows" section the anonymous page carries is gone for them: this is
 * how a show gets saved in the first place.
 *
 * The rows the saved module above is showing are left out here, so the two
 * lists do not repeat one. The list learns which rows are saved from the batch
 * save-count read it already makes for its hearts, so the exclusion is exact
 * for the page and costs no second request. A save or unsave invalidates that
 * batch and the module's read, which is what moves a row between the two lists
 * with no reload.
 *
 * "This week" is a claim the list has to keep: rows are held to the next seven
 * days. "Near you" is a claim it can only make when the city came from the
 * viewer (their pick, their favorites, or their location); when the city is
 * the last-resort guess, the header says so instead of asserting proximity.
 *
 * The header names the city the rows were actually fetched for: the selection
 * is owned here and handed to the list, so the two cannot drift. This surface
 * asks the selection to resolve a city for its COPY (favorites, then IP-geo,
 * then the liveliest city with shows), because the approved header and link
 * both name one.
 */
export function NearbyShowsSection({ id }: { id: string }) {
  const { authStatus } = useAuthContext()
  const isAuthenticated = authStatus === 'authenticated'
  // Every surface that can carry the city link reads the same predicate, so
  // "exactly one renders it" is enforced rather than coordinated. This section
  // owns it whenever it is visible, but it stays mounted while it collapses,
  // and by then ownership has already moved on.
  const ownsCityLink = useHomeCityLinkSlot() === 'nearby'
  const selection = useHomeShowCitySelection({ resolveCityForCopy: true })
  const { effectiveCities, source, isResolving } = selection
  // Geo is still deciding and nothing else claimed the city. The LIST does not
  // wait (an unfiltered page is a truthful thing to show, and narrowing it
  // when geo answers is not a correction, which is how the anonymous home
  // already behaves); only the COPY holds back the claim it cannot make yet.
  const isResolvingCity = isResolving && source === 'none'

  const cityCount = effectiveCities.length
  // Prose joins with the same separator the subline uses, so two cities do not
  // read as four; the link label below keeps the single-city form only.
  const cityNames = effectiveCities.map(cityLabel).join(' · ')
  const isGuessedCity = source === 'liveliest'
  const heading =
    isGuessedCity || isResolvingCity
      ? 'Shows this week'
      : 'Shows near you this week'
  const subline =
    isResolvingCity
      ? 'finding your city · tap ♡ to save'
      : cityCount === 0
        ? 'tap ♡ to save'
      : isGuessedCity
        ? `${cityNames} · the liveliest scene right now · tap ♡ to save`
        : `${cityNames} · tap ♡ to save`
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
            {heading}
          </h2>
          <p className="mt-0.5 text-sm text-muted-foreground">{subline}</p>
        </div>
        {ownsCityLink && <HomeCityShowsLink cities={effectiveCities} />}
      </div>

      <HomeShowListView
        selection={selection}
        rows={NEARBY_ROWS}
        excludeSaved={isAuthenticated}
        withinDays={NEXT_7_DAYS}
        excludedLabel={cityCount > 0 ? cityNames : undefined}
      />

      {/* The quiet acclimation row travels with this section so the five
          reorderable sections stay a contiguous run for PSY-2104. */}
      <HomeDiscoverLinks className="flex flex-wrap items-center gap-x-1.5 gap-y-1 pt-2 text-sm" />
    </section>
  )
}
