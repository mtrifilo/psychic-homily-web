'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Loader2 } from 'lucide-react'
import { BracketLink } from '@/components/shared/BracketLink'
import { DenseTable } from '@/components/shared/DenseTable'
import { getNewReleaseHref, useNewReleaseRadar } from '@/features/radio'
import type { RadioNewReleaseRadarEntry } from '@/features/radio'

const INITIAL_LIMIT = 20
const LOAD_MORE_STEP = 20
// GET /radio/new-releases caps limit at 100 and exposes no offset, so the
// full radar view tops out at the 100 hottest entries (documented cap,
// PSY-1076). The response carries count (page size) but no grand total.
const MAX_LIMIT = 100

/**
 * /radio/new-releases (PSY-1076): the full New Release Radar — the hub
 * sidebar box's capped teaser (PSY-1049) as a dedicated dense-table view
 * with label/plays/stations columns. Link resolution shares
 * getNewReleaseHref with the hub box (release → artist → plain text, no
 * dead links).
 *
 * Pagination is in-place (PSY-1050's choice): "More releases" grows the
 * query limit by 20 up to the API cap. With no total in the response, the
 * control shows whenever the last page came back full.
 */
export default function NewReleaseRadarPage() {
  const [limit, setLimit] = useState(INITIAL_LIMIT)
  const { data, isLoading, isFetching, isPlaceholderData, error } =
    useNewReleaseRadar({ limit })

  const releases = data?.releases ?? []

  // A short page means the radar is exhausted; a full page may have more.
  // While a limit bump is in flight, keepPreviousData serves the OLD (short)
  // page against the NEW limit — isPlaceholderData keeps the control mounted
  // in its disabled "Loading…" state instead of flickering out and back
  // (and, unlike isFetching, doesn't resurrect it on background refetches of
  // an exhausted radar).
  const canLoadMore =
    (releases.length >= limit || isPlaceholderData) && limit < MAX_LIMIT

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-4xl px-4 py-8 md:px-8">
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
          <h1 className="text-3xl font-bold">New release radar</h1>
          <p className="mt-1.5 text-muted-foreground">
            New releases surfaced by radio play across the dial — most played
            first.
          </p>
        </header>

        {isLoading && (
          <div className="flex justify-center py-10">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
            <span className="sr-only">Loading new release radar</span>
          </div>
        )}

        {!isLoading && !!error && (
          <p className="py-6 text-sm text-muted-foreground">
            Couldn&apos;t load the new release radar.
          </p>
        )}

        {!isLoading && !error && releases.length === 0 && (
          <p className="py-6 text-sm text-muted-foreground">
            Nothing on the radar yet.
          </p>
        )}

        {!isLoading && !error && releases.length > 0 && (
          <>
            <DenseTable variant="standard">
              <thead>
                <tr>
                  <th>Release</th>
                  <th>Label</th>
                  <th className="text-right">Plays</th>
                  <th className="text-right">Stations</th>
                </tr>
              </thead>
              <tbody>
                {releases.map((entry, idx) => (
                  <RadarRow
                    key={`${entry.artist_name}-${entry.album_title}-${idx}`}
                    entry={entry}
                  />
                ))}
              </tbody>
            </DenseTable>

            <div className="flex items-baseline gap-2 mt-2">
              {canLoadMore && (
                <BracketLink
                  label={isFetching ? 'Loading…' : 'More releases'}
                  onClick={() =>
                    setLimit(l => Math.min(l + LOAD_MORE_STEP, MAX_LIMIT))
                  }
                  disabled={isFetching}
                />
              )}
              <span className="font-mono text-xs text-muted-foreground tabular-nums">
                showing {releases.length}{' '}
                {releases.length === 1 ? 'release' : 'releases'}
              </span>
            </div>
          </>
        )}
      </main>
    </div>
  )
}

function RadarRow({ entry }: { entry: RadioNewReleaseRadarEntry }) {
  const title = entry.album_title
    ? `${entry.artist_name} — ${entry.album_title}`
    : entry.artist_name
  const href = getNewReleaseHref(entry)

  return (
    <tr>
      <td>
        {href ? (
          <Link
            href={href}
            className="font-medium text-primary transition-colors hover:underline"
          >
            {title}
          </Link>
        ) : (
          <span className="font-medium text-foreground">{title}</span>
        )}
      </td>
      <td className="text-muted-foreground">
        {entry.label_name ? (
          entry.label_slug ? (
            <Link
              href={`/labels/${entry.label_slug}`}
              className="hover:text-foreground transition-colors"
            >
              {entry.label_name}
            </Link>
          ) : (
            entry.label_name
          )
        ) : (
          <span aria-hidden>—</span>
        )}
      </td>
      <td className="text-right text-muted-foreground">{entry.play_count}</td>
      <td className="text-right text-muted-foreground">
        {entry.station_count}
      </td>
    </tr>
  )
}
