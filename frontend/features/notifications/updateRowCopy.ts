/**
 * One-line copy for a followed-entity update row: the compact form of the
 * inbox's alert rows, for surfaces that show a few of them in a ledger (the
 * signed-in home's "From your library", Figma `2185:1073`).
 *
 * Pure and ISOMORPHIC: no `'use client'`, no hooks, no clock of its own. The
 * verbs, fallback subjects and fallback destinations come from the same
 * `ALERT_ROW_FALLBACK` / `releaseDigestVerb` the inbox renders with.
 */

import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { resolveShowTimezone } from '@/lib/utils/formatters'
import { formatInTimezone } from '@/lib/utils/timeUtils'
import type { components } from '@/types/api'
import {
  ALERT_ROW_FALLBACK,
  isArtistReleaseDigestNotification,
  isArtistShowAlertNotification,
  isVenueShowAlertNotification,
  releaseDigestVerb,
  toAppPath,
} from './types'

/** One row of GET /me/notifications, as generated from the backend contract. */
export type UpdateRowEntry = components['schemas']['NotificationLogEntry']

export interface UpdateRowCopy {
  /** Kicker-tier kind label, sentence case; the row renders it uppercase. */
  label: string
  /** Subject · object. */
  title: string
  /** Date and recency. */
  detail: string
  href: string
}

/** "Fri Nov 20": weekday and month-day on the venue's clock. */
function formatShowDay(iso: string, timezone: string): string {
  const weekday = formatInTimezone(iso, timezone, { weekday: 'short' })
  return `${weekday} ${formatMonthDay(iso, timezone)}`
}

function formatMonthDay(iso: string, timezone: string): string {
  return formatInTimezone(iso, timezone, { month: 'short', day: 'numeric' })
}

/** "Nov 14 to Nov 22", or one day when the span is a single day. */
function formatDaySpan(first: string, last: string, timezone: string): string {
  const from = formatMonthDay(first, timezone)
  const to = formatMonthDay(last, timezone)
  return from === to ? from : `${from} to ${to}`
}

function joinParts(parts: ReadonlyArray<string | undefined | false>): string {
  return parts.filter(Boolean).join(' · ')
}

/**
 * The row's copy and destination, or `null` for a kind this row does not know.
 * Unknown kinds are skipped rather than rendered generically: a row that
 * cannot say what changed is noise in a five-row module.
 *
 * `now` is what recency is measured from. Every input is explicit (no clock
 * read, no runtime zone), so a server render and the hydrating client given
 * the same `now` produce the same text.
 *
 * A missing enrichment field degrades that half of the sentence, as the inbox
 * does; the row still renders, because the notification still happened.
 */
export function describeLibraryUpdate(
  entry: UpdateRowEntry,
  now: Date
): UpdateRowCopy | null {
  // An absent or unknown zone (the server omits one it would only guess)
  // falls to the app-wide default, the same on every runtime.
  const timezone = resolveShowTimezone(null, entry.alert_show_timezone)
  const ago = formatRelativeTime(entry.sent_at, { short: true, now, timeZone: timezone })

  if (isArtistShowAlertNotification(entry)) {
    return {
      label: 'New show',
      title: joinParts([
        entry.alert_artist_name || ALERT_ROW_FALLBACK.artistSubject,
        entry.alert_show_venue_name || entry.alert_show_title,
      ]),
      detail: joinParts([
        entry.alert_show_event_date &&
          formatShowDay(entry.alert_show_event_date, timezone),
        `announced ${ago}`,
      ]),
      href: toAppPath(entry.alert_show_url || ALERT_ROW_FALLBACK.showHref),
    }
  }

  if (isVenueShowAlertNotification(entry)) {
    // FULL batch size from the server. Absent when every member has since been
    // pulled, and then the label states no count rather than an invented one.
    const count = entry.alert_show_count
    return {
      label: !count ? 'New shows' : count === 1 ? '1 show' : `${count} shows`,
      title: joinParts([
        entry.alert_venue_name || ALERT_ROW_FALLBACK.venueSubject,
        count === 1 ? 'new show' : 'new shows',
      ]),
      detail: joinParts([
        entry.alert_show_first_date &&
          entry.alert_show_last_date &&
          formatDaySpan(
            entry.alert_show_first_date,
            entry.alert_show_last_date,
            timezone
          ),
        ago,
      ]),
      href: toAppPath(entry.alert_venue_url || ALERT_ROW_FALLBACK.venueHref),
    }
  }

  if (isArtistReleaseDigestNotification(entry)) {
    // One row per reader per week across every followed artist; its entity_id
    // is the READER's user id, never an artist or release id. The release
    // type is not on the row, so a one-release week is "Release", never a
    // guessed "New LP"; only that week has a single destination.
    const count = entry.alert_release_count
    return {
      label: !count ? 'Releases' : count === 1 ? 'Release' : `${count} releases`,
      title: entry.alert_release_artists || ALERT_ROW_FALLBACK.releaseSubject,
      detail: joinParts([releaseDigestVerb(count), ago]),
      href: toAppPath(entry.alert_release_url || ALERT_ROW_FALLBACK.releaseHref),
    }
  }

  return null
}
