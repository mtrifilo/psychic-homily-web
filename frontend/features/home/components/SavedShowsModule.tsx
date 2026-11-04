'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
// Concrete module paths, not the feature barrels: Turbopack does not
// tree-shake a `'use client'` barrel per export, so importing one here would
// ship the whole shows surface into the home chunk. See
// features/sharedChunkBarrelGuard.test.ts.
import { SAVED_SHOW_ROW_GRID } from '@/features/shows/components/SavedShowRow'
import {
  ShowRow,
  ShowRowSaveButton,
} from '@/features/shows/components/ShowRow'
import {
  SAVED_SHOWS_COLLAPSED_COUNT,
  useSavedShows,
  useShowSaveCountBatch,
} from '@/features/shows/hooks/useSavedShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { cn } from '@/lib/utils'
import { HOME_ACCENT_LINK_CLASS, ResolvedHomeCityShowsLink } from './HomeCityShowsLink'
import { SignedInHomeDiscoverLinks } from './HomeDiscoverLinks'
import { useHomeCityLinkSlot, useHomeDiscoverLinksSlot } from './HomeLinkSlots'

/**
 * What the module paints this render.
 *
 * One discriminant rather than a predicate re-tested per section: the subline,
 * the body and the footer all describe the same viewer, and three independent
 * conditions is how they start disagreeing.
 *
 * Every state here is TERMINAL except 'loading', and 'loading' is reachable
 * only from a read that is genuinely still in flight. A settled-anonymous
 * viewer is not a loading viewer: the module returns null instead, because the
 * alternative — an `enabled: false` query reporting `isPending` forever — is an
 * indefinite skeleton under a heading addressing someone who has signed out.
 */
type ModuleState = 'loading' | 'error' | 'empty' | 'rows'

/**
 * The signed-in home's lead module (PSY-2103): the viewer's next saved shows,
 * in place of the logged-out wordmark hero.
 *
 * It closes the save loop — save a show, come back, see it — which previously
 * had its second half only at /library, two clicks behind the avatar.
 *
 * Every row carries a SaveButton in its SAVED state: an unsave here is the
 * same gesture that made the save, and its invalidation already refreshes
 * both this list and Library's. `is_saved` is
 * asserted rather than read back, because these rows ARE the viewer's saved
 * shows; waiting on the save-count batch to learn that would paint an empty
 * heart labelled "Save show" on every row of a saved-shows list, and a click in
 * that window would fire a redundant save instead of the unsave the viewer
 * asked for. The batch still supplies the public count for the accessible name.
 *
 * `total` (not `shows.length`) drives the subline and the zero state: the read
 * is capped, so the row count says nothing about how many saves exist.
 */
