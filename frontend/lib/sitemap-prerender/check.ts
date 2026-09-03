/**
 * The policy half of the post-build sitemap prerender gate.
 *
 * WHY THIS EXISTS — measured on Next 16.1.4 with `cacheComponents: true`, by
 * `next build` + `next start` + curl, backend killed at each stage:
 *
 *   build backend | build cache | shards prerendered | serving with backend down
 *   --------------|-------------|--------------------|--------------------------
 *   reachable     | clean       | all                | 200, previous document
 *   reachable     | clean       | all                | 200 even with the Data
 *                 |             |                    | Cache purged first
 *   UNREACHABLE   | clean       | 1 (pages) of all   | 500 on every entity shard
 *   UNREACHABLE   | warm        | all                | 200, previous document
 *
 * Counts are written as "all" rather than a number on purpose: the shard count
 * is a property of app/sitemap-shards.ts and has changed with every sharding
 * decision (10 families, then 14, then 39, and 32 once the three over-cap
 * families moved to primary-key buckets), while the BEHAVIOUR each row
 * describes did not. Rows 1 and 3 were measured at 14 shards on PSY-1763,
 * again at 39 on PSY-2018, and again at 32 on PSY-2019: `32 of 32 shards have
 * a fallback document` against a healthy backend, and `31 of 32 shards have no
 * fallback document` with the backend unreachable, exit 1.
 *
 * The stale-serving fallback the sitemap needs is the PRERENDERED BODY that
 * `next build` writes to disk. It ships inside the deployment, so it survives a
 * backend outage and it does not depend on the fetch Data Cache being warm —
 * both measured above. Nothing needs adding to the route to get it.
 *
 * The hole is row 3. A build whose `/sitemap/entries` fetch fails silently
 * drops every entity family to a Dynamic route with no body, and `next build`
 * still EXITS 0. That deployment 500s to crawlers for the whole outage, and
 * having lost its fallback it 500s again on the NEXT outage too, until some
 * later deploy happens to rebuild it. Repeated 5xx is what makes Search Console
 * mark a sitemap "Couldn't fetch".
 *
 * So the fix is not a new serving path — it is refusing to ship row 3. The CLI
 * beside this file runs after `next build` in the `build` npm script, which is
 * also Vercel's `buildCommand` (frontend/vercel.json). Failing there fails the
 * deploy, and a failed deploy leaves the PREVIOUS deployment serving its own
 * prerendered sitemap. That is what "stale wins" resolves to in practice: the
 * last good document keeps being served, and nobody has to notice.
 *
 * SCOPE OF THE MEASUREMENT, so the next reader does not over-trust the table.
 * All four rows were measured locally on `next start` against a stub feed of
 * two slugs per family. What that establishes is which artifacts a build
 * produces and what the server does with them, which does not depend on payload
 * size — rows 1 to 3 carry over. Row 4 does NOT generalise safely: it depends
 * on the family's response fitting a fetch Data Cache entry (~2 MB cap, body
 * base64-encoded), which sharding by family exists to stay under but which a
 * large family could still exceed, and it depends on Vercel restoring
 * `.next/cache` between builds, which is platform behaviour this repo has not
 * probed. Read row 4 as "the gate should fire rarely", never as a guarantee
 * that a warm cache rescues a degraded build. The two Vercel-side steps in the
 * paragraph above — a failed build not being promoted, and the previous
 * deployment continuing to serve — are likewise the platform's documented model
 * rather than something measured here; what was measured is the gate's exit
 * code.
 *
 * A prerendered body is proof the fetch was ANSWERED, because `fetchShard`
 * throws on a bad answer rather than emitting a partial
 * document. So EXISTENCE is the whole assertion — deliberately not a
 * URL count. A family with genuinely zero rows is a legitimate empty shard, and
 * a threshold there would fail builds for a real state of the catalogue.
 *
 * "Answered" rather than "succeeded", since PSY-1756. There is exactly one
 * non-failure that produces an empty body: a backend that says it does not
 * serve that family at all (HTTP 400/422 from the family enum), which happens
 * for one deploy window whenever the frontend ships a new family ahead of the
 * backend that implements it. That case degrades to an empty document on
 * purpose — see UNKNOWN_FAMILY_STATUSES in app/sitemap.ts for why it is safe to
 * distinguish and why 404 and 5xx are deliberately NOT in it. Every failure
 * shape in the table above still throws, so row 3 is still refused here; what
 * changed is that a family the backend has never heard of no longer looks like
 * an outage.
 *
 * `/sitemap-index` is deliberately not checked. It is a route handler that
 * builds its XML from `sitemap-shards.ts` with no network call at all, so a
 * backend outage cannot degrade it — it stays `○ (Static)` in every build mode
 * measured above.
 *
 * Every coupling to Next internals here fails CLOSED. A renamed
 * `renderingMode`, a moved `.body` path, a changed manifest key or a changed
 * `distDir` all make shards look un-prerendered, so a Next upgrade that moves
 * any of them breaks the build loudly rather than passing vacuously. The only
 * cost is misattribution, which `formatShardFailures` handles.
 */
