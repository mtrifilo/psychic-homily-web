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
 * Earliest week key worth serving. The site has no shows before this, and an
 * unbounded key space is an unbounded set of distinct URLs — each one a cache
 * miss that renders a full share card. Bounding it keeps that finite.
 */
const FIRST_TRACKED_YEAR = 2015

/**
 * Whether a URL segment even looks like an ISO week key.
 *
 * A cheap shape check only — it deliberately does NOT decide whether the week
 * exists. `2025-W53` is well-formed but not a real week (2025 has 52), and only
 * the backend, which owns the calendar maths and the scene's timezone, can say
 * so. This exists to avoid a pointless round-trip for obvious junk.
 *
 * Matches the RAW segment, deliberately. Trimming here would accept forms the
 * proxy's copy of this rule rejects, so a page and its share card would
 * disagree about which URLs are legal — and every accepted variant of the same
 * week (leading space, tab, BOM) is another distinct URL that renders its own
 * card. One spelling per week.
 */
export function looksLikeISOWeek(segment: string): boolean {
  const match = ISO_WEEK_KEY.exec(segment)
  if (!match) return false
  const year = Number(segment.slice(0, 4))
  return year >= FIRST_TRACKED_YEAR && year <= new Date().getUTCFullYear() + 1
}

/**
 * Resolve a requested week segment into what should actually be fetched.
 *
 * Three outcomes, and the difference between the last two is the whole point:
 * - `undefined` — no segment supplied; the caller wants the CURRENT week.
 * - a normalized key — a well-formed week to fetch.
 * - `null` — the segment is junk and NOTHING should be fetched.
 *
 * The archived route's segment is dynamic, so it also catches any unmatched
 * child path under a scene. Collapsing junk to `undefined` would quietly serve
 * the current week for a URL whose page 404s — handing out a confident-looking
 * answer to a question nobody asked. The key is returned exactly as validated,
 * so the string that was checked is the string that gets used.
 */
export function resolveRequestedWeek(segment: string | undefined): string | undefined | null {
  if (segment === undefined) return undefined
  return looksLikeISOWeek(segment) ? segment : null
}

/**
 * A show count as a sentence, optionally suffixed "this week".
 *
 * WHICH week is the CALLER's to know — this function only spells the phrase.
 * Every caller now passes a Monday-to-Sunday total in the scene's own timezone:
 * the share card reads it off the week payload, the scene cards and the
 * `/shows` by-city index read `shows_calendar_week` from `GET /scenes`. The
 * rolling `shows_this_week` is a different seven days and must not be spelled
 * with these words next to a link to a week page.
 *
 * The suffix is dropped for an archived share card, which carries its date
 * range directly above this line: that reads correctly for a week shared months
 * later rather than claiming to be current. A quiet week says so in words —
 * `0 shows` is a bad thing to post.
 */
export function formatShowCountLine(total: number, isCurrentWeek: boolean): string {
  const period = isCurrentWeek ? ' this week' : ''
  if (total === 0) return `No shows${period}`
  return `${total} ${total === 1 ? 'show' : 'shows'}${period}`
}

/**
 * Parse a `YYYY-MM-DD` date as LOCAL midnight.
 *
 * `new Date('2026-07-27')` parses as UTC midnight, which renders as Jul 26 in
 * any negative-offset timezone — so a US reader would see every day of the week
 * shifted back by one. The backend already resolved these dates in the scene's
 * own timezone; they are calendar dates, not instants, and must be built
 * component-wise to stay that way.
 *
 * Exported for the day surface, which parses the same scene-local ISO dates and
 * must not grow a second copy of this rule — a page and its sibling disagreeing
 * about what `2026-07-31` means is exactly the bug this guards.
 */
export function parseCalendarDate(iso: string): Date {
  const [y, m, d] = iso.split('-').map(Number)
  return new Date(y, (m ?? 1) - 1, d ?? 1)
}

/**
 * The zone the cross-city week index names its week in.
 *
 * It names a heading, and that is a different question from resolving
 * "upcoming", which is decided per show against its own venue's zone and needs
 * no shared viewer zone at all (PSY-1678). There is no per-row zone to move
 * this to: the heading sits above every row at once.
 *
 * A single zone is a compromise here, not a correct answer. The per-row counts
 * are each scene's own calendar week, exact against the page the row links to
 * (`shows_calendar_week`, PSY-1623); this heading is not, because the approved
 * mock puts one range above every row. The scene-week pages resolve "current
 * week" in each scene's OWN venue timezone, so for a few hours after Monday
 * midnight a scene east of this zone has already turned over while this label
 * still names the previous week — its count and its destination agree with each
 * other, and only the shared heading above them is stale.
 *
 * It is not the block's only approximation: the counts also reach the page
 * through a cache while this label is derived from a live clock, so they can lag
 * it at the rollover (see `ThisWeekByCity`).
 *
 * Fixing either properly means per-row ranges, which is a design change rather
 * than a data one: `GET /scenes` would also have to publish each scene's bounds,
 * and the heading would be read off the payload instead of the clock.
 */
export const SCENE_WEEK_INDEX_TIMEZONE = 'America/Los_Angeles'

/** `2026-07-27` from a `Date` read in its own local fields. */
function toCalendarDate(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

/**
 * The Monday and Sunday bounding the week that CONTAINS `now` in `timeZone`.
 *
 * The scene-week pages get their bounds from the backend, which resolves them
 * in each scene's own zone. Nothing carries those bounds for a list of scenes,
 * so a surface that names one week across many cities has to derive it — and
 * derive it in a stated zone, because "which week is it" changes at a different
 * instant in Phoenix than in London.
 *
 * `formatToParts` reads the year, month and day the zone is currently on
 * without any date maths, and without depending on how a locale happens to
 * order a short date (`lib/utils/timeUtils.ts` and `lib/utils/showTiming.ts`
 * take the same route to the same question). The rest is plain local-field
 * arithmetic on a local-midnight `Date`, which `Date` normalizes across month
 * and year ends for us.
 *
 * `now` is a parameter rather than a `new Date()` inside, so tests can pin a
 * week and so the caller is forced to own reading the clock. Under
 * `cacheComponents` that read is only legal at request time.
 */
export function currentWeekBounds(
  now: Date,
  timeZone: string,
): { start: string; end: string } {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(now)
  const field = (type: string) => Number(parts.find(p => p.type === type)?.value)

  const today = new Date(field('year'), field('month') - 1, field('day'))
  // `getDay()` is Sunday-first; the week here is Monday-first, matching the ISO
  // week the `/scenes/{slug}/week` pages serve.
  const daysSinceMonday = (today.getDay() + 6) % 7
  const start = new Date(
    today.getFullYear(),
    today.getMonth(),
    today.getDate() - daysSinceMonday,
  )
  const end = new Date(
    start.getFullYear(),
    start.getMonth(),
    start.getDate() + 6,
  )
  return { start: toCalendarDate(start), end: toCalendarDate(end) }
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
 *
 * Shared with the day surface, which lists the same rows: the two pages must
 * name a show identically or a reader following one to the other sees it
 * rename itself.
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
