import { formatPrice } from '@/lib/utils/formatters'
import { showTimingInput } from '../utils'
import { startTimeFactSegment } from './showStatusStripeCopy'
import type { ShowResponse } from '../types'

/**
 * The ticket line's segments: `8PM · ON SALE · $35.00`, middot-joined by the
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
 * `ON SALE` is a claim, so it renders only while it can be true: somewhere
 * to buy (a ticket URL), not sold out, and not cancelled — the stripe says
 * CANCELLED at the top of the page and this line must not argue with it.
 * `SOLD OUT` swaps it per the mock. The past-show register flip (`NO LONGER
 * AVAILABLE`) belongs to the past-register ticket, not here. A DIY show with
 * no URL renders time + price and says nothing about sale state. The
 * door-price split (`DOOR $40 CASH`) has no schema — one `price` column —
 * so the single price is the whole statement.
 *
 * The age segment is a venue-less fallback: the venue module's facts line
 * owns the age fact, but a show with no venue row never mounts that module,
 * and losing "21+" from the page is a fact a reader plans around (the old
 * meta row printed it unconditionally).
 */
export function ticketLineSegments(show: ShowResponse): string[] {
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
  if (show.is_sold_out) {
    segments.push('SOLD OUT')
  } else if (show.ticket_url && !show.is_cancelled) {
    segments.push('ON SALE')
  }
  if (show.price != null) {
    segments.push(formatPrice(show.price))
  }
  const age = show.age_requirement?.trim()
  if (show.venues.length === 0 && age) {
    segments.push(age)
  }
  return segments
}