import {
  ALL_SHARD_IDS,
  PAGES_SHARD_ID,
  shardFamily,
  shardRoutePath,
} from '@/app/sitemap-shards'

/** The build artifact holding shard `id`'s rendered XML, relative to `.next`. */
export function shardBodyPath(id: string): string {
  return `server/app/sitemap/${id}.xml.body`
}

/** The slice of `prerender-manifest.json` this check reads. */
export interface PrerenderManifestLike {
  routes?: Record<string, { renderingMode?: string } | undefined> | null
}

export interface ShardPrerenderFailure {
  route: string
  /** Why this shard has no usable fallback, phrased for a build log. */
  reason: string
}

/**
 * Report every shard that this build cannot serve from disk during an outage.
 *
 * Always scans the whole of `ALL_SHARD_IDS` — there is no subset parameter, so
 * the count in the failure message can never disagree with the set actually
 * scanned. `hasBody` is injected instead, which keeps this a pure function over
 * the two facts that matter (the manifest entry and the artifact) and lets the
 * tests cover the matrix with no `.next` directory.
 */
export function findShardsWithoutFallback(
  manifest: PrerenderManifestLike,
  hasBody: (bodyPath: string) => boolean
): ShardPrerenderFailure[] {
  const routes = manifest.routes ?? {}
  const failures: ShardPrerenderFailure[] = []

  for (const id of ALL_SHARD_IDS) {
    const route = shardRoutePath(id)
    const entry = routes[route]

    if (!entry) {
      failures.push({
        route,
        reason: 'no prerender-manifest entry — the route fell back to Dynamic',
      })
      continue
    }
    // A non-STATIC mode would still be rendered per request, so it carries the
    // same outage exposure as a missing entry even though a manifest key exists.
    if (entry.renderingMode !== 'STATIC') {
      failures.push({
        route,
        reason: `renderingMode is "${entry.renderingMode}", expected "STATIC"`,
      })
      continue
    }
    if (!hasBody(shardBodyPath(id))) {
      failures.push({
        route,
        reason: `no rendered body at .next/${shardBodyPath(id)}`,
      })
    }
  }

  return failures
}

/**
 * True when the manifest describes a build that plainly has routes, yet none of
 * them are the shards this module expects.
 *
 * A backend outage cannot produce that: the pages shard makes no network call,
 * so it survives and every OTHER shard is the worst an outage can do (measured,
 * row 3 above). Losing the pages shard too, while other routes prerendered
 * normally, points at the shard route naming or the manifest shape moving under
 * a Next upgrade — a different debugging session, and worth saying so rather
 * than sending the reader at a backend that was healthy.
 */
export function looksLikeManifestShapeChange(
  manifest: PrerenderManifestLike,
  failures: readonly ShardPrerenderFailure[]
): boolean {
  return (
    failures.length === ALL_SHARD_IDS.length &&
    Object.keys(manifest.routes ?? {}).length > 0
  )
}

