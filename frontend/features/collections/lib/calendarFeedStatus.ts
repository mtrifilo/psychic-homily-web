import { formatRelativeTime } from '@/lib/formatRelativeTime'
import type {
  CalendarFeedClient,
  CalendarFeedRead,
  CalendarFeedStatusResponse,
  CalendarLinkResetReason,
} from '@/features/shows/types'

/**
 * The saved-shows calendar's observed status: which calendar apps are checking
 * the link, which have stopped, and the sentences that report them. Pure, so
 * every surface that talks about the calendar (the panel, the Library header,
 * the home footer, Settings) reads one set of rules.
 *
 * Every status here is an OBSERVED check reported by the server, never a
 * guess from a click: an app the server has not seen is not connected.
 */

const HOUR_MS = 60 * 60 * 1000
const DAY_MS = 24 * HOUR_MS

/**
 * How long after its last check an app still counts as connected. Google
 * re-checks every few hours, so three days without one means it stopped; a Mac
 * can be set to refresh weekly, so Apple and unnamed apps get eight.
 */
const CALENDAR_CONNECTED_WINDOW_MS: Record<CalendarFeedClient, number> = {
  google: 3 * DAY_MS,
  apple_ios: 8 * DAY_MS,
  apple_mac: 8 * DAY_MS,
  other: 8 * DAY_MS,
}

/**
 * An unnamed app counts only once its checks span this long. Pasting the link
 * into a chat app triggers a single preview fetch, which must not read as a
 * calendar.
 */
const UNNAMED_APP_MIN_CHECK_SPAN_MS = HOUR_MS

/** What each client family is called wherever the status names it. */
export const CALENDAR_CLIENT_NAMES: Record<CalendarFeedClient, string> = {
  google: 'Google Calendar',
  apple_ios: 'Apple Calendar (iPhone or iPad)',
  apple_mac: 'Apple Calendar (Mac)',
  other: 'Another calendar app',
}

/**
 * The add action an app's checks come from. Both Apple families are added
 * through the one Apple Calendar row; an unnamed app has no row of its own.
 */
export type CalendarAddTarget = 'google' | 'apple'

export const CALENDAR_CLIENT_ADD_TARGET: Record<
  CalendarFeedClient,
  CalendarAddTarget | null
> = {
  google: 'google',
  apple_ios: 'apple',
  apple_mac: 'apple',
  other: null,
}

export interface CalendarApp {
  client: CalendarFeedClient
  name: string
  lastReadAt: string
}

export interface CalendarApps {
  /** Checked within its window, most recent first. */
  connected: CalendarApp[]
  /** Checked before, but not within its window, most recent first. */
  stopped: CalendarApp[]
}

function counts(read: CalendarFeedRead): boolean {
  if (read.client !== 'other') return true
  const span = Date.parse(read.last_read_at) - Date.parse(read.first_read_at)
  return span >= UNNAMED_APP_MIN_CHECK_SPAN_MS
}

/**
 * Sorts the status's reads into connected and stopped apps. An unnamed app
 * whose checks span less than an hour is neither: it was never a calendar.
 */
export function classifyCalendarApps(
  status: Pick<CalendarFeedStatusResponse, 'reads'> | undefined,
  now: Date
): CalendarApps {
  const apps: CalendarApps = { connected: [], stopped: [] }
  const reads = [...(status?.reads ?? [])].sort(
    (a, b) => Date.parse(b.last_read_at) - Date.parse(a.last_read_at)
  )
  for (const read of reads) {
    if (!counts(read)) continue
    const app: CalendarApp = {
      client: read.client,
      name: CALENDAR_CLIENT_NAMES[read.client],
      lastReadAt: read.last_read_at,
    }
    const age = now.getTime() - Date.parse(read.last_read_at)
    if (age <= CALENDAR_CONNECTED_WINDOW_MS[read.client]) {
      apps.connected.push(app)
    } else {
      apps.stopped.push(app)
    }
  }
  return apps
}

/**
 * Which security action reset a link a calendar app had been checking, or
 * null. The server sends the reason exactly when it sends the marker; a
 * marker without one is read as no reset rather than guessed at.
 */
export function calendarResetReason(
  status: CalendarFeedStatusResponse | undefined
): CalendarLinkResetReason | null {
  return status?.link_reset_at ? (status.link_reset_reason ?? null) : null
}

/**
 * The calendar's one-word state, for the header and footer labels: connected
 * when any app is; stopped when every known app stopped, or a security action
 * reset a link an app had been checking; otherwise none.
 */
export type CalendarSummary = 'none' | 'connected' | 'stopped'

export function summarizeCalendar(
  status: CalendarFeedStatusResponse | undefined,
  now: Date
): CalendarSummary {
  const { connected, stopped } = classifyCalendarApps(status, now)
  if (connected.length > 0) return 'connected'
  if (stopped.length > 0 || calendarResetReason(status)) return 'stopped'
  return 'none'
}

/**
 * The muted second line under a connected app: when it checked, and whether
 * that check came after the newest save, which is what decides whether that
 * save is in the calendar yet. `newestSavedAt` is null when the account has
 * no saved shows.
 */
export function calendarCheckedLine(
  lastReadAt: string,
  newestSavedAt: string | null,
  now: Date
): string {
  const ago = formatRelativeTime(lastReadAt, { short: true, now })
  if (newestSavedAt === null) return `Checked ${ago}.`
  if (Date.parse(lastReadAt) >= Date.parse(newestSavedAt)) {
    return `Checked ${ago}, since your last save.`
  }
  return `Checked ${ago}, before your last save. It appears at its next check.`
}

/** The line under a stopped app. */
export function calendarStoppedLine(app: CalendarApp, now: Date): string {
  const ago = formatRelativeTime(app.lastReadAt, { now })
  return `${app.name} last checked ${ago}. It may have been removed there.`
}
