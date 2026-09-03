/**
 * The I/O half of the monitor: gather what production is actually serving.
 *
 * Everything here talks to the network and returns raw observations; the
 * verdict is evaluate.ts's job.
 */

import {
  ALL_SHARD_IDS,
  ENTITY_SHARD_IDS,
  shardFamily,
  SITEMAP_FAMILIES,
  PAGES_SHARD_ID,
  type Family,
} from '@/app/sitemap-shards'
import type { MonitorConfig } from './config'
import {
  classifyLoc,
  detectShape,
  parseSitemapIndex,
  parseUrlset,
  showDateFromLoc,
  type LocBucket,
  type SitemapShape,
} from './parse'
import type { SampleResult } from './evaluate'

/** Everything read off the served sitemap, before any judgement. */
export interface SitemapObservation {
  shape: SitemapShape
  shardCount: number
  observedByFamily: Record<Family, number>
  observedPages: number
  observedOther: number
  /**
   * Every `<loc>` seen, rebased onto the target origin and grouped by bucket.
   *
   * Rebased for the same reason the shard URLs are, plus a second one: these
   * get fetched WITH the Vercel bypass header, so a `<loc>` naming a foreign
   * host would send that secret to it. Storing them target-anchored makes the
   * off-origin case unrepresentable rather than merely unlikely.
   *
   * Grouped rather than flat so the reachability sample can be STRATIFIED.
   * Drawing uniformly from the concatenation would make the probe a
   * releases-only check: releases is ~20k of the ~33k URLs, so a 10-URL sample
   * expects 0.44 shows and would probe the highest-churn families essentially
   * never.
   */
  locsByBucket: Map<LocBucket, string[]>
  /**
   * `<loc>` count per shard document, for the shards that were fetched and
   * parsed. A shard that errored is absent rather than zero: its failure is
   * already an error above, and reporting it as an empty document would blame
   * the sitemap for a transport problem.
   */
  observedByShard: Map<string, number>
  /** Slug dates of every show URL that carries one. */
  showDates: string[]
  errors: string[]
}

const USER_AGENT = 'psychic-homily-sitemap-monitor'

function emptyCounts(): Record<Family, number> {
  return Object.fromEntries(SITEMAP_FAMILIES.map(f => [f, 0])) as Record<Family, number>
}

/** How many hops to follow. The real chain is at most one (apex → www). */
const MAX_REDIRECTS = 5

/** Bodies larger than this are a compromised or wedged origin, not a sitemap. */
const MAX_BODY_BYTES = 128 * 1024 * 1024

/**
 * Headers for one request.
 *
 * The bypass token is attached ONLY when the request is going to the target
 * origin itself. It is a Vercel deployment-protection secret granting read
 * access to every SSO-gated preview, and the Fetch spec strips only
 * `Authorization`/`Cookie`/`Proxy-Authorization` across a cross-origin
 * redirect — a custom header like this one would be re-sent to whatever host
 * the redirect names. Scoping it here rather than at the call site means no
 * future caller can leak it by forgetting.
 */
function headersFor(url: string, config: MonitorConfig): Record<string, string> {
  // A plain UA: some edges serve bot-flavoured responses to the default fetch
  // agent, which would make the monitor measure something no crawler sees.
  const value: Record<string, string> = { 'user-agent': USER_AGENT }
  const targetOrigin = new URL(config.target).origin
  if (config.vercelBypassToken && new URL(url).origin === targetOrigin) {
    value['x-vercel-protection-bypass'] = config.vercelBypassToken
  }
  return value
}

function isRedirect(status: number): boolean {
  return status === 301 || status === 302 || status === 303 || status === 307 || status === 308
}

/**
 * GET a URL, following redirects MANUALLY so each hop re-derives its own
 * headers, and returning the final response whatever its status.
 *
 * Manual rather than `redirect: 'follow'` because production legitimately
 * redirects the apex to `www`, and the automatic follow would carry the bypass
 * token across that origin change. Following by hand is what lets `headersFor`
 * decide per hop.
 *
 * Non-2xx is NOT an error here: the reachability probe needs the status code
 * itself (a 404 is the finding, not an exception). `fetchDocument` applies the
 * ok-check for the callers that are parsing a body.
 */
