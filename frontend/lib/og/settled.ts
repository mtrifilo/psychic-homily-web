import { isShowPast, type ShowTimingInput } from '@/lib/utils/showTiming'

/**
 * When a show's share card stops being able to change.
 *
 * Its own module, NOT part of `lib/og/response`, and that is a size decision
 * rather than a taste one. `response.tsx` is imported by `sceneWeekOgCard`,
 * which is the body of both scene OG routes, and those are Edge Functions.
 * `lib/og/brand.ts` records that route family shipping at 0.977 MiB against a
 * hard 1 MiB cap, with a prior revision REJECTED AT DEPLOY at 1.067 MiB and
 * nothing in `next build` warning about it. Putting this in `response.tsx`
 * would pull the whole `showTiming -> formatters -> timeUtils` chain into two
 * bundles that can never call it, for about 2 KB minified out of roughly 36 KiB
 * of remaining headroom. Only the show card imports this.
 */

/**
 * A day of margin between "the show is over" and "this card will never change
 * again".
 *
 * NOT a timezone allowance. Deriving the boundary in the venue's own zone
 * removed the need for that. It is a CACHE-INVALIDATION one. `ogCacheControl`
 * sets stale-while-revalidate equal to s-maxage, so committing a card to the
 * long window commits the CDN to it for up to twice that: an admin who cancels
 * a show, fixes its date, or finally attaches a flyer the morning after would
 * not reach an iMessage or Discord unfurl for two days. Shows are edited most
 * right after they happen, so the margin covers exactly that window.
 *
 * Carried over from the rule this replaced, which subtracted the same day for
 * a different stated reason. Changing the number is a product call about how
 * stale a share preview may be, not a refactor.
 */
const SETTLED_MARGIN_MS = 24 * 60 * 60 * 1000

/**
 * Whether a show's card can never change again, and so may cache hard.
 *
 * Exported rather than inlined at the route so it can be tested at all: the
 * brand fonts are route assets that do not resolve under vitest, so every card
 * rendered there is `degraded` and takes the short window before this branch is
 * reached. A copy of the rule living in a test would pin the copy, not the
 * route, so this is the one definition both use.
 *
 * Not behaviourally identical to the rule it replaced, which settled at START
 * + 24h in UTC. This settles at the end of the venue-local DAY + 24h: never
 * earlier, and up to a day later for a show that starts after midnight. The
 * cost of the difference is extra renders on the short window, not staleness.
 *
 * An unreadable `event_date` counts as settled here only because it cannot
 * reach this function: the route's `asShow` rejects one before rendering, and
 * a card that survives to be classified always has a parseable date. Do not
 * relax that guard without revisiting this, since a 48-hour freeze is the
 * expensive direction to be wrong in.
 */
export function isShowCardSettled(
  show: ShowTimingInput,
  now: Date = new Date()
): boolean {
  return isShowPast(show, new Date(now.getTime() - SETTLED_MARGIN_MS))
}
