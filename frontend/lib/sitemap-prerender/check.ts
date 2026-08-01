/**
 * The policy half of the post-build sitemap prerender gate.
 *
 * WHY THIS EXISTS — measured on Next 16.1.4 with `cacheComponents: true`, by
 * `next build` + `next start` + curl, backend killed at each stage:
 *
 *   build backend | build cache | shards prerendered | serving with backend down
 *   --------------|-------------|--------------------|--------------------------
 *   reachable     | clean       | 10 / 10            | 200, previous document
 *   reachable     | clean       | 10 / 10            | 200 even with the Data
 *                 |             |                    | Cache purged first
 *   UNREACHABLE   | clean       | 1 / 10 (pages)     | 500 on all 9 families
 *   UNREACHABLE   | warm        | 10 / 10            | 200, previous document
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
 * A prerendered body is proof the fetch succeeded, because
 * `fetchSitemapFamily` throws on a bad answer rather than emitting a partial
 * document. So EXISTENCE is the whole assertion — deliberately not a
 * URL count. A family with genuinely zero rows is a legitimate empty shard, and
 * a threshold there would fail builds for a real state of the catalogue.
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
import { ALL_SHARD_IDS, shardRoutePath } from '@/app/sitemap-shards'

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
 * so it survives and at most nine of ten shards fail (measured, row 3 above).
 * Losing all ten while other routes prerendered normally points at the shard
 * route naming or the manifest shape moving under a Next upgrade — a different
 * debugging session, and worth saying so rather than sending the reader at a
 * backend that was healthy.
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