async function requestFollowing(
  url: string,
  config: MonitorConfig,
  timeoutMs: number
): Promise<Response> {
  // ONE budget for the whole redirect chain, not one per hop. A per-hop signal
  // multiplies: 6 hops × 2 attempts × 30s is 6 minutes for a single document,
  // and only SHARD_FETCH_CONCURRENCY documents are in flight at a time, so that
  // blows the job's timeout-minutes. The runner then kills the process, main()'s
  // crash handler never runs, and NO alert is posted — the monitor goes silent
  // exactly when an origin is unhealthy, which is the failure class it exists
  // to eliminate.
  //
  // The shard count binds the same budget, divided by the pool: every
  // SHARD_FETCH_CONCURRENCY shards added to app/sitemap-shards.ts costs another
  // 65s of worst case, which is why .github/workflows/sitemap-freshness.yml
  // derives its ceiling from the count rather than picking a round number.
  // Check that budget when adding shards.
  const deadline = AbortSignal.timeout(timeoutMs)
  let current = url
  for (let hop = 0; hop <= MAX_REDIRECTS; hop++) {
    const response = await fetch(current, {
      headers: headersFor(current, config),
      redirect: 'manual',
      signal: deadline,
    })

    if (!isRedirect(response.status)) return response

    const location = response.headers.get('location')
    if (!location) {
      throw new Error(`GET ${url} returned ${response.status} with no Location header`)
    }
    await response.body?.cancel()
    current = new URL(location, current).toString()
  }
  throw new Error(`GET ${url} exceeded ${MAX_REDIRECTS} redirects`)
}

/**
 * Retry transport failures and 5xx once before giving up.
 *
 * The backend redeploys on a normal cadence, and a single 502 at 13:00 UTC
 * would otherwise post a "could not run" alert every time a release happened to
 * land on the cron. An alert that cries wolf on routine deploys gets muted, and
 * a muted alert is the end state this whole monitor exists to avoid.
 *
 * 4xx is NOT retried: a 404 on the entries feed is a real, stable answer, and
 * retrying it just doubles the time to the same verdict.
 */
async function withRetry<T>(attempt: () => Promise<T>, retryDelayMs: number): Promise<T> {
  try {
    return await attempt()
  } catch (error) {
    const message = (error as Error).message
    const isClientError = /returned 4\d\d/.test(message)
    if (isClientError) throw error
    await new Promise(resolve => setTimeout(resolve, retryDelayMs))
    return attempt()
  }
}

/**
 * Read a body, enforcing the cap on BYTES ACTUALLY READ.
 *
 * `content-length` alone is not enforcement: it is absent on any streamed
 * response — which is the normal case for a Next route handler, not an exotic
 * one — and `Number(null)` is 0, so a header check silently passes everything
 * it was meant to stop. It is kept only as a cheap fast-path for an origin
 * honest enough to declare an oversized body.
 */
async function readCapped(response: Response, url: string): Promise<string> {
  const declared = Number(response.headers.get('content-length'))
  if (Number.isFinite(declared) && declared > MAX_BODY_BYTES) {
    await response.body?.cancel()
    throw new Error(`GET ${url} declared ${declared} bytes, over the ${MAX_BODY_BYTES} cap`)
  }

  const reader = response.body?.getReader()
  if (!reader) return ''

  const chunks: Uint8Array[] = []
  let total = 0
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    total += value.byteLength
    if (total > MAX_BODY_BYTES) {
      await reader.cancel()
      throw new Error(`GET ${url} exceeded the ${MAX_BODY_BYTES} byte cap`)
    }
    chunks.push(value)
  }

  const joined = new Uint8Array(total)
  let offset = 0
  for (const chunk of chunks) {
    joined.set(chunk, offset)
    offset += chunk.byteLength
  }
  return new TextDecoder().decode(joined)
}

/**
 * Fetch one sitemap document as text, with retry.
 *
 * Fails on any non-2xx so a served error page never parses as data, and caps
 * the body so a wedged or hostile origin cannot OOM the runner.
 */
async function fetchDocument(url: string, config: MonitorConfig): Promise<string> {
  return withRetry(async () => {
    const response = await requestFollowing(url, config, config.fetchTimeoutMs)
    if (!response.ok) {
      await response.body?.cancel()
      throw new Error(`GET ${url} returned ${response.status}`)
    }
    return readCapped(response, url)
  }, config.retryDelayMs)
}

