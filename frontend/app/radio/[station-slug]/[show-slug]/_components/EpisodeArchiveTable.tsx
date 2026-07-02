'use client'

import type { ReactNode } from 'react'
import Link from 'next/link'
import { DenseTable } from '@/components/shared/DenseTable'
import {
  AirDateCellContent,
  airDateCellText,
  ArtistHops,
  isLiveNow,
  previewToHops,
} from '@/features/radio'
import type { RadioEpisodeListItem } from '@/features/radio'

interface EpisodeArchiveTableProps {
  episodes: RadioEpisodeListItem[]
  stationSlug: string
  showSlug: string
}

/**
 * The show page's playlist archive (PSY-1051): one reverse-chron episode
 * table — date · episode title · played preview · tracks · [mp3]. The show
 * page IS its archive (WFMU model, locked PSY-1049 decision 4); there is no
 * separate episodes sub-page. Episode previews come from the PSY-1048
 * artist_preview extension on the episodes list — no per-row detail fetch.
 */
export function EpisodeArchiveTable({
  episodes,
  stationSlug,
  showSlug,
}: EpisodeArchiveTableProps) {
  return (
    <DenseTable>
      <thead>
        <tr>
          <th className="w-28">Date</th>
          <th className="w-48">Episode</th>
          <th>Played</th>
          <th className="w-16 text-right">Tracks</th>
          <th className="w-20 text-right">
            <span className="sr-only">Archive</span>
          </th>
        </tr>
      </thead>
      <tbody>
        {episodes.map(episode => {
          // Upcoming rows aren't linkable — there's no playlist yet, so a link
          // would lead to an empty, not-yet-aired page (PSY-1205).
          const episodeUrl = episode.is_upcoming
            ? undefined
            : `/radio/${stationSlug}/${showSlug}/${episode.air_date}`
          const isLive = isLiveNow(episode.starts_at, episode.ends_at)
          // Same viewer-local date the cell shows — accessible names must not
          // announce a different day than the rendered text (PSY-1306).
          const cellDate = airDateCellText(episode.starts_at, episode.ends_at, episode.air_date, {
            withYear: true,
          }).dateLine
          const hops = previewToHops(episode.artist_preview)

          return (
            <tr key={episode.id} className="group">
              {/* PSY-1306: viewer-local date (+ air-time block) — the same
                  AirDateCellContent treatment as the playlists feeds, with the
                  year (archives span years). */}
              <td className="whitespace-nowrap align-top">
                <MaybeLink
                  href={episodeUrl}
                  linkedClassName="font-mono text-xs uppercase text-primary hover:text-primary/80 transition-colors"
                  plainClassName="font-mono text-xs uppercase text-muted-foreground"
                >
                  <AirDateCellContent
                    startsAt={episode.starts_at}
                    endsAt={episode.ends_at}
                    airDate={episode.air_date}
                    withYear
                  />
                </MaybeLink>
              </td>
              <td>
                {episode.title ? (
                  <MaybeLink
                    href={episodeUrl}
                    linkedClassName="text-primary hover:text-primary/80 transition-colors"
                    plainClassName="text-muted-foreground"
                  >
                    {episode.title}
                  </MaybeLink>
                ) : (
                  <span className="text-muted-foreground/50" aria-hidden="true">
                    —
                  </span>
                )}
              </td>
              <td className="max-w-0">
                <div className="truncate">
                  {hops.length > 0 ? (
                    <ArtistHops
                      hops={hops}
                      className="text-muted-foreground [&_a]:hover:text-primary"
                    />
                  ) : (
                    <span className="text-muted-foreground/50">&nbsp;</span>
                  )}
                </div>
              </td>
              <td className="text-right tabular-nums text-muted-foreground">
                {episode.play_count}
              </td>
              <td className="text-right whitespace-nowrap font-mono text-xs">
                {episode.is_upcoming ? (
                  // Not yet aired (PSY-1205): label it rather than linking to an
                  // empty, aired-looking [mp3] archive page.
                  <span className="text-muted-foreground">upcoming</span>
                ) : isLive ? (
                  <span className="text-primary">
                    <span aria-hidden="true">●</span> live
                  </span>
                ) : episode.archive_url ? (
                  <a
                    href={episode.archive_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:text-primary/80 transition-colors"
                    aria-label={`Listen to the ${cellDate} archive`}
                  >
                    [ mp3 ]
                  </a>
                ) : null}
              </td>
            </tr>
          )
        })}
      </tbody>
    </DenseTable>
  )
}

/**
 * Renders children inside a Link when `href` is set, else as a plain span. Lets
 * the archive show an upcoming (non-linkable) row's date/title as text — one
 * boolean (`linkable`) instead of a duplicated link-vs-text branch per cell.
 */
function MaybeLink({
  href,
  linkedClassName,
  plainClassName,
  children,
}: {
  href: string | undefined
  linkedClassName: string
  plainClassName: string
  children: ReactNode
}) {
  return href ? (
    <Link href={href} className={linkedClassName}>
      {children}
    </Link>
  ) : (
    <span className={plainClassName}>{children}</span>
  )
}
