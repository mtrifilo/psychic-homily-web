'use client'

import Link from 'next/link'
import { DenseTable } from '@/components/shared/DenseTable'
import {
  ArtistHops,
  formatArchiveDate,
  isAirDateToday,
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
          const episodeUrl = `/radio/${stationSlug}/${showSlug}/${episode.air_date}`
          const isToday = isAirDateToday(episode.air_date)
          const hops = previewToHops(episode.artist_preview)

          return (
            <tr key={episode.id} className="group">
              <td className="whitespace-nowrap">
                <Link
                  href={episodeUrl}
                  className="font-mono text-xs uppercase text-primary hover:text-primary/80 transition-colors"
                >
                  {formatArchiveDate(episode.air_date)}
                </Link>
              </td>
              <td>
                {episode.title ? (
                  <Link
                    href={episodeUrl}
                    className="text-primary hover:text-primary/80 transition-colors"
                  >
                    {episode.title}
                  </Link>
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
                {isToday ? (
                  <span className="text-primary">
                    <span aria-hidden="true">●</span> live
                  </span>
                ) : episode.archive_url ? (
                  <a
                    href={episode.archive_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:text-primary/80 transition-colors"
                    aria-label={`Listen to the ${formatArchiveDate(episode.air_date)} archive`}
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