/**
 * Rewrite a child-document URL onto the origin being checked.
 *
 * NOT cosmetic. app/sitemap-index/route.ts hardcodes the production base URL in
 * every `<loc>`, so the index served by stage — or by a preview deployment —
 * lists `https://psychichomily.com/sitemap/artists.xml`. Following those links
 * verbatim would silently measure PRODUCTION while reporting the stage target,
 * which is worse than not checking at all. Verified against stage on
 * 2026-07-30.
 */
export function rebaseOnTarget(loc: string, target: string): string {
  const source = new URL(loc)
  const rebased = new URL(target)
  // Assigning the components rather than resolving `pathname` as a reference
  // string against the origin. A `<loc>` of `https://x/​/evil.com/a.xml` has a
  // pathname of `//evil.com/a.xml`, which URL resolution treats as
  // SCHEME-RELATIVE — `new URL('//evil.com/a', 'https://target')` yields
  // `https://evil.com/a`, walking the anchor straight off the target origin
  // and taking the bypass header with it.
  rebased.pathname = source.pathname
  rebased.search = source.search

  if (rebased.origin !== new URL(target).origin) {
    throw new Error(`refused to rebase "${loc}" off the target origin`)
  }
  return rebased.toString()
}

/**
 * How many shard documents are fetched at once.
 *
 * Six, not the shard count: the documents come from ONE origin that rate-limits
 * anonymous callers per IP, and this job also probes sampled URLs and asks the
 * API for a per-shard count. Six keeps a walk of a few dozen documents inside a
 * couple of minutes of wall clock while staying an order of magnitude under any
 * per-minute limit, and it does not have to move when the shard count does.
 */
export const SHARD_FETCH_CONCURRENCY = 6

/**
 * Run `worker` over `items` with at most `limit` in flight, returning results in
 * INPUT order.
 *
 * Order is the whole reason this is not `Promise.all(items.map(...))` plus a
 * chunked loop: the caller folds these into order-dependent accumulators, and a
 * chunked loop also stalls on the slowest member of each chunk. Workers pull
 * from a shared cursor instead, so a slow document delays only itself.
 *
 * `worker` must not throw — every result is kept, so a rejection here would
 * abandon the whole walk rather than one shard.
 */
async function mapWithConcurrency<T, R>(
  items: readonly T[],
  limit: number,
  worker: (item: T) => Promise<R>
): Promise<R[]> {
  const results = new Array<R>(items.length)
  let next = 0
  const pull = async (): Promise<void> => {
    for (;;) {
      const index = next++
      if (index >= items.length) return
      results[index] = await worker(items[index])
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, pull))
  return results
}

function shardIdFromUrl(url: string): string {
  const path = new URL(url).pathname
  return path.slice(path.lastIndexOf('/') + 1).replace(/\.xml$/, '')
}

function countInto(observation: SitemapObservation, bucket: LocBucket, count: number): void {
  if (bucket === PAGES_SHARD_ID) observation.observedPages += count
  else if (bucket === 'other') observation.observedOther += count
  else observation.observedByFamily[bucket] += count
}

/** Record a loc under its bucket, target-anchored, for stratified sampling. */
function recordLoc(
  observation: SitemapObservation,
  bucket: LocBucket,
  loc: string,
  target: string
): void {
  const anchored = rebaseOnTarget(loc, target)
  const existing = observation.locsByBucket.get(bucket)
  if (existing) existing.push(anchored)
  else observation.locsByBucket.set(bucket, [anchored])
}

function collectShowDates(locs: readonly string[], into: string[]): void {
  for (const loc of locs) {
    const date = showDateFromLoc(loc)
    if (date) into.push(date)
  }
}

/**
 * Read the sitemap the way a crawler would.
 *
 * Handles BOTH document shapes on purpose. Production served a single
 * `<urlset>` before the generateSitemaps() sharding shipped and a
 * `<sitemapindex>` after; a monitor that understood only one of them would be
 * broken across that deploy in one direction or the other, and "the monitor
 * was broken" is how the original defect survived. The shape actually served
 * is reported so a silent regression from index back to single-document is
 * visible rather than merely tolerated.
 *
 * When an index is served the shard id RESOLVES to a family through the shared
 * table, so the family counts cannot be skewed by a URL-classification gap.
 * classifyLoc is only used for the single-document shape. The id is no longer
 * the family itself — a sub-sharded family answers to several ids (PSY-1763) —
 * which is why the resolution goes through `shardFamily` rather than a cast.
 */
