import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { API_BASE_URL } from '@/lib/api-base'
import { CHART_MODULE_SLUGS } from '@/features/charts/moduleConfig'

/**
 * Slug-existence proxy — real HTTP 404 for unknown entity slug pages (PSY-897).
 *
 * Why this file exists
 * --------------------
 * Under Next 16's `cacheComponents: true` (PPR, see `next.config.ts`), entity
 * slug pages that call `notFound()` render the global `app/not-found.tsx` UI
 * but commit **HTTP 200**, not 404. The root cause is the root layout
 * (`app/layout.tsx`): every page's `{children}` is wrapped in a `<Suspense>`
 * around the async, cookie-reading `<AuthHydrator>`. That boundary flushes the
 * static shell — committing the 200 status header — before any page's
 * `await getShow()` → `notFound()` resolves. Next's own docs describe this
 * exactly ("a `200` status code will be returned [...] the status code of the
 * response cannot be updated [after streaming starts]") and prescribe the fix:
 *
 *   "If you need a 404 status [...] ensure the resource exists before the
 *    response body is streamed [...] run this check in `proxy` to rewrite
 *    missing slugs to a not-found route, or produce a 404 response. Keep proxy
 *    checks fast, and avoid fetching full content there."
 *   — Next.js v16 loading.js "Status Codes" docs
 *
 * Proxy runs BEFORE the route renders/streams, so a 404 produced here sets the
 * status header before the streaming trap can commit a 200. This is Option C
 * from the PSY-897 architecture spike (Options A and B were rejected — A is
 * impossible because the trapping Suspense is the root-layout ancestor of every
 * page, and B regresses the PSY-797/PSY-841 PPR/ISR architecture).
 *
 * Scope: Phase 1 proved the approach on SHOWS. Phase 2 extends it to the five
 * uniform-shape entities (venues, artists, releases, labels, festivals) plus
 * `tags`. Phase 3 adds `scenes` (PSY-906) — a derived city/state aggregation,
 * not a stored entity, whose `GET /scenes/<slug>` already 404s for an
 * unresolvable slug or a location below the scene threshold. Adding an entity
 * is a single `ENTITY_CHECKS` entry + a single `config.matcher` source.
 * Collections (auth-gated) and blog/dj-sets (local MDX) remain out of scope and
 * have distinct semantics.
 *
 * How the 404 is produced
 * ------------------------
 * We `rewrite` to a synthetic path that matches NO route. Next's routing layer
 * returns HTTP 404 for an unmatched path and renders `app/not-found.tsx` — the
 * SAME mechanism that already correctly 404s for `/this-route-does-not-exist`
 * (a no-route-match 404 is resolved before any render/stream, so it is NOT
 * subject to the streaming-commits-200 trap that breaks page-level
 * `notFound()`). We also pass `{ status: 404 }` to `rewrite` as belt-and-
 * suspenders for Next versions that honor a rewrite status override. The
 * synthetic target lives OUTSIDE this proxy's matcher (which intercepts only
 * the enumerated entity prefixes), so the rewrite cannot re-trigger the
 * proxy — no loop.
 */

/**
 * Path Next rewrites unknown slugs to. Deliberately matches no route segment
 * AND is not matched by `config.matcher` below (matcher only intercepts
 * `/shows/...`), guaranteeing no rewrite loop. The leading-underscore segment
 * also signals "framework-internal, not a user route".
 */
const NOT_FOUND_REWRITE_PATH = '/_psy-not-found'

/**
 * Per-entity existence check. The function returns the backend HEAD probe URL
 * whose 404 response means "slug does not exist". The probe uses direct backend
 * existence queries instead of duplicating each page's full `GET /<type>/<slug>`
 * fetch and its response hydration. Adding an entity is one entry here plus one
 * `config.matcher` source.
 *
 * The backend probe centralizes the historical non-uniform cases: tags still
 * resolve by tag ID/slug without loading `/tags/<slug>/detail`, and scenes still
 * resolve derived city/state slugs against the same qualifying venue threshold
 * without loading the full computed scene detail. Collections (auth-gated) and
 * blog/dj-sets (local MDX, no backend existence endpoint) are deliberately
 * absent — they have distinct semantics.
 */
const ENTITY_CHECKS: Record<string, (slug: string) => string> = {
  shows: slug =>
    `${API_BASE_URL}/entities/shows/${encodeURIComponent(slug)}/exists`,
  venues: slug =>
    `${API_BASE_URL}/entities/venues/${encodeURIComponent(slug)}/exists`,
  artists: slug =>
    `${API_BASE_URL}/entities/artists/${encodeURIComponent(slug)}/exists`,
  releases: slug =>
    `${API_BASE_URL}/entities/releases/${encodeURIComponent(slug)}/exists`,
  labels: slug =>
    `${API_BASE_URL}/entities/labels/${encodeURIComponent(slug)}/exists`,
  festivals: slug =>
    `${API_BASE_URL}/entities/festivals/${encodeURIComponent(slug)}/exists`,
  tags: slug =>
    `${API_BASE_URL}/entities/tags/${encodeURIComponent(slug)}/exists`,
  scenes: slug =>
    `${API_BASE_URL}/entities/scenes/${encodeURIComponent(slug)}/exists`,
}

/**
 * Static (non-`[slug]`) routes that live UNDER a proxied entity prefix. These
 * are real Next routes — e.g. `app/shows/submit/page.tsx` (the show-submission
 * form) and `app/shows/saved/page.tsx` (a server redirect to `/library`) — NOT
 * entity slugs. The `config.matcher`'s `/shows/:path*` source intercepts them,
 * and the `/<entity>/<slug>` shape guard below would otherwise existence-check
 * `GET ${API_BASE_URL}/shows/submit` → backend 404 → rewrite the real page to a
 * 404 (PSY-913, regressed by PSY-897's shows phase). Excluding these segments
 * lets the genuine page render.
 *
 * Today only `shows` has static sub-routes; the other six proxied prefixes
 * (venues/artists/releases/labels/festivals/tags) have only `[slug]`. When
 * adding a NEW static route under ANY proxied prefix, add its segment here.
 */
