'use client'

import Link from 'next/link'
import { cn } from '@/lib/utils'
import { MusicEmbed } from '@/components/shared/MusicEmbed'
import { useSceneArtists, useSceneShows } from '../hooks'
import type { SceneListItem } from '../types'

// Format an ISO date-only string (YYYY-MM-DD) as e.g. "Fri, Jul 4" WITHOUT a
// timezone shift: parsing it via `new Date(iso)` lands at UTC midnight, which
// local formatting can render as the previous day. Force UTC on both ends.
function formatShowDate(isoDate: string): string {
  const parsed = new Date(`${isoDate}T00:00:00Z`)
  if (Number.isNaN(parsed.getTime())) return isoDate
  return parsed.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    timeZone: 'UTC',
  })
}

// How many roster rows the preview lists. We fetch a WIDER slice (below) so the
// embed search isn't capped to the shown few — a scene's only Bandcamp-having
// active band can rank below the visible list (PSY-1224 review). Full-roster
// coverage (a backend "representative embed") is the deferred complete fix.
const PREVIEW_ARTIST_LIMIT = 6
export const EMBED_SEARCH_LIMIT = 24

/**
 * The scene-preview payoff body — playable embed, "This week" shows, top local
 * artists, and the link into the full scene page. Shared between the desktop
 * globe's ScenePreviewPanel and the mobile list's expanded rows (PSY-1311), so
 * the two surfaces can't fork. Fetches on mount: mount it only when the
 * preview is actually open/expanded.
 */
export function ScenePreviewContent({
  scene,
  className,
}: {
  scene: SceneListItem
  // Layout hooks for the host container (e.g. the desktop panel's `flex-1`,
  // which lets mt-auto pin the scene link to the panel bottom) — the shared
  // body itself stays layout-neutral.
  className?: string
}) {
  const { data, isLoading, isError } = useSceneArtists({
    slug: scene.slug,
    limit: EMBED_SEARCH_LIMIT,
  })
  const artists = data?.artists ?? []
  // "This week" (PSY-1309): the scene's next shows in the 7-day window —
  // backend-defaulted window/limit, metro-scoped (member-city shows included).
  // Rendered only when non-empty; a quiet week simply has no section.
  const { data: showsData } = useSceneShows(scene.slug)
  const weekShows = showsData?.shows ?? []

  // The "instant payoff": play the first ACTIVE local band that has an
  // embeddable Bandcamp track, so opening a scene yields something to HEAR
  // immediately, not just a list (PSY-1224). The roster is active-first ordered,
  // so this is the most prominent active band with a track. Rendered only when
  // one exists — absence is the graceful empty state (no player). When one
  // exists, MusicEmbed owns its own loading state and degrades to a Bandcamp
  // link if the track can't be resolved.
  const embedArtist = artists.find((a) => a.is_active && a.bandcamp_embed_url)
  // Display only the top few; the embed may come from further down the roster.
  const displayArtists = artists.slice(0, PREVIEW_ARTIST_LIMIT)

  return (
    <div className={cn('flex flex-col gap-4', className)}>
      {embedArtist && (
        <div>
          <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Listen
          </h3>
          <div className="mt-2">
            <MusicEmbed
              bandcampAlbumUrl={embedArtist.bandcamp_embed_url}
              artistName={embedArtist.name}
              compact
            />
          </div>
        </div>
      )}

      {weekShows.length > 0 && (
        <div>
          <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            This week
          </h3>
          <ul className="mt-2 flex flex-col gap-1.5">
            {weekShows.map((show) => (
              <li key={show.id} className="text-sm leading-snug">
                <span className="font-mono text-xs text-muted-foreground">
                  {formatShowDate(show.event_date)}
                </span>{' '}
                <Link
                  href={`/shows/${show.slug || show.id}`}
                  className="underline-offset-4 hover:underline"
                >
                  {show.title}
                </Link>
                {show.venue_name && (
                  <span className="text-muted-foreground"> · {show.venue_name}</span>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div>
        <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          Local artists
        </h3>
        {isLoading ? (
          <p className="mt-2 text-sm text-muted-foreground">Loading…</p>
        ) : displayArtists.length > 0 ? (
          <ul className="mt-2 flex flex-col gap-1">
            {displayArtists.map((a) => (
              <li key={a.id} className="flex items-center gap-1.5">
                {/* Reserve the dot's width on every row so names stay aligned
                    whether or not the band is active. */}
                <span className="flex h-1.5 w-1.5 shrink-0" aria-hidden>
                  {a.is_active && (
                    <span className="h-1.5 w-1.5 rounded-full bg-success-foreground" />
                  )}
                </span>
                <Link
                  href={`/artists/${a.slug}`}
                  className="text-sm underline-offset-4 hover:underline"
                >
                  {a.name}
                </Link>
                {a.is_active && <span className="sr-only">(active)</span>}
              </li>
            ))}
          </ul>
        ) : isError ? (
          // A failed fetch must not read as an empty scene — on the mobile
          // fetch-on-tap path (flaky cell networks) that would misreport a
          // dense scene as dead. Ordered AFTER the list branch so a cached
          // roster survives a failed background refetch (data wins over
          // error). The shows section simply stays absent on its own error: a
          // quiet week and a failed week-fetch are both quiet.
          <p className="mt-2 text-sm text-muted-foreground">
            Couldn’t load this scene’s artists. Try again shortly.
          </p>
        ) : (
          <p className="mt-2 text-sm text-muted-foreground">
            No artists based here yet.
          </p>
        )}
      </div>

      {/* mt-auto pins this to the bottom only when the host grows the body
          (the desktop panel's flex-1); in a natural-height host it's inert. */}
      <Link
        href={`/scenes/${scene.slug}`}
        className="mt-auto inline-flex items-center gap-1 text-sm font-medium text-primary underline-offset-4 hover:underline"
      >
        Open scene →
      </Link>
    </div>
  )
}
