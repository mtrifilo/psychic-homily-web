import { showIsArchived } from '@/lib/utils/showTiming'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'

/**
 * Whether a surface may say `SOLD OUT` about this show.
 *
 * `SOLD OUT` asserts two things at once — that the event is happening, and
 * that tickets are gone — so a cancellation withdraws it, and so does the
 * show being over. The show page says it in two places, the header badge and
 * the ticket line, and they must agree; a guard held by only one of them is a
 * guard the other walks past.
 *
 * Being over means {@link showIsArchived}, not `lifecycle === 'past'`: an
 * undateable show is `past` to the lifecycle by default, and withholding a
 * TRUE badge on that non-evidence would be its own bug. The stored flag owes
 * nothing to the calendar.
 *
 * NOT a global rule for the badge. Other surfaces render it straight from
 * `is_sold_out`; `grep is_sold_out` before assuming this governs them. Those
 * are listing rows and moderation chrome, which make a different claim than a
 * detail page's status band, and some have tests pinning their current
 * behaviour — bringing any of them under this predicate is a product decision
 * with its own tests to rewrite.
 *
 * Its own module with a structural input type so a caller that builds a
 * narrow show payload can import the rule without the shows feature's import
 * graph. Keep it to one type import and one runtime import.
 */
export function saysSoldOut(
  show: {
    is_sold_out: boolean
    is_cancelled: boolean
    event_date: string | null | undefined
  },
  lifecycle: ShowLifecycleState
): boolean {
  if (!show.is_sold_out || show.is_cancelled) return false
  return !showIsArchived(
    { eventDate: show.event_date, isCancelled: show.is_cancelled },
    lifecycle
  )
}
