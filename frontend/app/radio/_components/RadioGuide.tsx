'use client'

import Link from 'next/link'
import {
  formatLocalTimeRange,
  formatTimeRangeInZone,
} from '@/features/radio/lib/stationOverview'
import type { RadioGuideRow } from '@/features/radio/types'

/**
 * ON NOW / UP NEXT guide rows on the /radio hub (PSY-1053 — the Option B
 * "program guide" element grafted onto the Option A hub, between THE DIAL and
 * LATEST PLAYLISTS).
 *
 * Schedule-derived and honest about it: rows exist only for stations with
 * scraped weekly schedules, ON NOW means "the schedule has this show in this
 * slot" (the dial strips above carry the live on-air claim, PSY-1239), and
 * the caption states the coverage contract for everything else. Times render
 * viewer-local (PSY-1298) with a station-local aside when the viewer is in a
 * different zone (PSY-1306 idiom).
 *
 * Renders nothing while loading, on error, or when the guide is empty — the
 * guide is an enhancement row, never a broken shell between the hub's two
 * anchor sections.
 */
export function RadioGuide({
  onNow,
  upNext,
}: {
  onNow: RadioGuideRow[] | null | undefined
  upNext: RadioGuideRow[] | null | undefined
}) {
  const onNowRows = onNow ?? []
  const upNextRows = upNext ?? []
  if (onNowRows.length === 0 && upNextRows.length === 0) return null

  return (
    <section className="mb-10" aria-label="Program guide">
      <div className="grid gap-x-10 gap-y-4 sm:grid-cols-2">
        <GuideGroup label="On now" rows={onNowRows} />
        <GuideGroup label="Up next" rows={upNextRows} />
      </div>
      <p className="mt-3 font-mono text-[11px] text-muted-foreground/70">
        Scheduled programming only — unscheduled streams appear in the feed
        when their playlists land.
      </p>
    </section>
  )
}

function GuideGroup({ label, rows }: { label: string; rows: RadioGuideRow[] }) {
  if (rows.length === 0) return null
  return (
    <div>
      <h3 className="mb-2 font-mono text-xs uppercase tracking-wider text-muted-foreground">
        {label}
      </h3>
      <ul className="flex flex-col gap-1.5">
        {rows.map(row => (
          <GuideRow key={`${row.station.slug}-${row.show.slug}-${row.starts_at}`} row={row} />
        ))}
      </ul>
    </div>
  )
}

function GuideRow({ row }: { row: RadioGuideRow }) {
  const local = formatLocalTimeRange(row.starts_at, row.ends_at)
  // The UP NEXT horizon is 24h, so a row can start past viewer-local
  // midnight — a bare "3–6 PM" would read as today and send the viewer to
  // dead air. Mark it. (The horizon caps at one day, so "tomorrow" is the
  // only case.)
  const startsTomorrow =
    new Date(row.starts_at).toDateString() !== new Date().toDateString()
  // Skip the station-local aside only when the viewer IS in the station's
  // zone (zone equality, not clock equality — same rule as the playlist
  // detail page's aired line).
  const viewerZone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const stationRange =
    row.station_timezone !== viewerZone
      ? formatTimeRangeInZone(row.starts_at, row.ends_at, row.station_timezone)
      : ''

  return (
    <li className="text-sm leading-snug">
      <Link
        href={`/radio/${row.station.slug}/${row.show.slug}`}
        className="font-medium hover:underline"
      >
        {row.show.name}
      </Link>
      {row.show.host_name && (
        <span className="text-muted-foreground"> · {row.show.host_name}</span>
      )}
      <span className="block font-mono text-xs text-muted-foreground">
        <Link href={`/radio/${row.station.slug}`} className="hover:underline">
          {row.station.name}
        </Link>
        {local && (
          <>
            {' · '}
            {local}
            {startsTomorrow && ' tomorrow'}
          </>
        )}
        {stationRange && (
          <span className="text-muted-foreground/70"> ({stationRange})</span>
        )}
      </span>
    </li>
  )
}