const RESERVED_SEGMENTS: Record<string, ReadonlySet<string>> = {
  shows: new Set(['submit', 'saved']),
}

/**
 * Fixed allowlist for `/charts/[module]` drill-downs. Unlike entity slug
 * pages there is no backend existence probe — unknown modules are rewritten
 * here so `notFound()` in the page does not soft-404 under cacheComponents.
 */
const CHART_MODULE_SEGMENTS = new Set<string>(CHART_MODULE_SLUGS)

/**
 * Returns the global 404 rewrite response (status 404 + render
 * `app/not-found.tsx` via the unmatched synthetic path).
 */
function notFoundResponse(request: NextRequest): NextResponse {
  return NextResponse.rewrite(new URL(NOT_FOUND_REWRITE_PATH, request.url), {
    status: 404,
  })
}

export async function proxy(request: NextRequest): Promise<NextResponse> {
  const { pathname } = request.nextUrl

  // pathname is `/<entity>/<slug>` (matcher guarantees one of the enumerated
  // entity prefixes). Split into ["", "<entity>", "<slug>", ...optional
  // sub-segments].
  const segments = pathname.split('/')
  const entityType = segments[1]
  const slug = segments[2]

  // Charts module drill-downs: allowlist check (no backend probe). Bare
  // `/charts` and deeper paths pass through; unknown modules get a real 404.
  if (entityType === 'charts') {
    if (!slug || segments.length !== 3) {
      return NextResponse.next()
    }
    if (!CHART_MODULE_SEGMENTS.has(slug)) {
      return notFoundResponse(request)
    }
    return NextResponse.next()
  }

  const buildCheckUrl = ENTITY_CHECKS[entityType]

  // Not a mapped entity, or a sub-route like `/<entity>/<slug>/edit`, or the
  // bare `/<entity>` list (no slug): leave untouched. Only the exact
  // `/<entity>/<slug>` detail shape is existence-checked.
  if (!buildCheckUrl || !slug || segments.length !== 3) {
    return NextResponse.next()
  }

  // `/<entity>/<segment>` where <segment> is a real static route (e.g.
  // `/shows/submit`, `/shows/saved`), not an entity slug — never existence-check
  // it, or we'd 404 a genuine page (PSY-913).
  if (RESERVED_SEGMENTS[entityType]?.has(slug)) {
    return NextResponse.next()
  }

  try {
    const res = await fetch(buildCheckUrl(slug), {
      method: 'HEAD',
      // `next: { revalidate }` has NO effect inside proxy (per Next docs), so
      // we don't set it. `redirect: 'manual'` keeps the check cheap and avoids
      // following any backend redirect chain. A 2xx means the slug resolves;
      // only a backend 404 is treated as "missing".
      redirect: 'manual',
    })

    // Backend reachable and slug resolves → let the page render normally
    // (ISR / hydration path untouched).
    if (res.ok) {
      return NextResponse.next()
    }

    // 404 from the backend = slug genuinely does not exist → real 404.
    if (res.status === 404) {
      return notFoundResponse(request)
    }

    // Any other non-ok (5xx, 403, 429, opaqueredirect, …): fail OPEN — let the
    // page render and apply its own handling (each page's server fetch reports
    // 5xx to Sentry, renders its own not-found on null, etc.). Producing a 404
    // here on a transient backend blip would mask real outages as "not found".
    return NextResponse.next()
  } catch {
    // Network error reaching the backend: fail OPEN. The proxy must never take
    // the whole route down when the existence check itself fails.
    return NextResponse.next()
  }
}

export const config = {
  /**
   * Intercept ONLY the enumerated entity prefixes (`/shows/...`, `/venues/...`,
   * `/artists/...`, `/releases/...`, `/labels/...`, `/festivals/...`,
   * `/tags/...`, `/scenes/...`, `/charts/...`). Each prefix scopes its match away from
   * everything else — the homepage, out-of-scope routes (`/collections/...`,
   * `/blog/...`, `/dj-sets/...`), `api`, `_next/static`, `_next/image`,
   * metadata files, and the `/_psy-not-found` rewrite target — so none of those
   * can be blocked or re-intercepted. (A root-level matcher would need a
   * negative-lookahead to exclude `api`/`_next`/metadata; enumerating exact
   * prefixes makes that unnecessary.) The `proxy()` body further narrows each
   * to the exact `/<entity>/<slug>` detail shape — bare `/<entity>` list pages
   * and `/<entity>/<slug>/<sub>` routes pass through untouched. Charts is an
   * allowlist check (no backend probe), not an `ENTITY_CHECKS` entry.
   *
   * Each entry's `missing` excludes RSC prefetch requests (`next-router-
   * prefetch` / `purpose: prefetch` headers) so client-side route prefetches
   * don't fire a backend existence lookup on every hovered link.
   *
   * Keep entity sources in lockstep with `ENTITY_CHECKS` above: a source here with
   * no matching `ENTITY_CHECKS` entry would intercept the route only to fall
   * through `NextResponse.next()` (wasted match), and an `ENTITY_CHECKS` entry
   * with no source here would never run. Charts is the intentional exception.
   */
  matcher: [
    {
      source: '/shows/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/venues/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/artists/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/releases/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/labels/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/festivals/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/tags/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/scenes/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/charts/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
  ],
}
