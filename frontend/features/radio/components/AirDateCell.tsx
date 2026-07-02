import { formatLocalAirDate, formatLocalTimeRange } from '../lib/stationOverview'

interface AirDateCellContentProps {
  startsAt: string | null
  endsAt: string | null
  airDate: string
}

/**
 * Stacked viewer-local date + air-time block for the latest-playlists tables
 * (PSY-1298) — the single definition of the cell content so the station feed
 * and the dial-wide hub table cannot drift. Renders inside the tables'
 * font-mono/uppercase DATE td; the time line opts back out of the uppercase
 * transform so any future lowercase copy renders as designed. Windowless rows
 * (null starts_at) get the date line only.
 */
export function AirDateCellContent({
  startsAt,
  endsAt,
  airDate,
}: AirDateCellContentProps) {
  const timeBlock = formatLocalTimeRange(startsAt, endsAt)
  return (
    <>
      <span className="block">{formatLocalAirDate(startsAt, airDate)}</span>
      {timeBlock && (
        <span className="block text-[10px] normal-case text-muted-foreground/85">
          {timeBlock}
        </span>
      )}
    </>
  )
}
