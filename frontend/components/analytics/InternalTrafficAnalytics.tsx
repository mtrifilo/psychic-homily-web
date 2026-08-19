'use client'

import { Analytics } from '@vercel/analytics/react'
import { useEffect } from 'react'

/**
 * Vercel Web Analytics with an opt-in "internal traffic" suppressor (PSY-1546).
 *
 * Vercel Analytics is cookieless and has no identity concept, so it cannot
 * exclude the maintainer's own visits by account the way PostHog can (which
 * filters on the `is_admin` person property). Without this, every development
 * and smoke-test visit inflates the production numbers — which matters most
 * right now, when real traffic is small enough to be drowned out by it.
 *
 * A browser marks itself internal by loading any page with `?internal=1`
 * (`?internal=0` clears it). The choice persists in localStorage, so it is
 * per-browser-profile and must be set once on each device — deliberately
 * cheap to toggle, since devtools access isn't always convenient.
 *
 * Fails OPEN: if the flag can't be read (private mode, storage disabled, a
 * throwing localStorage), events are sent. Under-reporting real visitors is a
 * worse failure than counting a few of the maintainer's own.
 */

const INTERNAL_FLAG_KEY = 'ph-internal-traffic'
const INTERNAL_PARAM = 'internal'

/**
 * Hobby-plan Web Analytics includes 50k events/month and PAUSES collection
 * entirely once exceeded — so a single runaway client (an aggressive scraper
 * was observed at ~10k pageviews/day) can blind the instrument for everyone.
 * No genuine browsing session approaches 50 pages in a day; anything past the
 * cap is machine traffic we can afford to stop counting. Same fail-open
 * posture as the internal flag: if storage is unavailable, keep counting.
 */
const DAILY_PAGEVIEW_CAP = 50
const PAGEVIEW_COUNT_KEY = 'ph-pv-count'

/**
 * Spend one pageview from today's per-browser budget; returns false once the
 * budget is gone. The counter keys on the UTC date so it self-resets daily.
 * Never throws.
 */
export function pageviewWithinDailyCap(todayUtc: string): boolean {
  try {
    let count = 0
    const raw = window.localStorage.getItem(PAGEVIEW_COUNT_KEY)
    if (raw) {
      const stored: unknown = JSON.parse(raw)
      if (
        typeof stored === 'object' &&
        stored !== null &&
        (stored as { d?: unknown }).d === todayUtc &&
        typeof (stored as { n?: unknown }).n === 'number'
      ) {
        count = (stored as { n: number }).n
      }
    }
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

/** Read the flag without ever throwing — storage access can fail. */
function isInternalTraffic(): boolean {
  try {
    return window.localStorage.getItem(INTERNAL_FLAG_KEY) === '1'
  } catch {
    return false
  }
}

/** Apply `?internal=1` / `?internal=0` to the stored flag. Never throws. */
export function syncInternalFlagFromUrl(search: string): void {
  try {
    const value = new URLSearchParams(search).get(INTERNAL_PARAM)
    if (value === '1') window.localStorage.setItem(INTERNAL_FLAG_KEY, '1')
    else if (value === '0') window.localStorage.removeItem(INTERNAL_FLAG_KEY)
  } catch {
    // Storage unavailable — the flag simply can't be set on this browser.
  }
}

export default function InternalTrafficAnalytics() {
  useEffect(() => {
    syncInternalFlagFromUrl(window.location.search)
  }, [])

  return (
    <Analytics
      beforeSend={event => {
        if (isInternalTraffic()) return null
        if (!pageviewWithinDailyCap(new Date().toISOString().slice(0, 10)))
          return null
        return event
      }}
    />
  )
}
