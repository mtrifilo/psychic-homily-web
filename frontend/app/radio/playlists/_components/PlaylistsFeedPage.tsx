'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ArrowLeft } from 'lucide-react'
import { BracketLink } from '@/components/shared/BracketLink'
import { useRecentRadioEpisodes } from '@/features/radio'
import { LatestPlaylistsTable } from '../../_components/LatestPlaylistsTable'

const INITIAL_LIMIT = 20
const LOAD_MORE_STEP = 20
// The recent-episodes endpoint caps limit at 100; past that the feed points
// readers at the per-station and per-show pages instead of paginating
// further in place (same call as StationPlaylistsFeed, PSY-1050).
const MAX_LIMIT = 100

/**
 * /radio/playlists (PSY-1076): the full dial-wide latest-playlists feed —
 * the hub's "Latest playlists — across the dial" teaser (PSY-1049) as a
 * dedicated destination. Reuses the hub's LatestPlaylistsTable so the row
 * anatomy has one source of truth.
 *
 * Pagination is in-place (PSY-1050's choice): "More playlists" grows the
 * query limit by 20 (keepPreviousData keeps the table stable while the
 * larger page loads) up to the API's limit cap of 100.
 */
export default function PlaylistsFeedPage() {
  const [limit, setLimit] = useState(INITIAL_LIMIT)
  const { data, isLoading, isFetching, error } = useRecentRadioEpisodes({
    limit,
  })

  const episodes = data?.episodes ?? []
  const total = data?.total ?? 0

  const canLoadMore = episodes.length < total && limit < MAX_LIMIT

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        {/* Breadcrumb */}
        <div className="mb-6">
          <Link
            href="/radio"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Radio
          </Link>
        </div>

        <header className="mb-6">
          <h1 className="text-3xl font-bold">All playlists</h1>
          <p className="mt-1.5 text-muted-foreground">
            Every playlist tracked across the dial, newest first.
          </p>
        </header>

        <LatestPlaylistsTable
          rows={data?.episodes}
          isLoading={isLoading}
          error={error}
        />

        {!isLoading && !error && episodes.length > 0 && (
          <div className="flex items-baseline gap-2 mt-2">
            {canLoadMore && (
              <BracketLink
                label={isFetching ? 'Loading…' : 'More playlists'}
                onClick={() =>
                  setLimit(l => Math.min(l + LOAD_MORE_STEP, MAX_LIMIT))
                }
                disabled={isFetching}
              />
            )}
            <span className="font-mono text-xs text-muted-foreground tabular-nums">
              showing {episodes.length} of {total} playlists
            </span>
          </div>
        )}
      </main>
    </div>
  )
}
