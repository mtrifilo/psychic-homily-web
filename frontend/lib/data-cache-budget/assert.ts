import * as Sentry from '@sentry/nextjs'
import {
  DATA_CACHE_RAW_BUDGET_BYTES,
  DATA_CACHE_RAW_LIMIT_BYTES,
  encodedSize,
  formatMib,
  isWarnBandAllowlisted,
} from './budget'

/**
 * Thrown only during `next build`, and distinguished so the fail-open list
 * helpers can re-raise it instead of absorbing it. Nothing else should catch it.
 */
export class DataCacheBudgetError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'DataCacheBudgetError'
  }
}

/** Byte length of a response body, which is what the cap is applied to. */
export function byteLength(text: string): number {
  return new TextEncoder().encode(text).length
}

/**
 * The half of the budget gate that the disk scan cannot do.
 *
 * Next drops an over-cap entry WITHOUT writing it (see ./budget.ts), so
 * `.next/cache/fetch-cache` never contains the failure — only payloads that
 * still fit. Measuring the body at the fetch site is the only place the
 * oversized case is observable, so every cached list fetch calls this with the
 * body it just read.
 *
 * DURING `next build` THIS THROWS. An uncaught throw in a prerender fails the
 * build, which is the loud failure this ticket asked for: the deploy does not
 * promote and the previous deployment keeps serving. That is a deliberate
 * exception to `fetchSeoList`'s fail-open contract, and it is narrow — a build
 * is the one moment where an oversized payload is a defect to surface rather
 * than a blip to survive, because there is no reader waiting on the response.
 * Callers must therefore invoke this OUTSIDE their fail-open catch, or the
 * throw is swallowed and the gate silently does nothing.
 *
 * AT REQUEST TIME IT ONLY REPORTS. A page that renders is worth more than a
 * cache entry, and by then the deploy has already happened; Sentry is where a
 * catalogue that grew past the line after the last build shows up.
 *
 * Sizes are RAW bytes because that is what a caller has in hand. The cap is on
 * the base64 envelope, so the raw budget is the cap divided by 4/3.
 */
export function assertFetchFitsDataCache(url: string, rawBytes: number): void {
  if (rawBytes < DATA_CACHE_RAW_BUDGET_BYTES) return

  const overLimit = rawBytes > DATA_CACHE_RAW_LIMIT_BYTES
  // A recorded warn-band exception is reported but does not fail the build. The
  // HARD cap is never waived — an allowlisted URL that actually breaches is a
  // route that has stopped caching, which is the whole point of the gate.
  const allowlisted = !overLimit && isWarnBandAllowlisted(url)
  const message =
    `${url} responded with ${formatMib(rawBytes)} raw (~${formatMib(encodedSize(rawBytes))} base64), ` +
    (overLimit
      ? `over the ${formatMib(DATA_CACHE_RAW_LIMIT_BYTES)} raw budget for a 2 MB Data Cache item. ` +
        'Next will refuse to cache it and every render will re-pull it from origin.'
      : `within ${formatMib(DATA_CACHE_RAW_LIMIT_BYTES)} of the raw budget for a 2 MB Data Cache item, ` +
        'so it will stop being cached shortly, in silence.') +
    ' Shrink the payload — project it to the fields the caller reads, or shard the fetch.' +
    ' See lib/data-cache-budget/budget.ts.'

  if (isProductionBuild() && !allowlisted) {
    // Verified against next/dist/build/index.js, which assigns
    // PHASE_PRODUCTION_BUILD ('phase-production-build') before rendering.
    throw new DataCacheBudgetError(`Data Cache budget exceeded during build: ${message}`)
  }

  Sentry.captureMessage(`data-cache-budget: ${message}`, {
    level: overLimit ? 'error' : 'warning',
    tags: { service: 'data-cache-budget' },
    extra: { url, rawBytes, encodedBytes: encodedSize(rawBytes) },
  })
}

function isProductionBuild(): boolean {
  return process.env.NEXT_PHASE === 'phase-production-build'
}
