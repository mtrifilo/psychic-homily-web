'use client'

import { Fragment, useState } from 'react'
import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import { BracketLink, DenseTable, SectionHeader } from '@/components/shared'
import { useStationEpisodes } from '../hooks/useStationEpisodes'
import { formatShortAirDate } from '../lib/stationOverview'
import type { RadioStationDetail, RadioStationEpisodeRow } from '../types'

const INITIAL_LIMIT = 10
const LOAD_MORE_STEP = 20
// The episodes endpoint caps limit at 100; past that the feed points readers
// at the per-show episode pages instead of paginating further in place.
const MAX_LIMIT = 100

interface StationPlaylistsFeedProps {
  station: RadioStationDetail
}

/**
 * "Latest playlists" dense table (PSY-1050): newest episodes across all of a
 * station's shows, from GET /radio-stations/{slug}/episodes (PSY-1048). The
 * feed is strictly per-station (PSY-1074) — a network flagship's tab shows
 * only its own playlists; channel shows live under their own tabs.
 *
 * Pagination is in-place: start at 10 rows, "More playlists" grows the query
 * limit by 20 (keepPreviousData keeps the table stable while the larger page
 * loads) up to the API's limit cap of 100.
 */
export function StationPlaylistsFeed({ station }: StationPlaylistsFeedProps) {
  const [limit, setLimit] = useState(INITIAL_LIMIT)
  const { data, isLoading, isFetching } = useStationEpisodes({
    stationSlug: station.slug,
    limit,
  })

  const episodes = data?.episodes ?? []
  const total = data?.total ?? 0

  const canLoadMore = episodes.length < total && limit < MAX_LIMIT

  return (
    <section aria-label="Latest playlists">
      <SectionHeader title="Latest playlists" as="h2" size="md" />

      {isLoading && (
        <div className="flex justify-center py-6">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      )}

      {!isLoading && episodes.length === 0 && (
        <p className="py-4 text-sm text-muted-foreground">No playlists logged yet.</p>
      )}

      {!isLoading && episodes.length > 0 && (
        <>
          <DenseTable variant="bare">
            <thead>
              <tr>
                <th className="w-16">Date</th>
                <th>Show</th>
                <th>Played</th>
                <th className="text-right w-16">Tracks</th>
              </tr>
            </thead>
            <tbody>
              {episodes.map(row => (
                <PlaylistRow key={row.id} row={row} />
              ))}
            </tbody>
          </DenseTable>

          <div className="flex items-baseline gap-2 mt-2">
            {canLoadMore && (
              <BracketLink
                label={isFetching ? 'Loading…' : 'More playlists'}
                onClick={() => setLimit(l => Math.min(l + LOAD_MORE_STEP, MAX_LIMIT))}
                disabled={isFetching}
              />
            )}
            <span className="font-mono text-xs text-muted-foreground tabular-nums">
              showing {episodes.length} of {total} playlists
            </span>
          </div>
        </>
      )}
    </section>
  )
}

function PlaylistRow({ row }: { row: RadioStationEpisodeRow }) {
  const playlistUrl = `/radio/${row.station_slug}/${row.show_slug}/${row.air_date}`
  const showUrl = `/radio/${row.station_slug}/${row.show_slug}`

  return (
    <tr>
      <td className="whitespace-nowrap font-mono text-xs uppercase text-muted-foreground">
        <Link
          href={playlistUrl}
          className="hover:text-foreground transition-colors"
          aria-label={`Playlist from ${row.air_date}`}
        >
          {formatShortAirDate(row.air_date)}
        </Link>
      </td>
      <td>
        <Link
          href={showUrl}
          className="font-medium text-foreground hover:text-primary transition-colors"
        >
          {row.show_name}
        </Link>
      </td>
      <td className="text-muted-foreground">
        <ArtistPreview artists={row.artist_preview} />
      </td>
      <td className="text-right text-muted-foreground">{row.play_count}</td>
    </tr>
  )
}

/**
 * Short " · "-joined artist preview. Matched artists (slug present) link into
 * the knowledge graph; unmatched names render as plain text — never a dead
 * link. An empty preview (no plays logged) renders an em dash.
 */
function ArtistPreview({
  artists,
}: {
  artists: RadioStationEpisodeRow['artist_preview']
}) {
  if (!artists || artists.length === 0) {
    return <span aria-hidden>—</span>
  }
  return (
    <>
      {artists.map((artist, i) => (
        <Fragment key={`${artist.artist_name}-${i}`}>
          {i > 0 && <span aria-hidden> · </span>}
          {artist.artist_slug ? (
            <Link
              href={`/artists/${artist.artist_slug}`}
              className="hover:text-foreground transition-colors"
            >
              {artist.artist_name}
            </Link>
          ) : (
            <span>{artist.artist_name}</span>
          )}
        </Fragment>
      ))}
    </>
  )
}
