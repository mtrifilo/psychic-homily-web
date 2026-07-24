/**
 * Station-overview derivation helpers (PSY-1016; consumed by the Dial
 * surfaces since PSY-1049/1050).
 *
 * Pure, hook-free transforms that turn the radio API responses into the
 * shapes the station surfaces render. The "Now Playing" derivation that
 * originally lived here was superseded by the live now-playing endpoint
 * (PSY-1022) and removed (PSY-1075).
 */

/**
 * A single artist hop (name + optional graph link). `slug` is null when the
 * matching engine hasn't linked the play to a catalog artist yet — the panel
 * renders those as plain text rather than a dead link.
 */
export interface ArtistHop {
  name: string
  slug: string | null
}

/**
 * Format a YYYY-MM-DD air-date as a short "Jun 4" (no year), the dense
 * editorial register the radio surfaces share. Parses at local midnight so a date-only string doesn't shift a day
 * in negative-offset timezones.
 */
export function formatShortAirDate(dateStr: string | null | undefined): string {
  if (!dateStr) return ''
  const date = new Date(dateStr + 'T00:00:00')
  if (isNaN(date.getTime())) return ''
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

/**
 * The one definition of the "Jul 1" / "Jul 1 2026" short-date rendering all
 * paths share. Year on demand (archive surfaces span years; feeds do not).
 */
function shortDate(date: Date, withYear = false): string {
  const monthDay = date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
  return withYear ? `${monthDay} ${date.getFullYear()}` : monthDay
}

/**
 * Viewer-local date line for a feed row (PSY-1298): when the frozen air
 * window exists, the date derives from starts_at in the VIEWER's timezone
 * (fully viewer-local — an 11 PM ET Tuesday broadcast reads as Wednesday in
 * Berlin, locked design decision); windowless rows fall back to the station
 * air_date, date-only. `withYear` for archive surfaces (PSY-1306).
 */
export function formatLocalAirDate(
  startsAt: string | null | undefined,
  airDate: string | null | undefined,
  opts: { withYear?: boolean } = {}
): string {
  if (startsAt) {
    const date = new Date(startsAt)
    if (!isNaN(date.getTime())) {
      return shortDate(date, opts.withYear)
    }
  }
  if (!airDate) return ''
  const fallback = new Date(airDate + 'T00:00:00')
  if (isNaN(fallback.getTime())) return ''
  return shortDate(fallback, opts.withYear)
}

interface ClockParts {
  hours24: number
  minutes: number
}

/**
 * One end of the air window as compact 12h: drop :00 minutes ("9", "6:30",
 * "12" for noon/midnight), meridiem carried separately so the range renderer
 * decides whether to show it once or twice.
 */
function formatCompactTime({ hours24, minutes }: ClockParts): {
  clock: string
  meridiem: string
} {
  const meridiem = hours24 < 12 ? 'AM' : 'PM'
  const hours12 = hours24 % 12 === 0 ? 12 : hours24 % 12
  const clock =
    minutes === 0 ? `${hours12}` : `${hours12}:${String(minutes).padStart(2, '0')}`
  return { clock, meridiem }
}

/** Shared compact-range composition: single AM/PM only when both ends share it. */
function compactRangeText(startParts: ClockParts, endParts: ClockParts): string {
  const s = formatCompactTime(startParts)
  const e = formatCompactTime(endParts)
  const startText = s.meridiem === e.meridiem ? s.clock : `${s.clock} ${s.meridiem}`
  return `${startText}–${e.clock} ${e.meridiem}`
}

/**
 * A frozen air window this long or longer is corrupt data, not a radio slot —
 * real slots top out at a few hours. 12h (not 24h) so a wrong-day ends_at
 * can't produce a same-meridiem cross-day range that reads inverted
 * ("11–10 PM" for an 11 PM → next-day 10 PM window).
 */
const MAX_WINDOW_MS = 12 * 60 * 60 * 1000

/** Parse + validate a window; null for windowless/unparsable/degenerate. */
function parseWindow(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined
): { start: Date; end: Date } | null {
  if (!startsAt || !endsAt) return null
  const start = new Date(startsAt)
  const end = new Date(endsAt)
  if (isNaN(start.getTime()) || isNaN(end.getTime())) return null
  const span = end.getTime() - start.getTime()
  if (span <= 0 || span >= MAX_WINDOW_MS) return null
  return { start, end }
}

/**
 * True when the frozen air window is present, parseable, and non-degenerate —
 * the ONE definition of "trustworthy window" every consumer (time blocks,
 * viewer-local dates, the detail page's airs/airing/aired verb) must share,
 * so no surface can trust a window another surface rejects as corrupt.
 */
export function isValidWindow(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined
): boolean {
  return parseWindow(startsAt, endsAt) !== null
}

/**
 * Viewer-local air-time block (PSY-1298): "3–6 PM", "6:30–9 PM",
 * "9 PM–12 AM" — compact 12h, minutes only when non-zero, single AM/PM
 * suffix when both ends share it (a range crossing noon or midnight always
 * carries both, so "9–12 PM" is deliberately never produced — it would be
 * ambiguous). Returns '' for a windowless row (the date-only rendering is
 * the designed fallback), an unparsable window, or a degenerate one
 * (inverted / ≥12h) — corrupt data must not render as a confident range.
 */
export function formatLocalTimeRange(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined
): string {
  const window = parseWindow(startsAt, endsAt)
  if (!window) return ''
  return compactRangeText(
    { hours24: window.start.getHours(), minutes: window.start.getMinutes() },
    { hours24: window.end.getHours(), minutes: window.end.getMinutes() }
  )
}

/** Hour/minute of an instant in an arbitrary IANA zone, via Intl parts. */
function clockPartsInZone(date: Date, timeZone: string): ClockParts {
  const parts = new Intl.DateTimeFormat('en-US', {
    hour: 'numeric',
    minute: 'numeric',
    hour12: false,
    timeZone,
  }).formatToParts(date)
  const get = (type: string) =>
    Number(parts.find(p => p.type === type)?.value ?? NaN)
  // hour12:false can yield "24" for midnight in some engines — normalize.
  return { hours24: get('hour') % 24, minutes: get('minute') }
}

/**
 * The air window rendered in a specific IANA zone, with the zone's short name
 * appended: "9 AM–12 PM EDT" (PSY-1306 — the playlist detail page's
 * station-local aside). Same compact rules and degenerate-window guard as
 * formatLocalTimeRange; '' when the window is invalid, the zone is missing,
 * or the zone name isn't a valid IANA identifier.
 */
export function formatTimeRangeInZone(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined,
  timeZone: string | null | undefined
): string {
  const window = parseWindow(startsAt, endsAt)
  if (!window || !timeZone) return ''
  try {
    const range = compactRangeText(
      clockPartsInZone(window.start, timeZone),
      clockPartsInZone(window.end, timeZone)
    )
    const zonePart = new Intl.DateTimeFormat('en-US', {
      timeZone,
      timeZoneName: 'short',
    })
      .formatToParts(window.start)
      .find(p => p.type === 'timeZoneName')?.value
    return zonePart ? `${range} ${zonePart}` : range
  } catch {
    // Unrecognized IANA zone (bad stored value) — omit the aside entirely.
    return ''
  }
}

/**
 * The single-station identity sub-line: "Seattle, WA" / "London, UK" etc.
 * Drops empty parts; returns "" when no location is known.
 */
export function formatStationLocation(
  city: string | null | undefined,
  state: string | null | undefined
): string {
  return [city, state].filter(Boolean).join(', ')
}