/** The build-log message for a set of failures. Empty string when there are none. */
export function formatShardFailures(
  failures: readonly ShardPrerenderFailure[],
  manifest: PrerenderManifestLike
): string {
  if (failures.length === 0) return ''

  const diagnosis = looksLikeManifestShapeChange(manifest, failures)
    ? [
        'NONE of the expected shard routes are in the manifest, yet the manifest',
        'lists other routes. A backend outage leaves the pages shard prerendered,',
        'so it cannot produce this. Suspect a Next.js upgrade moving the manifest',
        'shape or the shard route naming before you suspect the backend.',
      ]
    : [
        'These shards would return HTTP 500 to crawlers for the duration of any',
        'backend outage, and this deployment would carry that exposure until the',
        'next successful build. Almost always the cause is that GET /sitemap/entries',
        'was unreachable or answering badly while this build ran — re-run the build',
        'once the backend is healthy.',
      ]

  return [
    `Sitemap prerender check FAILED: ${failures.length} of ${ALL_SHARD_IDS.length} shards have no fallback document.`,
    ...failures.map(f => `  ${f.route} — ${f.reason}`),
    '',
    ...diagnosis,
    '',
    'To build locally without a backend on purpose, run the local Next binary',
    'directly — `node_modules/.bin/next build`. This gate is chained in the npm',
    'script, not in Next, so that path skips it without any way for the skip to',
    'reach a deploy.',
  ].join('\n')
}

/**
 * What the backend says about a shard, as far as this gate cares.
 *
 * `unknown` is the ONLY verdict that excuses a missing shard, and it means the
 * backend answered — definitely and quickly — that it does not serve that
 * family (see UNKNOWN_FAMILY_STATUSES in app/sitemap.ts). `unreachable` covers
 * everything else, including a backend that is simply down, which is the case
 * this gate exists to refuse.
 *
 * The probe asks about a SHARD id, not a family, and that works unchanged for
 * a sub-shard because a sub-shard id is itself the wire value of the `family`
 * query, which is precisely why the sub-shard rides in that parameter rather
 * than a second one.
 */
export type FamilyVerdict = 'served' | 'unknown' | 'unreachable'

/** Asks the backend about one shard id. Injected, so `check.ts` stays pure. */
export type FamilyProbe = (shardId: string) => Promise<FamilyVerdict>

export interface PartitionedFailures {
  /** Shards that must fail the build. */
  blocking: ShardPrerenderFailure[]
  /** Shards excused because the backend does not serve that family yet. */
  pending: ShardPrerenderFailure[]
}

/**
 * Split shard failures into the ones that must fail the build and the ones the
 * backend has explained (PSY-1756).
 *
 * WHY THIS EXISTS. A family lands in the frontend and the backend in the same
 * PR, but they deploy on different pipelines: Vercel builds the frontend
 * against the ALREADY-DEPLOYED API. So for one window the build asks for a
 * family the running backend has never heard of, that shard alone fails to
 * prerender, and this gate fails the deploy — measured on this ticket's own
 * preview builds, `1 of 11 shards`, with the other ten prerendered from the
 * same healthy backend. The same race recurs at merge between Vercel's
 * production build and Railway's. Hand-sequencing two pipelines is not a fix
 * anyone can be relied on to repeat, and there is no ordering that works in
 * both directions anyway.
 *
 * WHY IT DOES NOT WEAKEN THE GATE. The excuse requires a POSITIVE answer from
 * the backend: HTTP 400/422 for that family, i.e. "I do not serve that". A
 * backend that is down, slow, or erroring returns `unreachable`, so every row
 * of the measured outage table still fails closed — including the row 3 case
 * where every entity shard loses its body at once, because a down backend
 * cannot say "I do not serve shows". The `pages` shard is never excused: it
 * makes no network call, so nothing about the backend can explain its absence.
 *
 * WHAT THE EXCUSED SHARD COSTS, and it is NOT the same for the two things that
 * can be excused. Either way the shard ships Dynamic for one deploy window, so
 * it would 500 during a backend outage in that window instead of serving a
 * stale body, and the next build after the backend ships prerenders it normally
 * with no manual step. What differs is whether the empty document it serves in
 * the meantime is TRUE:
 *
 *   - A new FAMILY (PSY-1756's `venue_years`): true. The backend has no URLs
 *     for that family yet, so nothing is lost by announcing none.
 *   - A new SUB-SHARD of an existing family (a bucket of `releases`):
 *     FALSE. The backend holds every one of those rows and simply does not
 *     recognise the id, and because a rollout adds all the buckets at once the
 *     whole family drops out of the index until the backend ships. Recovery is
 *     on the next RENDER, and fully on the next build — an excused shard is
 *     Dynamic, so it has no ISR timer to wait on. See describePendingCost.
 *
 * The gate cannot tell the two apart and must not try: during a legitimate
 * sub-shard rollout the old backend serves `releases` and rejects
 * `releases-b0`, which is indistinguishable from a genuinely drifted id. So
 * the excuse stays, and the compensating control is elsewhere — `compareFamilies`
 * in lib/sitemap-monitor/evaluate.ts flags a family as `vanished` when the API
 * has rows and the sitemap serves none, at any tolerance.
 */
