'use client'

import Link from 'next/link'
import type { SavedShowResponse } from '@/features/shows'
import { formatShowMonthDay } from '@/lib/utils/showDateBadge'

function displayName(show: SavedShowResponse): string {
  if (show.artists.length > 0) {
    return show.artists.map(a => a.name).join(' · ')
  }
  return show.title
}

function primaryName(show: SavedShowResponse): string {
  return show.artists[0]?.name ?? show.title
}

function metaLine(show: SavedShowResponse): string {
  const monthDay = formatShowMonthDay(
    show.event_date,
    show.state,
    show.venues?.[0]?.timezone
  )
  const venue = show.venues[0]?.name
  return venue ? `${monthDay} · ${venue}` : monthDay
}

function WallTile({
  show,
  onRemove,
  isRemoving,
  isRemovalPending,
}: {
  show: SavedShowResponse
  onRemove: (showId: number) => void
  isRemoving: boolean
  isRemovalPending: boolean
}) {
  const name = displayName(show)
  const tileLabel = primaryName(show)
  const href = `/shows/${show.slug || show.id}`
  const hasImage = Boolean(show.image_url)

  return (
    <article
      aria-label={name}
      className="group flex min-w-0 flex-col gap-1.5"
      data-testid="library-wall-tile"
    >
      <Link
        href={href}
        className="block aspect-square overflow-hidden border border-border bg-muted/40 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {hasImage ? (
          /* eslint-disable-next-line @next/next/no-img-element -- flyer URLs are hotlinked hosts outside next/image remotePatterns */
          <img
            src={show.image_url ?? ''}
            alt=""
            className="h-full w-full object-cover"
            data-testid="library-wall-tile-image"
          />
        ) : (
          <div
            className="flex h-full w-full items-center justify-center bg-muted/50 px-3 text-center"
            data-testid="library-wall-tile-fallback"
          >
            <span className="line-clamp-3 text-sm font-medium leading-snug text-foreground/80">
              {tileLabel}
            </span>
          </div>
        )}
      </Link>

      <div className="min-w-0">
        <Link
          href={href}
          className="block truncate text-[13px] font-medium leading-tight transition-colors hover:text-primary"
        >
          {name}
        </Link>
        <p className="mt-0.5 truncate font-mono text-[10px] uppercase tracking-wide text-muted-foreground">
          {metaLine(show)}
        </p>
        <button
          type="button"
          onClick={() => onRemove(show.id)}
          disabled={isRemovalPending}
          className="mt-1 font-mono text-[10px] text-muted-foreground transition-colors hover:text-destructive disabled:cursor-wait disabled:opacity-60"
          aria-label={`Remove ${show.title} from saved shows`}
        >
          {isRemoving ? 'removing…' : '✕ remove'}
        </button>
      </div>
    </article>
  )
}

export interface LibraryWallGridProps {
  shows: SavedShowResponse[]
  onRemove: (showId: number) => void
  removingShowId?: number
  isRemovalPending: boolean
}

/**
 * Cover-art wall for saved shows (PSY-1429 State H).
 * No-image entities render as typographic tiles (muted surface + hairline +
 * centred name) — a legitimate design state, not a broken placeholder.
 */
export function LibraryWallGrid({
  shows,
  onRemove,
  removingShowId,
  isRemovalPending,
}: LibraryWallGridProps) {
  return (
    <div
      className="grid grid-cols-2 gap-x-3 gap-y-5 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5"
      data-testid="library-wall-grid"
    >
      {shows.map(show => (
        <WallTile
          key={show.id}
          show={show}
          onRemove={onRemove}
          isRemoving={removingShowId === show.id}
          isRemovalPending={isRemovalPending}
        />
      ))}
    </div>
  )
}