export async function walkSitemap(config: MonitorConfig): Promise<SitemapObservation> {
  const observation: SitemapObservation = {
    shape: 'urlset',
    shardCount: 0,
    observedByFamily: emptyCounts(),
    observedPages: 0,
    observedOther: 0,
    locsByBucket: new Map(),
    observedByShard: new Map(),
    showDates: [],
    errors: [],
  }

  const entryUrl = `${config.target}${config.entryPath}`
  const entryXml = await fetchDocument(entryUrl, config)
  observation.shape = detectShape(entryXml)

  if (observation.shape === 'urlset') {
    const locs = parseUrlset(entryXml)
    collectShowDates(locs, observation.showDates)
    for (const loc of locs) {
      const bucket = classifyLoc(loc)
      countInto(observation, bucket, 1)
      recordLoc(observation, bucket, loc, config.target)
    }
    return observation
  }

  const shardUrls = parseSitemapIndex(entryXml).map(loc => rebaseOnTarget(loc, config.target))
  observation.shardCount = shardUrls.length

  const known = new Set<string>(ALL_SHARD_IDS)
  const listed = new Set<string>()

  // Fetched with bounded concurrency, folded in index order.
  //
  // Serially, the worst case is a function of the shard count: one fetch
  // timeout plus a retry delay plus one more fetch is 65s per document from the
  // config defaults, so a few dozen documents is most of an hour, past the
  // GitHub job ceiling, at which point the runner kills the process, main()'s
  // crash handler never runs, and NO alert is posted. Raising the ceiling
  // instead would make time-to-alarm that long on the one run that matters, and
  // re-raise it on every future family split.
  //
  // BOUNDED rather than `Promise.all` over all of them: these hit one origin,
  // which rate-limits anonymous callers per IP, and a burst of every document
  // at once would be measuring the limiter rather than the sitemap.
  //
  // Folding is a SEPARATE, ordered pass over the results rather than done in
  // the workers, because `observation.errors`, `showDates` and `locsByBucket`
  // are order-dependent accumulators and a report whose contents depend on
  // which fetch happened to land first is not diffable between runs.
  const outcomes = await mapWithConcurrency(shardUrls, SHARD_FETCH_CONCURRENCY, async shardUrl => {
    const id = shardIdFromUrl(shardUrl)
    if (!known.has(id)) {
      return { id, error: `sitemap index lists unknown shard "${id}" (${shardUrl})` }
    }
    try {
      return { id, xml: await fetchDocument(shardUrl, config) }
    } catch (error) {
      return { id, error: `shard "${id}": ${(error as Error).message}` }
    }
  })

  for (const outcome of outcomes) {
    listed.add(outcome.id)
    if (outcome.error !== undefined) {
      observation.errors.push(outcome.error)
      continue
    }
    const { id, xml } = outcome
    // PARSING IS INSIDE THE PER-SHARD BOUNDARY, not only fetching. Two throws
    // below are reachable from a shard that answered 200 — `detectShape` on a
    // body that is not a sitemap document, and `new URL` inside `rebaseOnTarget`
    // on a `<loc>` that is not a URL — because `fetchDocument` checks the status
    // and nothing else. Letting either escape discards every other shard's
    // observations and reports a crash naming no shard at all.
    try {
      // A shard must be a urlset. An index here would mean nested sharding
      // this monitor does not understand, and counting it as zero URLs would
      // read as a catastrophically empty family.
      const shape = detectShape(xml)
      if (shape !== 'urlset') {
        observation.errors.push(`shard "${id}" served a <${shape}> where a <urlset> was expected`)
        continue
      }
      // The shard id is NOT the family once a family is sub-sharded: counting
      // `releases-a-e` as its own bucket would leave `releases` observed at
      // zero, which the evaluator reports as a vanished family — a false alarm
      // that looks exactly like the real incident. Resolved through the shared
      // table so a new sub-shard cannot reintroduce the confusion.
      //
      // NOT `?? 'other'`: `format.ts` prints the unclassified tally only for
      // the single-document shape, so an id that fell through would have its
      // URLs counted into a bucket the report never shows. An id that resolves
      // to no family here is a table inconsistency, not a URL to bucket — the
      // `known.has(id)` guard above should already have rejected it — so say so
      // and skip rather than swallow it.
      const family = id === PAGES_SHARD_ID ? PAGES_SHARD_ID : shardFamily(id)
      if (!family) {
        observation.errors.push(
          `shard "${id}" is listed and known but maps to no family — the shard table is inconsistent`
        )
        continue
      }
      const bucket: LocBucket = family
      const locs = parseUrlset(xml)
      // One at a time, not `push(...locs)`: the releases family is already ~20k
      // entries and spreading it into the argument list approaches the engine's
      // stack argument limit.
      for (const loc of locs) recordLoc(observation, bucket, loc, config.target)
      if (bucket === 'shows') collectShowDates(locs, observation.showDates)
      countInto(observation, bucket, locs.length)
      observation.observedByShard.set(id, locs.length)
    } catch (error) {
      observation.errors.push(`shard "${id}": ${(error as Error).message}`)
    }
  }

  // Every shard the sitemap claims to emit must actually be listed. A shard
  // silently dropped from the index is thousands of URLs vanishing with no
  // other signal — the incident's exact shape.
  //
  // Checked per SHARD rather than per family. What that buys is a NAMED,
  // immediate error instead of a drift number: a sub-sharded family that loses
  // one of its documents is still short only a fraction of its URLs, and
  // whether that fraction clears `driftRatio` is a coincidence of where the cut
  // points fell rather than something this check should depend on. Naming the
  // missing document also says which one, which a drift percentage cannot.
  for (const shardId of ENTITY_SHARD_IDS) {
    if (!listed.has(shardId)) {
      observation.errors.push(`sitemap index is missing the "${shardId}" shard`)
    }
  }

  return observation
}

