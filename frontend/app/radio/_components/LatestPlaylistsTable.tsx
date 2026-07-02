'use client'

import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import { DenseTable } from '@/components/shared/DenseTable'
import { AirDateCellContent, ArtistHops } from '@/features/radio'
import type { RadioStationEpisodeRow } from '@/features/radio'

interface LatestPlaylistsTableProps {
  rows: RadioStationEpisodeRow[] | undefined
  isLoading: boolean
  error: unknown
}

/**
 * "LATEST PLAYLISTS — ACROSS THE DIAL" (PSY-1049): the dial-wide recent feed
 * (PSY-1048) as a dense table — date · station · show · played preview ·
 * track count. Show names are orange content links; preview artists link into
 * the graph only when matched (ArtistHops rule — no dead links).
 *
 * Presentational: the hub owns the fetch (useRecentRadioEpisodes).
 */
export function LatestPlaylistsTable({
  rows,
  isLoading,
  error,
}: LatestPlaylistsTableProps) {
  if (isLoading) {
    return (
      <div className="flex justify-center py-10">
        <Loader2 className="size-5 animate-spin text-muted-foreground" />
        <span className="sr-only">Loading latest playlists</span>
      </div>
    )
  }

  if (error) {
    return (
      <p className="py-6 text-sm text-muted-foreground">
        Couldn&apos;t load the latest playlists.
      </p>
    )
  }

  if (!rows || rows.length === 0) {
    return (
      <p className="py-6 text-sm text-muted-foreground">
        No playlists tracked yet.
      </p>
    )
  }

  return (
    <DenseTable variant="standard">
      <thead>
        <tr>
          <th>Date</th>
          <th>Station</th>
          <th>Show</th>
          <th>Played</th>
          <th className="text-right">Tracks</th>
        </tr>
      </thead>
      <tbody>
        {rows.map(row => (
          <tr key={row.id}>
            {/* PSY-1298: shared stacked viewer-local date + air-time cell —
                same AirDateCellContent the station feed renders. */}
            <td className="whitespace-nowrap font-mono text-xs uppercase text-muted-foreground align-top">
              <AirDateCellContent
                startsAt={row.starts_at}
                endsAt={row.ends_at}
                airDate={row.air_date}
              />
            </td>
            <td className="max-w-[8rem] truncate font-mono text-xs uppercase text-muted-foreground">
              {row.station_name}
            </td>
            <td>
              <Link
                href={`/radio/${row.station_slug}/${row.show_slug}`}
                className="font-medium text-primary transition-colors hover:underline"
              >
                {row.show_name}
              </Link>
            </td>
            <td className="text-muted-foreground">
              {row.artist_preview.length > 0 ? (
                <ArtistHops
                  hops={row.artist_preview.map(a => ({
                    name: a.artist_name,
                    slug: a.artist_slug,
                  }))}
                  className="text-sm text-muted-foreground [&_a]:text-foreground"
                />
              ) : (
                <span aria-hidden>—</span>
              )}
            </td>
            <td className="text-right text-muted-foreground">
              {row.play_count}
            </td>
          </tr>
        ))}
      </tbody>
    </DenseTable>
  )
}
