/**
 * Daily per-browser pageview budget for Vercel Web Analytics.
 *
 * Exists because the Hobby plan includes a fixed monthly event allowance and
 * PAUSES collection entirely once it is exceeded: one runaway client can
 * blind the instrument for every real visitor for the rest of the month.
 * The cap only constrains clients that persist localStorage; that is
 * accepted, because the alternative failure (a persistent client burning the
 * whole allowance) is the one actually observed. If the account ever moves
 * to a plan without a hard event ceiling, this cap can be removed.
 *
 * Fail-open posture, same as the internal-traffic flag: when storage is
 * unavailable or holds garbage, keep counting. Under-counting real visitors
 * is a worse failure than over-counting.
 */

export const DAILY_PAGEVIEW_CAP = 50
export const PAGEVIEW_COUNT_KEY = 'ph-pv-count'

/**
 * Read today's spent budget. Anything unreadable, stale, or out of range
 * counts as a fresh day so the caller's write replaces the bad value instead
 * of preserving it.
 */
function readTodayCount(todayUtc: string): number {
  const raw = window.localStorage.getItem(PAGEVIEW_COUNT_KEY)
  if (!raw) return 0
  try {
    const stored: unknown = JSON.parse(raw)
    if (
      typeof stored === 'object' &&
      stored !== null &&
      (stored as { d?: unknown }).d === todayUtc
    ) {
      const n = (stored as { n?: unknown }).n
      if (
        typeof n === 'number' &&
        Number.isInteger(n) &&
        n >= 0 &&
        n <= DAILY_PAGEVIEW_CAP
      ) {
        return n
      }
    }
  } catch {
    // Unparseable: fall through to 0 so the value gets overwritten.
  }
  return 0
}

/**
 * Spend one pageview from today's per-browser budget; returns false once the
 * budget is gone. The counter keys on the UTC date so it self-resets daily.
 * Never throws.
 */
export function pageviewWithinDailyCap(todayUtc: string): boolean {
  try {
    const count = readTodayCount(todayUtc)
    if (count >= DAILY_PAGEVIEW_CAP) return false
    window.localStorage.setItem(
      PAGEVIEW_COUNT_KEY,
      JSON.stringify({ d: todayUtc, n: count + 1 })
    )
    return true
  } catch {
    return true
  }
}
