'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { SaveButton } from '@/components/shared/SaveButton'
// Concrete module paths, not the feature barrels: Turbopack does not
// tree-shake a `'use client'` barrel per export, so importing one here would
// ship the whole shows surface into the home chunk. See
// features/sharedChunkBarrelGuard.test.ts.
import {
  SAVED_SHOW_ROW_GRID,
  SavedShowRow,
} from '@/features/shows/components/SavedShowRow'
import {
  SAVED_SHOWS_COLLAPSED_COUNT,
  useSavedShows,
  useShowSaveCountBatch,
} from '@/features/shows/hooks/useSavedShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  ResolvedHomeCityShowsLink,
  useHomeCityLinkSlot,
} from './HomeCityShowsLink'

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
 * Every row carries a SaveButton in its SAVED state rather than Library's
 * "✕ remove": an unsave here is the same gesture that made the save, and its
 * invalidation already refreshes both this list and Library's. `is_saved` is
 * asserted rather than read back, because these rows ARE the viewer's saved
 * shows; waiting on the save-count batch to learn that would paint an empty
 * heart labelled "Save show" on every row of a saved-shows list, and a click in
 * that window would fire a redundant save instead of the unsave the viewer
 * asked for. The batch still supplies the public count for the accessible name.
 *
 * `total` (not `shows.length`) drives the subline and the zero state: the read
 * is capped, so the row count says nothing about how many saves exist.
 */
export function SavedShowsModule({
  nearbySectionId,
}: {
  /** Anchor for "Pick from this week ↓", the zero state's way down to the
   *  nearby list. */
  nearbySectionId: string
}) {
  const { user, authStatus } = useAuthContext()
  const isAuthenticated = authStatus === 'authenticated'
  // Takes the city link only when the nearby section that normally carries it
  // is hidden. The footer is where it goes, next to the other two lines that
  // point off this module.
  const cityLinkSlot = useHomeCityLinkSlot()
  const ownsCityLink = cityLinkSlot === 'saved'
  // The zero state's "Pick from this week ↓" scrolls to the nearby section. It
  // only still owns the city link while it is on the page, so that is also the
  // test for whether the anchor exists to scroll to.
  const hasNearbySection = cityLinkSlot === 'nearby'

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
  // Shown in the zero state too: it is the only line that explains where a
  // viewer's past saves went, and that viewer is the one asking.
  const showsFooter = state === 'rows' || state === 'empty'

  // The server picked this variant from the viewer's cookie; a viewer who signs
  // out without navigating leaves it mounted. Say nothing rather than address
  // someone who is no longer there.
  if (authStatus === 'anonymous') return null

  return (
    <section
      aria-labelledby="signed-in-home-heading"
      className="flex w-full flex-col gap-4 pt-2"
    >
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <p className="font-mono text-[11px] uppercase tracking-wider text-muted-foreground">
            Welcome back
          </p>
          {/* h2, not h1: this section is hideable, and the page's h1 lives on
              the layout shell so the document outline survives hiding it. */}
          <h2
            id="signed-in-home-heading"
            className="mt-1 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl"
          >
            Your upcoming shows
          </h2>
          {state === 'rows' && (
            <p className="mt-1.5 text-sm text-muted-foreground">
              {total} saved · soonest first · times are venue-local
            </p>
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

        <div className="flex shrink-0 items-center gap-4">
          <Link
            href="/library"
            className="text-sm font-medium text-muted-foreground transition-colors hover:text-primary hover:underline underline-offset-4"
          >
            View all in Library →
          </Link>
          <Button asChild size="lg">
            <Link href="/shows">Find a show</Link>
          </Button>
        </div>
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
          <Skeleton className="my-2.5 h-9 w-full rounded-none md:my-3" aria-hidden />
        </div>
      )}

      {state === 'empty' && (
        <div className={SAVED_SHOW_ROW_GRID}>
          <div
            className="font-mono text-[11px] font-bold uppercase text-muted-foreground md:text-xs"
            aria-hidden
          >
            - - -
          </div>
          <div className="min-w-0 self-center">
            <p className="text-sm font-medium leading-tight text-foreground md:text-[15px]">
              Save a show and it shows up here.
            </p>
            <p className="mt-0.5 text-xs text-muted-foreground md:text-[13px]">
              Upcoming first, past ones kept as your record in Library.
            </p>
          </div>
          <div className="col-start-2 mt-1 font-mono text-[11px] md:col-start-3 md:row-start-1 md:mt-0 md:self-center">
            {hasNearbySection ? (
              <a
                href={`#${nearbySectionId}`}
                className="whitespace-nowrap text-primary transition-colors hover:underline underline-offset-4"
              >
                Pick from this week ↓
              </a>
            ) : (
              <Link
                href="/shows"
                className="whitespace-nowrap text-primary transition-colors hover:underline underline-offset-4"
              >
                Find one to save →
              </Link>
            )}
          </div>
        </div>
      )}

      {state === 'rows' && (
        <div className="flex flex-col">
          {shows.map(show => (
            <SavedShowRow
              key={show.id}
              show={show}
              isPast={false}
              action={
                <SaveButton
                  showId={show.id}
                  showLabel
                  // The row is about THIS viewer's save; the public count is
                  // a different fact and stays in the accessible name only.
                  showCount={false}
                  // Asserted, not read back — see the note on this component.
                  saveData={{
                    save_count:
                      saveCounts?.[String(show.id)]?.save_count ?? 0,
                    is_saved: true,
                  }}
                  className="-my-1 h-auto py-1"
                />
              }
            />
          ))}
        </div>
      )}

      {isStale && (
        <p className="font-mono text-[11px] text-muted-foreground" role="status">
          Could not refresh your saved shows. Showing the last list.
        </p>
      )}

      {(showsFooter || ownsCityLink) && (
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 font-mono text-[11px] text-muted-foreground">
          {showsFooter && (
            <span>Past saved shows move to Library → Past automatically</span>
          )}
          {/* ml-auto, not justify-between alone: the left line is absent when
              this row exists only to carry the relocated city link. */}
          <div className="ml-auto flex flex-wrap items-center gap-x-4 gap-y-1">
            {ownsCityLink && (
              <ResolvedHomeCityShowsLink className="font-mono text-[11px] text-primary transition-colors hover:underline underline-offset-4" />
            )}
            {showsFooter && (
              <Link
                href="/library#calendar-feed"
                className="text-primary transition-colors hover:underline underline-offset-4"
              >
                Subscribe to calendar →
              </Link>
            )}
          </div>
        </div>
      )}
    </section>
  )
}