/** What the projection feed says each document and each family should hold. */
export interface ExpectedCounts {
  /** Emitted-slug count per entity shard id. */
  byShard: Map<string, number>
  /** The same counts summed per family. */
  byFamily: Record<Family, number>
}

/**
 * Slug counts per SHARD, straight from the projection feed, plus the family
 * totals they sum to.
 *
 * PER SHARD, not per family, because a per-family count cannot see one document
 * of a sub-sharded family going dark: eight buckets carry an eighth of their
 * family each, which sits inside the default 20% drift tolerance, so the family
 * comparison would pass while thousands of URLs were missing. Asking the API
 * what each shard should hold turns that into a named, exact failure.
 *
 * One request per entity shard rather than one for every family at once. The
 * cost is 31 requests instead of 1 against an API that rate-limits anonymous
 * callers per IP, which is why they go through the same bounded pool the
 * document walk uses, and why .github/workflows/sitemap-freshness.yml counts
 * these waves in its timeout derivation.
 *
 * Failures are collected rather than thrown from inside the pool: a rejection
 * there would abandon in-flight requests and report whichever one lost the race,
 * so the throw happens once, after the fold, naming how many shards failed.
 */
export async function fetchExpectedCounts(config: MonitorConfig): Promise<ExpectedCounts> {
  const outcomes = await mapWithConcurrency(
    ENTITY_SHARD_IDS,
    SHARD_FETCH_CONCURRENCY,
    async (shardId): Promise<{ shardId: string; count?: number; error?: string }> => {
      const family = shardFamily(shardId)
      if (!family) {
        return {
          shardId,
          error: `shard "${shardId}" maps to no family — the shard table is inconsistent`,
        }
      }
      try {
        return { shardId, count: await fetchShardCount(shardId, family, config) }
      } catch (error) {
        return { shardId, error: (error as Error).message }
      }
    }
  )

  const byShard = new Map<string, number>()
  const byFamily = emptyCounts()
  const errors: string[] = []
  for (const outcome of outcomes) {
    if (outcome.count === undefined) {
      errors.push(outcome.error ?? `shard "${outcome.shardId}" returned no count`)
      continue
    }
    byShard.set(outcome.shardId, outcome.count)
    const family = shardFamily(outcome.shardId)
    if (family) byFamily[family] += outcome.count
  }

  // Fail the whole run rather than compare against a partial expectation. A
  // missing shard count would otherwise read as an expectation of zero, which
  // makes a served document look like over-coverage instead of an API problem.
  if (errors.length > 0) {
    throw new Error(
      `could not read expected counts for ${errors.length} of ${ENTITY_SHARD_IDS.length} shards: ${errors.join('; ')}`
    )
  }
  return { byShard, byFamily }
}

