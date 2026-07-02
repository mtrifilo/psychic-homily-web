import {
  formatLocalAirDate,
  formatLocalTimeRange,
  formatShortAirDate,
} from '../lib/stationOverview'

interface AirDateCellContentProps {
  // string | null models the current API; undefined is tolerated contractually
  // for deploy skew (frontend ships first → cached/old rows lack the keys).
  startsAt: string | null | undefined
  endsAt: string | null | undefined
  airDate: string
}

/**
 * Text pair for the stacked date + viewer-local air-time cell (PSY-1298).
 * ONE validation decides both lines: the date derives from starts_at (fully
 * viewer-local, locked design decision) only when the window is VALID enough
 * to also render a time block — a rejected window (degenerate, one-sided,
 * unparsable) falls back to the station air_date for the date line too, so a
 * corrupt window can never shift the date while silently dropping the time
 * that would explain the shift. Also the single source for accessible names:
 * aria-labels must compose from this, never re-derive from the helpers
 * (label-in-name — visible text and accessible name may not diverge).
 */
export function airDateCellText(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined,
  airDate: string
): { dateLine: string; timeBlock: string } {
  const timeBlock = formatLocalTimeRange(startsAt, endsAt)
  const dateLine = timeBlock
    ? formatLocalAirDate(startsAt, airDate)
    : formatShortAirDate(airDate)
  return { dateLine, timeBlock }
}

/**
 * Stacked viewer-local date + air-time block for the latest-playlists tables
 * (PSY-1298) — the shared definition of the cell content for the station feed
 * and the dial-wide hub table. Renders inside the tables' font-mono/uppercase
 * DATE td; the time line opts back out of the uppercase transform so any
 * future lowercase copy renders as designed. Windowless rows (null starts_at)
 * get the date line only.
 */
export function AirDateCellContent({
  startsAt,
  endsAt,
  airDate,
}: AirDateCellContentProps) {
  const { dateLine, timeBlock } = airDateCellText(startsAt, endsAt, airDate)
  return (
    <>
      <span className="block">{dateLine}</span>
      {timeBlock && (
        <span className="block text-[10px] normal-case text-muted-foreground/85">
          {timeBlock}
        </span>
      )}
    </>
  )
}
