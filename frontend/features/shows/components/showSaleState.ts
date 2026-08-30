import { showIsArchived } from '@/lib/utils/showTiming'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'

/**
 * Whether a surface may say `SOLD OUT` about this show.
 *
 * Its own module, and a STRUCTURAL input type rather than `ShowResponse`, so
 * the rule can be imported by a surface that does not carry the full show
 * payload — the share-card route builds a narrow `ShowData` of its own, and
 * it runs on the edge, where a feature module's import graph would be a real
 * cost. This file has one `import type` and one tiny runtime import for that
 * reason; keep it that way. It lives here rather than in `showTicketLine`
 * because the ticket line is only one of its callers, and a header badge
 * whose rule is filed under the ticket line is a rule nobody greps for.
 *
 * `SOLD OUT` asserts two things at once — that the event is happening, and
 * that tickets are gone. A cancellation withdraws it outright. Being over
 * withdraws it too, but only on the evidence {@link showIsArchived} accepts:
 * an UNDATEABLE show is `past` to the lifecycle by a default that is not
 * evidence of anything, and withholding a true badge on that basis would be
 * its own quiet bug — the badge stands, exactly as it did before this rule
 * existed.
 *
 * KNOWN SCOPE — this is NOT the only place the badge is drawn, and the list
 * below is what was audited when the rule was written, not a guarantee.
 * `grep -rn "is_sold_out" frontend` before assuming otherwise. Governed
 * today: the show page's ticket line, its header badge, and its
 * `opengraph-image`. NOT governed, and each is a separate decision rather
 * than an oversight:
 *
 * - `ShowStatusBadge` via `ShowCard` — the /shows list and the homepage list.
 * - The scene calendar / week / day views, which do render past days.
 * - `ShowBill`, rendered by the venue and artist PAST-SHOWS tables, which
 *   have pinned tests asserting the badge appears there.
 * - The admin consoles (`ShowSubmissionsConsole`, `ShowActions`), which
 *   hand-roll their own sold-out pill and share no code with
 *   `ShowStatusBadge` — the likeliest of these to drift, for that reason.
 *
 * The first three are LISTING rows, and a row is a different claim from a
 * detail page's status band; the pinned tests say the current behaviour is at
 * least deliberate. The admin pills are moderation chrome, which answers to
 * the stored flag rather than to a reader's register. Unifying any of them is
 * a product call with tests to rewrite, not a refactor to slip into an
 * unrelated change.
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
