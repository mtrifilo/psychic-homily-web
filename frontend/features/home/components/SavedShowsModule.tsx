'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { SaveButton } from '@/components/shared/SaveButton'
import { batchedSaveFor } from '@/components/shared/batchedSaveData'
// Concrete module paths, not the feature barrels: Turbopack does not
// tree-shake a `'use client'` barrel per export, so importing one here would
// ship Library's wall grid and the whole shows surface into the home chunk.
// See features/sharedChunkBarrelGuard.test.ts.
import { SavedShowRow } from '@/features/library/components/SavedShowRow'
import {
  useSavedShows,
  useShowSaveCountBatch,
} from '@/features/shows/hooks/useSavedShows'
import { useAuthContext } from '@/lib/context/AuthContext'

/**
 * Rows shown before the viewer is sent to Library. Mirrors the Library Shows
 * tab's collapsed count so the same save reads the same way on both surfaces.
 */
const SAVED_ROW_CAP = 4

/**
 * The signed-in home's lead module (PSY-2103): the viewer's next saved shows,
 * in place of the logged-out wordmark hero.
 *
 * It closes the save loop — save a show, come back, see it — which previously
 * had its second half only at /library, two clicks behind the avatar.
 *
 * Every row carries the pressed SaveButton rather than Library's "✕ remove":
 * an unsave here is the same gesture that made the save, and its invalidation
 * already refreshes both this list and Library's.
 *
 * `total` (not `shows.length`) drives the subline and the zero state: the query
 * asks for at most {@link SAVED_ROW_CAP} rows, so the row count says nothing
 * about how many saves exist.
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

  const { data, isPending, error } = useSavedShows({
    timeFilter: 'upcoming',
    limit: SAVED_ROW_CAP,
    userId: user?.id,
    enabled: isAuthenticated,
  })

  const shows = useMemo(() => data?.shows ?? [], [data?.shows])
  const total = data?.total ?? 0

  const showIds = useMemo(() => shows.map(show => show.id), [shows])
  const { data: saveCounts } = useShowSaveCountBatch(
    showIds,
    isAuthenticated,
    user?.id
  )

  // An unresolved read is not an answer about this viewer's saves, so neither
  // the count nor the zero state may be painted from it.
  const isSettled = isAuthenticated && !isPending && !error

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
          {error ? null : isSettled ? (
            <p className="mt-1.5 text-sm text-muted-foreground">
              {total > 0 ? (
                <>
                  {total} saved · soonest first
                  <span className="hidden sm:inline">
                    {' '}
                    · times are venue-local
                  </span>
                </>
              ) : (
                'Nothing saved yet · tap ♡ on any show and it lands here'
              )}
            </p>
          ) : (
            // Reserves the subline's line box so the settled copy does not
            // push the rows down when it arrives.
            <Skeleton className="mt-2 h-4 w-64" aria-hidden />
          )}
        </div>

        <div className="flex shrink-0 items-center gap-4">
          <Link
            href="/library?tab=shows"
            className="text-sm font-medium text-muted-foreground transition-colors hover:text-primary hover:underline underline-offset-4"
          >
            View all in Library →
          </Link>
          <Button asChild size="lg">
            <Link href="/shows">Find a show</Link>
          </Button>
        </div>
      </div>

      {error ? (
        <p className="py-4 text-sm text-muted-foreground">
          Unable to load your saved shows.
        </p>
      ) : !isSettled ? (
        <div aria-busy="true" className="flex flex-col">
          {Array.from({ length: SAVED_ROW_CAP }, (_, i) => (
            <Skeleton
              key={i}
              className="my-2.5 h-9 w-full rounded-none md:my-3"
              aria-hidden
            />
          ))}
        </div>
      ) : total === 0 ? (
        <div className="grid grid-cols-[74px_minmax(0,1fr)] gap-x-3 border-b border-border py-2.5 md:grid-cols-[104px_minmax(0,1fr)_auto] md:gap-x-5 md:py-3">
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
      ) : (
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
                  // The row is about THIS viewer's save; the public count is a
                  // different fact and stays in the accessible name only.
                  showCount={false}
                  saveData={batchedSaveFor(saveCounts, show.id)}
                  className="-my-1 h-auto py-1"
                />
              }
            />
          ))}
        </div>
      )}

      {isSettled && total > 0 && (
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 font-mono text-[11px] text-muted-foreground">
          <span>Past saved shows move to Library → Past automatically</span>
          <Link
            href="/library?tab=shows"
            className="text-primary transition-colors hover:underline underline-offset-4"
          >
            Subscribe to calendar →
          </Link>
        </div>
      )}
    </section>
  )
}
