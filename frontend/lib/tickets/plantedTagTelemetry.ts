/**
 * Sentry visibility for affiliate tags planted in contributor-submitted ticket
 * URLs.
 *
 * WHY THIS EXISTS
 *
 * `show.ticket_url` is open contribution that publishes without review, and
 * this app never writes an affiliate tag into the database: `ticketLink`
 * appends ours at render time and returns a string. So a tag found in a STORED
 * value was put there by whoever submitted the row. `ticketOffer` refuses to
 * render an outbound anchor for such a value, since the click would pay
 * whoever planted the tag. That refusal is correct and completely silent.
 * Nothing tells anyone the row exists, so a contributor trying to monetize
 * our traffic, or an automated probe testing whether we strip tags, produces
 * no signal at all.
 *
 * WHAT THIS IS AND IS NOT
 *
 * It is the OPERATOR-FACING half only: a warning, so somebody can look.
 * Nothing here changes a href, blocks a submission, or edits a row. The
 * moderation-queue flag that lets an admin actually clear one is a separate,
 * filed follow-up.
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
 * DEDUPING, AND WHY IT IS NOT JUST A MODULE-LEVEL SET
 *
 * A planted tag is a property of a stored row, not of an event: every viewer
 * of that page, and every re-render, sees the same one. Reporting per
 * occurrence would turn one bad row into unbounded identical events.
 *
 * The trigger is attacker-chosen, which rules out the obvious implementation.
 * Any authenticated contributor can store `?irmp=1` on a show that publishes
 * without review, so this is the first client event class in the app whose
 * firing condition an untrusted user controls. A module-level `Set` alone is
 * per-DOCUMENT, so a scripted reload loop resets it and mints one event per
 * request. Three bounds, because none of them is sufficient alone:
 *
 *   1. `sessionStorage`, which SURVIVES reloads for the origin, so the reload
 *      loop collapses to one event per row per tab. Falls back to memory
 *      wherever it is unavailable (server rendering, private-mode quota
 *      failures) rather than failing the report.
 *   2. A module-level set, which covers the fallback and is the fast path.
 *   3. A GLOBAL per-process ceiling. The keyed bounds are all keyed on
 *      dimensions the attacker picks, so "200 distinct keys" is 200 events on
 *      demand; this one is not keyed on anything and is the real backstop.
 *
 * Replay flushing is excluded for this event class in `instrumentation-replay`
 * — see the note there. Without it, planting a tag chooses which visitors get
 * session-recorded.
 *
 * The right long-term home for this detection is a backend sweep over stored
 * `ticket_url`s feeding the moderation queue: the finding is a property of a
 * row and does not need one report per pageview. That is the filed follow-up;
 * this module should retire when it lands.
 */

import * as Sentry from '@sentry/nextjs'
import type { PlantedTicketTag } from './ticketVendors'

/** Entities whose ticket URL this app resolves for rendering. */
export type TicketTagEntityType = 'show' | 'festival'

export interface PlantedTagReport {
  entityType: TicketTagEntityType
  /** Public entity id, the same one in the page's own URL. */
  entityId: number | string
  tag: PlantedTicketTag
}

/**
 * Distinct rows remembered in memory. Bounds the set itself; it does NOT bound
 * the event count, because the key includes attacker-chosen fields.
 */
const MAX_REMEMBERED_TAGS = 200

/**
 * Hard ceiling on events from this module per process, keyed on nothing.
 *
 * The one bound an attacker cannot widen by varying the host, the parameter or
 * the row. Low on purpose: the message is "somebody should look at planted
 * ticket tags", and it does not become truer the twentieth time. Anything past
 * this is volume, and volume belongs in the moderation queue.
 */
const MAX_REPORTS_PER_PROCESS = 20

/**
 * Fraction of otherwise-eligible reports actually sent.
 *
 * The bounds above are per-document and per-session, so volume still scales
 * with UNIQUE VISITORS to a page carrying a planted tag — a popular show is
 * thousands of identical warnings a day, and a contributor picks which show.
 * Sampling makes the cost scale with a rate we choose instead.
 *
 * It does not lose the signal, because the question is "does any page have a
 * planted tag", asked repeatedly by independent visitors: any page with ~60
 * views a day surfaces within a day at better than 95%. A page too quiet for
 * that is also a page nobody is being redirected from at volume.
 *
 * Rolled AFTER the dedupe checks and only recorded when a report is actually
 * sent, so a page that loses the roll simply asks again on the next visit.
 */
const REPORT_SAMPLE_RATE = 0.05

const remembered = new Set<string>()
let reportsSent = 0

const STORAGE_PREFIX = 'ph:planted-tag:'

function reportKey({ entityType, entityId, tag }: PlantedTagReport): string {
  return `${entityType}:${entityId}:${tag.host}:${tag.param}`
}

/**
 * Whether this row has already been reported from this browser session.
 *
 * `sessionStorage` rather than memory because a reload resets the module but
 * not the tab, and a reload loop is the cheapest way to turn this signal into
 * a quota bill. Every access is guarded: it throws on a disabled or full
 * store, and does not exist at all during server rendering.
 */
function alreadyReportedInSession(key: string): boolean {
  try {
    if (typeof sessionStorage === 'undefined') return false
    return sessionStorage.getItem(STORAGE_PREFIX + key) !== null
  } catch {
    return false
  }
}

function rememberInSession(key: string): void {
  try {
    if (typeof sessionStorage === 'undefined') return
    sessionStorage.setItem(STORAGE_PREFIX + key, '1')
  } catch {
    // A disabled or full store just means the in-memory bounds carry it.
  }
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
  if (reportsSent >= MAX_REPORTS_PER_PROCESS) return

  const key = reportKey(report)
  if (remembered.has(key)) return
  if (alreadyReportedInSession(key)) return
  if (Math.random() >= REPORT_SAMPLE_RATE) return

  if (remembered.size < MAX_REMEMBERED_TAGS) remembered.add(key)
  rememberInSession(key)
  reportsSent += 1

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
        // The filter that keeps the benign case (our own rendered link copied
        // back in) from burying somebody redirecting our commission.
        matches_configured_partner: tag.matchesConfiguredPartner,
        runtime: typeof window === 'undefined' ? 'server' : 'browser',
      },
      extra: {
        entityId,
        // Stated rather than implied: whoever reads this event should know
        // the stored value was left alone on purpose, and that the page
        // renders no outbound anchor for it, before deciding what to do about
        // the row. A free-admission exemption cannot override this.
        renderedAs: 'stored value unmodified; no outbound anchor rendered',
      },
    }
  )
}
