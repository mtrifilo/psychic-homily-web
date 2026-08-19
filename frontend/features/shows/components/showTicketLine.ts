import { showTimingInput } from '../utils'
import { startTimeFactSegment } from './showStatusStripeCopy'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { ShowResponse } from '../types'

/**
 * The stored ticket URL repaired into something navigable, or null when
 * there is nothing to buy or no honest way to offer it.
 *
 * Null for a blank/whitespace value (storable: the backend persists the
 * field untrimmed and ingest paths skip the handler validator), and for a
 * cancelled or sold-out show — the line above this bracket says SOLD OUT or
 * the stripe says CANCELLED, and an enabled buy link under either is the
 * page arguing with itself. ONE derivation for "is there somewhere to buy",
 * shared by the segment logic and the component, so the two cannot drift.
 *
 * Submitters type scheme-less hosts ("tix.example/1") and vendors print
 * uppercase schemes; the scheme test is therefore case-insensitive and
 * anchored (`https?://`), not a bare prefix check — `startsWith('http')`
 * passed "httpfoo.example" through as a RELATIVE href that navigated under
 * /shows/. Protocol-relative values keep their own scheme resolution.
 */
export function ticketHref(show: ShowResponse): string | null {
  const raw = show.ticket_url?.trim()
  if (!raw || show.is_cancelled || show.is_sold_out) return null
  if (/^https?:\/\//i.test(raw)) return raw
  if (raw.startsWith('//')) return `https:${raw}`
  return `https://${raw}`
}

/**
 * The ticket line's price register: `$35`, `$12.50`, `Free`. Whole dollars
 * drop the cents — the locked mock's line reads `$35 ADV`, and `.00` on a
 * tabular mono line is noise. Local to this line on purpose; the app-wide
 * `formatPrice` keeps its two-decimal form for the surfaces built on it.
 */
function ticketPrice(price: number): string {
  if (price === 0) return 'Free'
  return Number.isInteger(price) ? `$${price}` : `$${price.toFixed(2)}`
}

/**
 * The ticket line's segments: `8PM · ON SALE · $35`, middot-joined by the
 * component.
 *
 * The start time leads (user decision): for the common show with no
 * announced doors/music the stripe prints a date alone, and this line is
 * then the only clock on the page — the mock's bare `ON SALE · $35 ADV`
 * assumed times the data mostly does not have yet. It renders through the
 * stripe's register and refusal rules ({@link startTimeFactSegment}): the
 * page has ONE clock spelling, and a venue whose timezone is a guess gets no
 * confidently-wrong hour here either.
 *
 * The sale state must never argue with the stripe, so its guards mirror the
 * stripe's own precedence: a CANCELLED show makes no sale claim at all
 * (cancellation outranks sold-out — "sold out" asserts the event is
 * happening); a PAST show makes none either (`ON SALE` is present tense,
 * and the archive is most of the corpus — the fuller past register, `NO
 * LONGER AVAILABLE`, belongs to the past-register ticket); `SOLD OUT` swaps
 * `ON SALE` per the mock; and `ON SALE` requires somewhere to actually buy
 * ({@link ticketHref}'s trimmed presence). The door-price split
 * (`DOOR $40 CASH`) has no schema — one `price` column — so the single
 * price is the whole statement.
 *
 * The age segment is a venue-less fallback: the venue module's facts line
 * owns the age fact, but a show with no venue row never mounts that module,
 * and losing "21+" from the page is a fact a reader plans around (the old
 * meta row printed it unconditionally).
 */
export function ticketLineSegments(
  show: ShowResponse,
  lifecycle: ShowLifecycleState
): string[] {
  const timing = showTimingInput(show)
  const segments: string[] = []
  const startTime = startTimeFactSegment({
    eventDate: show.event_date,
    state: timing.state,
    timezone: timing.timezone,
  })
  if (startTime) {
    segments.push(startTime)
  }
  if (!show.is_cancelled) {
    if (show.is_sold_out) {
      segments.push('SOLD OUT')
    } else if (show.ticket_url?.trim() && lifecycle !== 'past') {
      segments.push('ON SALE')
    }
  }
  if (show.price != null) {
    segments.push(ticketPrice(show.price))
  }
  const age = show.age_requirement?.trim()
  if (show.venues.length === 0 && age) {
    segments.push(age)
  }
  return segments
}
