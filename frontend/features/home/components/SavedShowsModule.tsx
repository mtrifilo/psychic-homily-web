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
  SAVED_SHOWS_HOME_READ_LIMIT,
  useSavedShows,
  useShowSaveCountBatch,
} from '@/features/shows/hooks/useSavedShows'
import { useAuthContext } from '@/lib/context/AuthContext'

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

  // One read shared with NearbyShowsSection, which needs the full id set to
  // exclude. See SAVED_SHOWS_HOME_READ_LIMIT for why it is not the four rows
  // painted here.
  const { data, isPending, error } = useSavedShows({
    timeFilter: 'upcoming',
    limit: SAVED_SHOWS_HOME_READ_LIMIT,
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

  const state: ModuleState = error
    ? 'error'
    : authStatus === 'pending' || isPending
      ? 'loading'
      : total === 0
        ? 'empty'
        : 'rows'

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
          <h1
            id="signed-in-home-heading"
            className="mt-1 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl"
          >
            Your upcoming shows
          </h1>
          {state === 'rows' && (
            <p className="mt-1.5 text-sm text-muted-foreground">
              {total} saved · soonest first
              <span className="hidden sm:inline"> · times are venue-local</span>
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
            <a
              href={`#${nearbySectionId}`}
              className="whitespace-nowrap text-primary transition-colors hover:underline underline-offset-4"
            >
              Pick from this week ↓
            </a>
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

      {/* Shown in the zero state too: it is the only line that explains where a
          viewer's past saves went, and that viewer is the one asking. */}
      {(state === 'rows' || state === 'empty') && (
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 font-mono text-[11px] text-muted-foreground">
          <span>Past saved shows move to Library → Past automatically</span>
          <Link
            href="/library#calendar-feed"
            className="text-primary transition-colors hover:underline underline-offset-4"
          >
            Subscribe to calendar →
          </Link>
        </div>
      )}
    </section>
  )
}