export async function partitionShardFailures(
  failures: readonly ShardPrerenderFailure[],
  probe: FamilyProbe
): Promise<PartitionedFailures> {
  const blocking: ShardPrerenderFailure[] = []
  const pending: ShardPrerenderFailure[] = []

  for (const failure of failures) {
    const shardId = shardIdFromRoute(failure.route)
    // The pages shard makes no network call; the backend cannot excuse it.
    // An unrecognised route shape is likewise never excused — see
    // looksLikeManifestShapeChange for what that usually means.
    if (shardId === null || shardId === PAGES_SHARD_ID) {
      blocking.push(failure)
      continue
    }
    const verdict = await probe(shardId)
    if (verdict === 'unknown') pending.push(failure)
    else blocking.push(failure)
  }

  return { blocking, pending }
}

/** The shard id a route path names, or null if it is not a shard route. */
export function shardIdFromRoute(route: string): string | null {
  return ALL_SHARD_IDS.find(id => shardRoutePath(id) === route) ?? null
}

/**
 * What the pending shards actually cost, which is NOT the same for the two
 * things that can be pending — and the gate can tell them apart even though it
 * must not BLOCK on the difference.
 *
 * A brand-new family: the backend has no rows either, so the empty document is
 * true and nothing is missing. A sub-shard of a family the backend already
 * serves: the rows exist and go unannounced, so a share of a live family leaves
 * the index. Naming which case applies is the difference between a reassuring
 * message and an accurate one.
 *
 * Recovery is deliberately NOT described as "within the hour". A pending shard
 * shipped Dynamic — it has no prerendered body and therefore no ISR timer — so
 * it recovers when it is next RENDERED, and sitemap documents are rendered when
 * something requests them. The next build restores the prerender.
 */
function describePendingCost(pending: readonly ShardPrerenderFailure[]): string[] {
  const families = new Set<string>()
  for (const failure of pending) {
    const shardId = shardIdFromRoute(failure.route)
    const family = shardId === null ? undefined : shardFamily(shardId)
    // A shard whose id IS its family is a whole new family; anything else is a
    // slice of a family the backend already serves.
    if (family && family !== shardId) families.add(family)
  }

  if (families.size === 0) {
    return [
      'Each of these is a whole new FAMILY, so the backend has no rows for it either —',
      'the empty document is true and no known URL is missing from the index.',
    ]
  }

  return [
    `These include sub-shards of ${[...families].map(f => `"${f}"`).join(', ')}, which the`,
    'backend ALREADY serves rows for. Those URLs exist and are simply not being',
    'announced while this deployment is live, so a share of a live family is absent',
    'from the index — not nothing. A pending shard is Dynamic, so it has no ISR timer:',
    'it recovers when it is next rendered, and fully on the next build after the',
    'backend ships. The freshness monitor is the backstop and it runs DAILY, so do not',
    'rely on it to notice this within the deploy window.',
  ]
}

/** The build-log message for shards the backend has not caught up with yet. */
export function formatPendingShards(
  pending: readonly ShardPrerenderFailure[]
): string {
  if (pending.length === 0) return ''
  return [
    `Sitemap prerender check: ${pending.length} shard(s) are not prerendered because the`,
    'backend does not serve that family yet. This is the frontend deploying ahead of',
    'the API that implements it, and it clears itself on the first build after the',
    'backend ships.',
    ...pending.map(f => `  ${f.route}`),
    '',
    'Until then those shards render per request and serve an EMPTY document, and they',
    'have no stale-serving fallback, so they would 500 during a backend outage in this',
    'window.',
    '',
    ...describePendingCost(pending),
    '',
    'If this is still printing after the backend has deployed, an id has drifted',
    'between the two sides. Start at SITEMAP_FAMILIES and the shard id lists in',
    'app/sitemap-shards.ts, and at sitemapFamilies / sitemapShardsPerFamily in',
    'backend/internal/services/catalog/sitemap.go.',
  ].join('\n')
}
