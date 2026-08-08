/**
 * Reading the nightly overview snapshot from the SERVER — the share surface's
 * fetch, and nothing else.
 *
 * A leaf module by design: the share card renders on the edge runtime, which
 * already sits at ~96.5% of Vercel's 1 MB per-function limit, so this must not
 * reach into anything that drags a page view, React Query or the app's `lib/api`
 * client into that bundle. See `pattern_og_edge_size_and_font_hermeticity`.
 *
 * Separate from `hooks/useGraphOverview` on purpose. That hook is the CLIENT's
 * read: it needs a query key, a stale time and a retry policy. This is one
 * server fetch with a revalidate window, and the two have nothing to share but
 * the URL, which they both take from `./api`.
 */

import * as Sentry from '@sentry/nextjs'

import { GRAPH_OVERVIEW_NOT_BUILT_STATUS, graphEndpoints } from './api'
import { buildSceneMap, type GraphOverview, type SceneMap } from './sceneMap'
import { resolveGraphWeek, type GraphWeek } from './graphWeek'

/**
 * How long a fetched snapshot stays fresh.
 *
 * The snapshot changes exactly once a night, so an hour is generous rather than
 * stale — and it matches the `max-age` the endpoint sets for itself, keeping the
 * data cache and the HTTP cache on one clock instead of asking for a
 * revalidation the layer below would answer from its own store.
 */
export const GRAPH_OVERVIEW_REVALIDATE = 3600

/** Which surface a failure came from, so Sentry triage can tell them apart. */
export type GraphOverviewService = 'graph-week-page' | 'og-image'

/**
 * Fetch the snapshot, or null when there is not one to be had.
 *
 * A deliberate fail-open, the same classification the rest of the share-card
 * family carries: every failure returns null and the caller falls back — the
 * page to its empty state, the card to the branded fallback image. There is no
 * reader-visible content behind this to protect, and a share preview that errors
 * is worse than one that renders generically.
 */
export async function fetchGraphOverview(
  service: GraphOverviewService
): Promise<GraphOverview | null> {
  try {
    const res = await fetch(graphEndpoints.OVERVIEW, {
      next: { revalidate: GRAPH_OVERVIEW_REVALIDATE },
    })
    // `await` inside the try is load-bearing: `return res.json()` would adopt
    // the promise after the block exits, so a malformed body would reject past
    // this catch and 500 the route instead of reaching the fallback.
    if (res.ok) return asOverview(await res.json())
    // Everything except "not built yet" and a 404 is worth knowing about. A
    // nightly job that has never run is a fact about the install; a 500 is not.
    if (res.status !== GRAPH_OVERVIEW_NOT_BUILT_STATUS && res.status !== 404) {
      Sentry.captureMessage(`Graph overview: API returned ${res.status}`, {
        level: res.status >= 500 ? 'error' : 'warning',
        tags: { service },
        extra: { status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, { tags: { service } })
  }
  return null
}

/**
 * A 200 is not proof of the right shape.
 *
 * `last_mapped` and `epoch` go straight into date maths and `node_count` into
 * every column-length check, so a redirect, a CDN error page or a future API
 * change answering 200 with something else would crash the route rather than
 * reach its fallback. `buildSceneMap` validates the columns themselves; this
 * only has to establish that the body IS a snapshot.
 */
function asOverview(body: unknown): GraphOverview | null {
  if (!body || typeof body !== 'object') return null
  const record = body as Record<string, unknown>
  if (typeof record.last_mapped !== 'string' || typeof record.epoch !== 'string') return null
  if (typeof record.node_count !== 'number' || typeof record.version !== 'number') return null
  if (!record.nodes || typeof record.nodes !== 'object') return null
  return body as GraphOverview
}

/**
 * A snapshot we can draw, and the week it describes.
 *
 * NOT named `GraphWeekView`: that is the component module which renders this,
 * and a data type sharing a name with a component is the kind of collision that
 * costs a reader a file open every time.
 */
export interface ResolvedGraphWeek {
  map: SceneMap
  week: GraphWeek
}

/**
 * The whole read: fetch, decode, and date the week — or null.
 *
 * ONE function because the share card and the share page must agree about what
 * "there is no week" means, and they are two separate renders of the same fact.
 * Written twice, a fourth reason to refuse a snapshot (a version floor, a
 * staleness cut-off) would have to be remembered in both, and the failure mode
 * is a page and its own `og:image` disagreeing — silently, in a preview nobody
 * re-checks.
 *
 * FOUR DIFFERENT ABSENCES, ONE ANSWER. No snapshot yet (a fresh install's 503),
 * a fetch that failed, a payload we cannot decode, and a snapshot we cannot date
 * all leave a reader in the same place, so the callers do not distinguish them:
 * the card falls back to the branded image, the page to its empty state.
 */
export async function loadResolvedGraphWeek(
  service: GraphOverviewService
): Promise<ResolvedGraphWeek | null> {
  const overview = await fetchGraphOverview(service)
  if (!overview) return null
  const map = buildSceneMap(overview)
  if (!map) return null
  const week = resolveGraphWeek(map)
  if (!week) return null
  return { map, week }
}
