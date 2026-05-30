import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { API_BASE_URL } from '@/lib/api-base'

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
 * Per-entity existence check. The function returns the backend URL whose
 * non-2xx response means "slug does not exist". Each entry reuses the SAME
 * backend GET the corresponding `app/<type>/[slug]/page.tsx` already calls in
 * its server-side `cache()`'d fetch, so the proxy's notion of existence matches
 * the page's exactly (each of these pages calls `notFound()` when that fetch
 * misses — verified per type). The spike flagged that a backend HEAD /
 * `/exists` endpoint would be cheaper than a full GET; using the existing GET
 * is acceptable for now (latency noted as a follow-up). Adding an entity is one
 * entry here plus one `config.matcher` source.
 *
 * Every type uses the uniform `/<type>/<slug>` backend shape EXCEPT `tags`,
 * whose page fetches the enriched `/tags/<slug>/detail` endpoint (the shape its
 * `useTagDetail` hook consumes) — see `app/tags/[slug]/page.tsx`. `scenes` is
 * also uniform: `GET /scenes/<slug>` is the same backend call the scene page's
 * `getScene` fetch makes, and it already 404s for an unresolvable slug or a
 * below-threshold location (≥2 verified venues). Collections (auth-gated) and
 * blog/dj-sets (local MDX, no backend existence endpoint) are deliberately
 * absent — they have distinct semantics.
 */
const ENTITY_CHECKS: Record<string, (slug: string) => string> = {
  shows: (slug) => `${API_BASE_URL}/shows/${encodeURIComponent(slug)}`,
  venues: (slug) => `${API_BASE_URL}/venues/${encodeURIComponent(slug)}`,
  artists: (slug) => `${API_BASE_URL}/artists/${encodeURIComponent(slug)}`,
  releases: (slug) => `${API_BASE_URL}/releases/${encodeURIComponent(slug)}`,
  labels: (slug) => `${API_BASE_URL}/labels/${encodeURIComponent(slug)}`,
  festivals: (slug) => `${API_BASE_URL}/festivals/${encodeURIComponent(slug)}`,
  // NOTE: tags is the lone non-uniform endpoint — `/tags/<slug>/detail`, not
  // `/tags/<slug>`. Matches the tag page's getTagDetail fetch.
  tags: (slug) => `${API_BASE_URL}/tags/${encodeURIComponent(slug)}/detail`,
  // scenes are derived city/state aggregations; the backend resolves the slug
  // against verified venues and 404s when it doesn't qualify (PSY-906). Matches
  // the scene page's getScene fetch.
  scenes: (slug) => `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}`,
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
      // `next: { revalidate }` has NO effect inside proxy (per Next docs), so
      // we don't set it. `redirect: 'manual'` keeps the check cheap and avoids
      // following any backend redirect chain. A 2xx/3xx means the slug
      // resolves; only a hard non-ok (4xx/5xx) is treated as "missing".
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
   * `/tags/...`, `/scenes/...`). Each prefix scopes its match away from
   * everything else — the homepage, out-of-scope routes (`/collections/...`,
   * `/blog/...`, `/dj-sets/...`), `api`, `_next/static`, `_next/image`,
   * metadata files, and the `/_psy-not-found` rewrite target — so none of those
   * can be blocked or re-intercepted. (A root-level matcher would need a
   * negative-lookahead to exclude `api`/`_next`/metadata; enumerating exact
   * prefixes makes that unnecessary.) The `proxy()` body further narrows each
   * to the exact `/<entity>/<slug>` detail shape — bare `/<entity>` list pages
   * and `/<entity>/<slug>/<sub>` routes pass through untouched.
   *
   * Each entry's `missing` excludes RSC prefetch requests (`next-router-
   * prefetch` / `purpose: prefetch` headers) so client-side route prefetches
   * don't fire a backend existence lookup on every hovered link.
   *
   * Keep this list in lockstep with `ENTITY_CHECKS` above: a source here with
   * no matching `ENTITY_CHECKS` entry would intercept the route only to fall
   * through `NextResponse.next()` (wasted match), and an `ENTITY_CHECKS` entry
   * with no source here would never run.
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
  ],
}
