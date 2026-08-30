/**
 * Sentry visibility for affiliate tags planted in contributor-submitted ticket
 * URLs.
 *
 * WHY THIS EXISTS
 *
 * `show.ticket_url` is open contribution that publishes without review, and
 * this app never writes an affiliate tag into the database: `ticketLink`
 * appends ours at render time and returns a string. So a tag found in a STORED
 * value was put there by whoever submitted the row, and today the only
 * consequence is that the page renders it with `rel="sponsored"` and declines
 * to add ours beside it. That is correct link hygiene and completely silent.
 * Nothing tells anyone the row exists, so a contributor monetizing our
 * traffic, or an automated probe testing whether we strip tags, produces no
 * signal at all.
 *
 * WHAT THIS IS AND IS NOT
 *
 * It is the OPERATOR-FACING half only: a warning, so somebody can look. The
 * link still renders exactly as stored; nothing here changes a href, blocks a
 * submission, or edits a row. The moderation-queue flag that lets an admin
 * actually clear one is a separate, filed follow-up.
 *
 * WHAT LEAVES THE PROCESS
 *
 * The parameter NAME and the HOST, and nothing else. Not the partner ID (an
 * account identifier belonging to a third party), not the path, query or
 * fragment (contributor text that can carry anything), and not the URL. The
 * entity type and id are what an operator needs to find the row, and they are
 * public identifiers already in the page's own URL. Sized so the server
 * `beforeSend` header scrub in `sentry.server.config.ts` has nothing of ours
 * left to filter.
 *
 * DEDUPING
 *
 * A planted tag is a property of a stored row, not of an event: every viewer
 * of that page, and every re-render, sees the same one. Reporting per
 * occurrence would turn one bad row into unbounded identical events. The key
 * is the row plus what was found, so a second distinct tag on the same show
 * still reports, and the set is bounded so a long-lived tab or server process
 * cannot grow it without limit.
 *
 * Module-level state means per-tab in the browser and per-instance on the
 * server, which is the right granularity for "has this operator-facing warning
 * already been raised in this process". A row that stays broken re-reports on
 * the next deploy or the next visitor's tab, which is the behaviour we want
 * from a signal that should not be ignorable.
 */

import * as Sentry from '@sentry/nextjs'
import type { PlantedTicketTag } from './ticketVendors'

/** Entities whose ticket URL this app renders as an outbound link. */
export type TicketTagEntityType = 'show' | 'festival'

export interface PlantedTagReport {
  entityType: TicketTagEntityType
  /** Public entity id, the same one in the page's own URL. */
  entityId: number | string
  tag: PlantedTicketTag
}

/**
 * Bounded so a pathological input space cannot grow this set without limit.
 * Past the cap reporting stops rather than leaking: the signal is "somebody
 * should look", and by the hundredth distinct planted tag that message has
 * been delivered.
 */
const MAX_REPORTED_TAGS = 200

const reported = new Set<string>()

function reportKey({ entityType, entityId, tag }: PlantedTagReport): string {
  return `${entityType}:${entityId}:${tag.host}:${tag.param}`
}

/**
 * Report a planted affiliate tag, once per row per process.
 *
 * Never throws and never returns a failure. This runs from a render effect on
 * a page whose actual job is showing a reader where to buy a ticket; a
 * telemetry fault must not become that page's problem, and there is nowhere
 * useful to report a failure of the reporting system.
 */
export function reportPlantedTicketTag(report: PlantedTagReport): void {
  try {
    reportPlantedTicketTagUnguarded(report)
  } catch {
    // Deliberately silent. See above.
  }
}

function reportPlantedTicketTagUnguarded(report: PlantedTagReport): void {
  const key = reportKey(report)
  if (reported.has(key)) return
  if (reported.size >= MAX_REPORTED_TAGS) return
  reported.add(key)

  const { entityType, entityId, tag } = report
  Sentry.captureMessage(
    'Affiliate tag planted in a contributor-submitted ticket URL',
    {
      level: 'warning',
      tags: {
        error_type: 'planted_affiliate_tag',
        entity_type: entityType,
        // Indexed so an operator can see at a glance whether one vendor is
        // being targeted, and so an alert can be scoped to a host.
        ticket_host: tag.host,
        affiliate_param: tag.param,
        runtime: typeof window === 'undefined' ? 'server' : 'browser',
      },
      extra: {
        entityId,
        // Stated rather than implied: whoever reads this event should know the
        // link was left alone on purpose, and that qualification already
        // happened, before deciding what to do about the row.
        renderedAs: 'unmodified, with rel="sponsored"',
      },
    }
  )
}

/** Test seam: clears the per-process dedupe set. */
export function resetPlantedTagReportsForTest(): void {
  reported.clear()
}