export function SavedShowsModule() {
  const { user, authStatus } = useAuthContext()
  const isAuthenticated = authStatus === 'authenticated'
  // Takes the city link and the Discover row only when the nearby section that
  // normally carries them is hidden. The footer is where they go, next to the
  // other lines that point off this module.
  const ownsCityLink = useHomeCityLinkSlot() === 'saved'
  const ownsDiscoverLinks = useHomeDiscoverLinksSlot() === 'saved'

  // Exactly the rows painted. The server prefetches this same key for the
  // first paint (app/_components/HomeContentSlot.tsx), so the key must stay
  // byte-identical to the one that prefetch builds.
  const { data, isPending, error } = useSavedShows({
    timeFilter: 'upcoming',
    limit: SAVED_SHOWS_COLLAPSED_COUNT,
    userId: user?.id,
    enabled: isAuthenticated,
  })

  const shows = useMemo(
    () => (data?.shows ?? []).slice(0, SAVED_SHOWS_COLLAPSED_COUNT),
    [data?.shows]
  )
  const total = data?.total ?? 0

  const showIds = useMemo(() => shows.map(show => show.id), [shows])
  const { data: saveCounts } = useShowSaveCountBatch(
    showIds,
    isAuthenticated,
    user?.id
  )

  // A failed REFETCH keeps the rows it already has: TanStack retains `data`
  // alongside `error`, and un-answering a payload the viewer has seen is worse
  // than a stale one. Only a read that never answered is an error state.
  const state: ModuleState =
    error && !data
      ? 'error'
      : authStatus === 'pending' || isPending
        ? 'loading'
        : total === 0
          ? 'empty'
          : 'rows'
  const isStale = state === 'rows' && !!error
  // A calendar feed is worth offering only to a viewer with saves to put in it.
  const showsFooter = state === 'rows'

  // The server picked this variant from the viewer's cookie; a viewer who signs
  // out without navigating leaves it mounted. Say nothing rather than address
  // someone who is no longer there.
  if (authStatus === 'anonymous') return null

  return (
    <section
      aria-labelledby="signed-in-home-heading"
      className="flex w-full flex-col gap-4 pt-2"
    >
      {/* No call to action here: a viewer with saves, or whose saves failed
          to load, is pointed at Library; only the zero state offers "Find a
          show", under its prompt row. */}
      <div className="min-w-0">
        {/* h2, not h1: this section is hideable, and the page's h1 lives on
            the layout shell so the document outline survives hiding it. */}
        <h2
          id="signed-in-home-heading"
          className="text-3xl font-semibold tracking-tight text-foreground sm:text-4xl"
        >
          Your upcoming shows
        </h2>
        {(state === 'rows' || state === 'error') && (
          <div className="mt-1.5 flex items-baseline justify-between gap-x-4">
            {state === 'rows' && (
              <p className="min-w-0 text-sm text-muted-foreground">
                {total} saved · soonest first
              </p>
            )}
            <Link
              href="/library"
              className={cn(HOME_ACCENT_LINK_CLASS, 'ml-auto shrink-0')}
            >
              {state === 'rows' ? (
                <>
                  All {total}{' '}
                  <span className="sr-only">
                    {total === 1 ? 'saved show' : 'saved shows'} in Library
                  </span>{' '}
                  →
                </>
              ) : (
                // No count: a read that failed has none to give.
                'Library →'
              )}
            </Link>
          </div>
        )}
        {state === 'empty' && (
          <p className="mt-1.5 text-sm text-muted-foreground">
            Nothing saved yet · tap ♡ on any show and it lands here
          </p>
        )}
        {state === 'loading' && (
          // Reserves the subline's line box so the settled copy does not
          // push the rows down when it arrives.
          <Skeleton className="mt-2 h-4 w-64" aria-hidden />
        )}
      </div>

      {state === 'error' && (
        <p className="py-4 text-sm text-muted-foreground">
          Unable to load your saved shows.
        </p>
      )}

      {state === 'loading' && (
        // ONE row, not the four the read may return: the prompt row is the
        // guaranteed minimum, so reserving the maximum would drop the rest of
        // the page ~160px for the zero-save viewer this module is designed for.
        <div aria-busy="true" className="flex flex-col">
          <Skeleton className="my-1.5 h-11 w-full rounded-none" aria-hidden />
        </div>
      )}

      {state === 'empty' && (
        <div className={SAVED_SHOW_ROW_GRID}>
          <div
            className="font-data text-[11px] font-bold uppercase text-muted-foreground md:text-xs"
            aria-hidden
          >
            - - -
          </div>
          {/* One line: the "Find a show" button below is the call to action. */}
          <p className="min-w-0 self-center text-sm font-medium leading-tight text-foreground md:text-[15px]">
            Save a show and it shows up here.
          </p>
        </div>
      )}

      {state === 'empty' && (
        <Button asChild size="lg" className="w-full sm:w-auto sm:self-start">
          <Link href="/shows">Find a show</Link>
        </Button>
      )}

      {state === 'rows' && (
        <div className="flex flex-col">
          {shows.map(show => (
            <ShowRow
              key={show.id}
              show={show}
              trailing={
                <ShowRowSaveButton
                  showId={show.id}
                  // Asserted, not read back — see the note on this component.
                  saveData={{
                    save_count:
                      saveCounts?.[String(show.id)]?.save_count ?? 0,
                    is_saved: true,
                  }}
                />
              }
            />
          ))}
        </div>
      )}

      {isStale && (
        <p className="font-data text-[11px] text-muted-foreground" role="status">
          Could not refresh your saved shows. Showing the last list.
        </p>
      )}

      {(showsFooter || ownsCityLink) && (
        <div className="flex flex-wrap items-center justify-end gap-x-4 gap-y-1 font-data text-[11px]">
          {ownsCityLink && (
            <ResolvedHomeCityShowsLink className="font-data text-[11px]" />
          )}
          {showsFooter && (
            <Link
              href="/library#calendar-feed"
              className="text-muted-foreground transition-colors hover:text-link hover:underline underline-offset-4"
            >
              Subscribe to calendar →
            </Link>
          )}
        </div>
      )}

      {/* The footer's last line, in every module state, loading and error
          included: while this slot owns the row, no other surface renders it. */}
      {ownsDiscoverLinks && <SignedInHomeDiscoverLinks />}
    </section>
  )
}
