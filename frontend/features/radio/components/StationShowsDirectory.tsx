'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { BracketLink, DenseTable, SectionHeader } from '@/components/shared'
import { useRadioShows } from '../hooks/useRadioShows'
import { formatShortAirDate } from '../lib/stationOverview'
import type { RadioShowListItem } from '../types'

const COLLAPSED_ROW_COUNT = 10

interface StationShowsDirectoryProps {
  stationId: number
  stationSlug: string
}

/**
 * Dense sortable shows directory (PSY-1050) — replaces the old card grid.
 * Ordering is server-side via GET /radio-shows?sort=latest (PSY-1048):
 * active shows first, most recent playlist first, archived shows last. The
 * station page deliberately serves INACTIVE stations'/shows' archives too,
 * so archived rows render (muted) rather than being filtered out.
 *
 * Expansion decision (per ticket): in-place. The list endpoint returns every
 * show in one response, so "View all" is a pure client-side slice toggle —
 * first 10 rows collapsed, the full list expanded, no extra fetch.
 */
export function StationShowsDirectory({
  stationId,
  stationSlug,
}: StationShowsDirectoryProps) {
  const [expanded, setExpanded] = useState(false)
  const { data, isLoading } = useRadioShows(stationId, { sort: 'latest' })

  const shows = data?.shows ?? []
  const visible = expanded ? shows : shows.slice(0, COLLAPSED_ROW_COUNT)
  const activeCount = shows.filter(s => s.is_active).length
  const archivedCount = shows.length - activeCount

  return (
    <section aria-label="Shows">
      <SectionHeader
        title="Shows — active first, sorted by latest playlist"
        as="h2"
        size="md"
      />

      {isLoading && (
        <div className="flex justify-center py-6">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      )}

      {!isLoading && shows.length === 0 && (
        <p className="py-4 text-sm text-muted-foreground">No shows yet.</p>
      )}

      {!isLoading && shows.length > 0 && (
        <>
          <DenseTable variant="bare">
            <thead>
              <tr>
                <th>Show</th>
                <th>Host</th>
                <th>Schedule</th>
                <th>Genres</th>
                <th className="whitespace-nowrap w-16">Last</th>
                <th className="text-right w-12">Eps</th>
              </tr>
            </thead>
            <tbody>
              {visible.map(show => (
                <ShowRow key={show.id} show={show} stationSlug={stationSlug} />
              ))}
            </tbody>
          </DenseTable>

          <div className="flex items-baseline gap-2 mt-2">
            <span className="font-mono text-xs text-muted-foreground tabular-nums">
              {activeCount} active
              {archivedCount > 0 && ` · ${archivedCount} archived`}
            </span>
            {shows.length > COLLAPSED_ROW_COUNT && (
              <BracketLink
                label={expanded ? 'Show fewer' : `View all ${shows.length}`}
                onClick={() => setExpanded(e => !e)}
              />
            )}
          </div>
        </>
      )}
    </section>
  )
}

function ShowRow({
  show,
  stationSlug,
}: {
  show: RadioShowListItem
  stationSlug: string
}) {
  const lastDate = formatShortAirDate(show.latest_air_date)
  const genres = (show.genre_tags ?? []).slice(0, 3).join(' · ')

  return (
    <tr className={cn(!show.is_active && 'opacity-60')}>
      <td>
        <Link
          href={`/radio/${stationSlug}/${show.slug}`}
          className="font-medium text-foreground hover:text-primary transition-colors"
        >
          {show.name}
        </Link>
      </td>
      <td className="text-muted-foreground">{show.host_name ?? '—'}</td>
      <td className="whitespace-nowrap font-mono text-xs text-muted-foreground">
        {show.schedule_display ?? '—'}
      </td>
      <td className="font-mono text-xs text-muted-foreground">{genres || '—'}</td>
      <td className="whitespace-nowrap font-mono text-xs uppercase text-muted-foreground">
        {lastDate || '—'}
      </td>
      <td className="text-right text-muted-foreground">{show.episode_count}</td>
    </tr>
  )
}