/** Emitted-slug count for one shard, from the family field it populates. */
async function fetchShardCount(
  shardId: string,
  family: Family,
  config: MonitorConfig
): Promise<number> {
  const url = `${config.apiBase}/sitemap/entries?family=${encodeURIComponent(shardId)}`
  // No bypass header: the API is a separate origin and never SSO-gated.
  const body: unknown = await withRetry(async () => {
    const response = await fetch(url, {
      headers: { 'user-agent': USER_AGENT },
      signal: AbortSignal.timeout(config.fetchTimeoutMs),
    })
    if (!response.ok) {
      throw new Error(`GET ${url} returned ${response.status}`)
    }
    return response.json()
  }, config.retryDelayMs)
  if (typeof body !== 'object' || body === null) {
    throw new Error(`GET ${url} did not return a JSON object`)
  }

  const rows = (body as Record<string, unknown>)[family]
  if (!Array.isArray(rows)) {
    // Coercing a missing family to 0 would report the shard as 100% drift and
    // blame the sitemap for an API-side problem. Fail on the real cause instead.
    throw new Error(`GET ${url} is missing the "${family}" family`)
  }
  // Count only rows the sitemap would actually emit: app/sitemap.ts drops
  // entries with an empty slug, so counting them here would manufacture drift
  // that does not exist.
  let emitted = 0
  for (const row of rows) {
    const slug = (row as { slug?: unknown } | null)?.slug
    if (typeof slug === 'string' && slug !== '') emitted++
  }
  return emitted
}

/**
 * Probe sampled URLs, catching the inverse failure: a sitemap that is the right
 * size but advertises URLs that 404.
 *
 * Redirects are followed and the FINAL status is what counts. The sitemap
 * emits apex-host URLs (`https://psychichomily.com/...`) while the site is
 * served from `www`, so every entry legitimately redirects once; asserting on
 * the first hop would fail every sample.
 *
 * Every URL is re-anchored to the target origin before the request, so the
 * bypass header cannot be sent to a host named by the fetched document.
 * `walkSitemap` already rebases; this is the enforcing boundary, and it fails
 * the sample rather than silently probing elsewhere. Even if the target itself
 * redirected off-origin, `headersFor` re-derives per hop, so the token is
 * dropped on the foreign hop rather than following it.
 */
export async function sampleUrls(
  urls: readonly string[],
  config: MonitorConfig
): Promise<SampleResult[]> {
  const targetOrigin = new URL(config.target).origin
  return Promise.all(
    urls.map(async url => {
      try {
        if (new URL(url).origin !== targetOrigin) {
          return {
            url,
            status: null,
            ok: false,
            error: `refusing to probe off-target origin (expected ${targetOrigin})`,
          }
        }
        // These probe dynamically-rendered pages, ten at once, at the cron
        // instant — so a backend mid-redeploy answers 502/503 and would post a
        // FAILED alert with no sitemap defect behind it.
        //
        // A 5xx is RETURNED, not thrown, so wrapping requestFollowing alone
        // would retry the transport error and silently skip the far more likely
        // case. Throw on 5xx inside the attempt to make it retryable, and turn
        // the final failure back into a result below. 4xx is deliberately left
        // alone: a 404 on a sitemap-advertised URL is the finding.
        const status = await withRetry(async () => {
          const response = await requestFollowing(url, config, config.sampleTimeoutMs)
          // Only the status matters; release the body rather than buffering a
          // full SSR page for each probe.
          await response.body?.cancel()
          if (response.status >= 500) {
            throw new Error(`GET ${url} returned ${response.status}`)
          }
          return response.status
        }, config.retryDelayMs)
        return { url, status, ok: status >= 200 && status < 300 }
      } catch (error) {
        return { url, status: null, ok: false, error: (error as Error).message }
      }
    })
  )
}
