import type { components } from '@/types/api'

/**
 * Scene-week types are DERIVED from the generated OpenAPI schema, not
 * hand-written.
 *
 * The rest of this feature predates `types/api.d.ts` and hand-writes its
 * interfaces, which is the exact drift hazard that has bitten this codebase
 * before: a hand-written type can claim a field the API never sends, and the
 * mismatch is invisible to CI while being fatal in production. New surfaces
 * derive from the schema so the `api:types:check` drift gate keeps them honest.
 */
export type SceneWeekResponse = components['schemas']['SceneWeekResponse']
export type SceneWeekDay = components['schemas']['SceneWeekDay']
export type SceneWeekShow = components['schemas']['SceneShowSummary']

/** ISO week key, e.g. `2026-W31`. Case-insensitive, zero-padded week. */
const ISO_WEEK_KEY = /^\d{4}-W\d{2}$/i

/**
 * Whether a URL segment even looks like an ISO week key.
 *
 * A cheap shape check only — it deliberately does NOT decide whether the week
 * exists. `2025-W53` is well-formed but not a real week (2025 has 52), and only
 * the backend, which owns the calendar maths and the scene's timezone, can say
 * so. This exists to avoid a pointless round-trip for obvious junk.
 */
export function looksLikeISOWeek(segment: string): boolean {
  return ISO_WEEK_KEY.test(segment.trim())
}

/**
 * Parse a `YYYY-MM-DD` date as LOCAL midnight.
 *
 * `new Date('2026-07-27')` parses as UTC midnight, which renders as Jul 26 in
 * any negative-offset timezone — so a US reader would see every day of the week
 * shifted back by one. The backend already resolved these dates in the scene's
 * own timezone; they are calendar dates, not instants, and must be built
 * component-wise to stay that way.
 */
function parseCalendarDate(iso: string): Date {
  const [y, m, d] = iso.split('-').map(Number)
  return new Date(y, (m ?? 1) - 1, d ?? 1)
}

/** `JUL 27` — the shared stem of every uppercase date label here. */
function monthDay(iso: string): string {
  const date = parseCalendarDate(iso)
  const month = date.toLocaleDateString('en-US', { month: 'short' })
  return `${month} ${date.getDate()}`.toUpperCase()
}

/** `MON JUL 27` — the day-group heading. */
export function formatDayHeading(iso: string): string {
  const weekday = parseCalendarDate(iso).toLocaleDateString('en-US', { weekday: 'short' })
  return `${weekday.toUpperCase()} ${monthDay(iso)}`
}

/**
 * `JUL 27 – AUG 2` — the share card's week range.
 *
 * Drops the weekday and year the page header carries: on a card the range sits
 * above the city at display scale and only has to say *which week*, and at the
 * 300px a link renders at in a group chat every dropped word buys size on the
 * words that remain.
 */
export function formatWeekRangeCompact(startISO: string, endISO: string): string {
  return `${monthDay(startISO)} – ${monthDay(endISO)}`
}

/** `Mon Jul 27 – Sun Aug 2, 2026` — the header's week range. */
export function formatWeekRange(startISO: string, endISO: string): string {
  const start = parseCalendarDate(startISO)
  const end = parseCalendarDate(endISO)
  const fmt = (d: Date) =>
    d.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
  return `${fmt(start)} – ${fmt(end)}, ${end.getFullYear()}`
}

/**
 * The line a reader actually scans.
 *
 * Most shows carry an empty `title` — display names are composed from the bill
 * everywhere else in the app — so artist names are the primary source and the
 * title is the fallback, not the other way round.
 */
export function showDisplayTitle(show: SceneWeekShow): string {
  const names = show.artist_names ?? []
  if (names.length > 0) return names.join(', ')
  if (show.title) return show.title
  return 'Live music'
}

/** Canonical `/shows/...` target; falls back to the id when a slug is missing. */
export function showHref(show: SceneWeekShow): string {
  return show.slug ? `/shows/${show.slug}` : `/shows/${show.id}`
}

/**
 * Total shows actually listed.
 *
 * Prefer the server's `show_count`, but fall back to counting: `days` is typed
 * nullable by the generator even though the API always emits an array, and a
 * header that disagrees with the list below it is worse than a recount.
 */
export function countShows(week: SceneWeekResponse): number {
  if (typeof week.show_count === 'number') return week.show_count
  return (week.days ?? []).reduce((n, day) => n + (day.shows?.length ?? 0), 0)
}
