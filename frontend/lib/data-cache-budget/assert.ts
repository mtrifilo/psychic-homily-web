import * as Sentry from '@sentry/nextjs'
import { appendFileSync, mkdirSync } from 'node:fs'
import { dirname } from 'node:path'
import {
  breachLogPath,
  DATA_CACHE_RAW_BUDGET_BYTES,
  DATA_CACHE_RAW_LIMIT_BYTES,
  encodedSize,
  formatMiB,
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

/**
 * Read a cached fetch's body as JSON, weighing it against the budget on the way
 * through. THE ONLY WAY A CALL SITE SHOULD CONSUME A CACHED RESPONSE.
 *
 * It exists so the gate is not a three-step ritual a future caller can get
 * partly right: reading as text, measuring before parsing, and measuring
 * OUTSIDE the caller's fail-open catch are all done here. A helper that keeps
 * using `res.json()` opts out of the gate visibly rather than silently.
 *
 * Callers must still let `DataCacheBudgetError` escape their own catch — see
 * the note on `assertFetchFitsDataCache`.
 */
export async function readJsonWithinDataCacheBudget<T>(
  url: string,
  res: Response
): Promise<T> {
  const text = await res.text()

  // Measuring exactly costs a pass over the body, so it is skipped for the
  // ~everything that is nowhere near the line: UTF-8 never exceeds 3 bytes per
  // UTF-16 code unit, making `length * 3` a safe upper bound that costs a
  // property read. `Buffer.byteLength` over `TextEncoder` for the real
  // measurement — same number, without allocating a copy of the body. Every
  // call site is Node runtime (the only `runtime = 'edge'` routes are the OG
  // images, which do not import this module).
  //
  // Content-Length is NOT usable here: the API gzips compressible bodies, so
  // the header carries the compressed transfer size, several-fold under the
  // number the cap is applied to. Weighing it would disarm the gate.
  if (text.length * 3 >= DATA_CACHE_RAW_BUDGET_BYTES) {
    assertFetchFitsDataCache(url, Buffer.byteLength(text, 'utf8'))
  }

  return JSON.parse(text) as T
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
 * HOW A BREACH ACTUALLY STOPS A DEPLOY — measured on Next 16.1.4 with
 * `cacheComponents: true`, by forcing a throw at each call site and reading the
 * exit code. The two call-site kinds do NOT behave the same, and the difference
 * is the reason for the breach log below:
 *
 *   call site                       next build   what stops the deploy
 *   ------------------------------  -----------  ----------------------------
 *   page route (fetchSeoList /       EXIT 1      the throw itself; the error
 *   fetchListPayload, e.g.                       names the URL and the size
 *   app/artists/page.tsx)
 *
 *   metadata route (app/sitemap.ts   EXIT 0      NOT the throw. The shard
 *   via fetchShard)                      degrades to Dynamic with no
 *                                                prerendered body and the build
 *                                                still succeeds.
 *
 * The second row is why an earlier draft of this comment was wrong to claim a
 * throw always fails the build: on the metadata route it does not. Before the
 * breach log, a `releases` breach would have surfaced as
 * `lib/sitemap-prerender/cli.ts` failing with a message about BACKEND OUTAGES —
 * a red build pointing at the wrong subsystem entirely. That was measured, not
 * reasoned about; `lib/sitemap-prerender/check.ts` records the same exit-0
 * behaviour for this route from the other direction.
 *
 * So a breach is ALSO appended to a log that `./cli.ts` reads and fails on,
 * which makes the outcome uniform across call sites and keeps the diagnosis
 * attached to the real cause. The throw stays because on a page route it is the
 * earliest, clearest failure available.
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
    `${url} responded with ${formatMiB(rawBytes)} raw (~${formatMiB(encodedSize(rawBytes))} base64), ` +
    (overLimit
      ? `over the ${formatMiB(DATA_CACHE_RAW_LIMIT_BYTES)} raw budget for a 2 MB Data Cache item. ` +
        'Next will refuse to cache it and every render will re-pull it from origin.'
      : `inside ${formatMiB(DATA_CACHE_RAW_LIMIT_BYTES)}, the raw budget for a 2 MB Data Cache item, ` +
        'but close enough that ordinary growth will cross it — after which it stops ' +
        'being cached and nothing fails.') +
    ' Shrink the payload — project it to the fields the caller reads, or shard the fetch.' +
    ' See lib/data-cache-budget/budget.ts.'

  if (!isProductionBuild()) {
    Sentry.captureMessage(`data-cache-budget: ${message}`, {
      level: overLimit ? 'error' : 'warning',
      tags: { service: 'data-cache-budget' },
      extra: { url, rawBytes, encodedBytes: encodedSize(rawBytes) },
    })
    return
  }

  // A build's signal channel is stdout, not an unflushed Sentry transport:
  // nothing calls Sentry.flush() and `next build` exits immediately, so an
  // event queued here would simply be lost.
  console.warn(`Data Cache budget ${overLimit ? 'BREACH' : 'WARNING'}: ${message}`)

  if (allowlisted) return

  recordBreach(url, rawBytes)

  if (enforcementDisabled()) {
    console.warn(
      'Data Cache budget enforcement is DISABLED for this build ' +
        '(DATA_CACHE_BUDGET_ENFORCE=warn). The payload above is not cached, and every ' +
        'render re-pulls it from origin. This is a break-glass, not a fix.'
    )
    return
  }

  throw new DataCacheBudgetError(
    `Data Cache budget exceeded during build: ${message}` +
      ' To ship an unrelated change right now, DATA_CACHE_BUDGET_ENFORCE=warn overrides' +
      ' this — at the cost of shipping a route that is not cached.'
  )
}

/**
 * The break-glass.
 *
 * Without one, a breach is triggered by DATA rather than code — a release
 * import nobody shipped a commit for — and blocks EVERY frontend deploy,
 * including an unrelated hotfix, until a payload-shrinking change lands. That
 * inverts the severity: the condition being blocked on is a route that stopped
 * being cached, which is a performance regression, not an outage.
 *
 * Deliberately awkward and deliberately loud. It is set per-build, never
 * committed, and it prints on every affected fetch plus a summary from
 * ./cli.ts, so a build that used it cannot be mistaken for a clean one. The
 * design goal is that a breach is impossible to MISS, which a loud failure
 * serves; it does not require a failure that is impossible to OVERRIDE.
 *
 * Deliberately NOT modelled on lib/sitemap-prerender/cli.ts, which has no
 * opt-out on purpose. That gate guards a sitemap that would 500 to crawlers;
 * this one guards a cache entry. Different blast radius, different policy.
 */
function enforcementDisabled(): boolean {
  return process.env.DATA_CACHE_BUDGET_ENFORCE === 'warn'
}

/**
 * Append a breach so ./cli.ts can fail on it with the right message even when
 * the throw did not stop the build (the metadata-route row above).
 *
 * Best-effort: a failure to write must not mask the breach itself, and on a
 * page route the throw is about to fail the build anyway.
 *
 * NOT under vitest. Unit tests drive this function with `NEXT_PHASE` set to the
 * build phase, and without this guard each run left fabricated records in the
 * real log — enough that the documented standalone `bun run
 * lib/data-cache-budget/cli.ts` then failed reporting breaches no build
 * produced, including the very `/artists` payload this change fixes. A test
 * must not be able to forge input to the channel the metadata-route half of the
 * gate depends on.
 */
function recordBreach(url: string, rawBytes: number): void {
  if (process.env.VITEST) return
  try {
    const path = breachLogPath()
    mkdirSync(dirname(path), { recursive: true })
    appendFileSync(path, `${JSON.stringify({ url, rawBytes })}\n`)
  } catch {
    // Intentionally ignored; see above.
  }
}

function isProductionBuild(): boolean {
  return process.env.NEXT_PHASE === 'phase-production-build'
}
