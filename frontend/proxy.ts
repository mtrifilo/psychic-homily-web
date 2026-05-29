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
 * Phase 1 scope: SHOWS ONLY (proof-of-approach). Venues/artists/releases/
 * labels/festivals/tags are Phase 2 — adding one is a single `ENTITY_CHECKS`
 * entry. Collections (auth-gated), blog/dj-sets (local MDX), and scenes
 * (separate soft-404) are Phase 3 and have distinct semantics.
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
 * synthetic target lives OUTSIDE this proxy's matcher (which only intercepts
 * `/shows/...`), so the rewrite cannot re-trigger the proxy — no loop.
 */

/**
 * Path Next rewrites unknown slugs to. Deliberately matches no route segment
 * AND is not matched by `config.matcher` below (matcher only intercepts
 * `/shows/...`), guaranteeing no rewrite loop. The leading-underscore segment
 * also signals "framework-internal, not a user route".
 */
const NOT_FOUND_REWRITE_PATH = '/_psy-not-found'

/**
 * Per-entity existence check. Phase 1 maps only `shows`. The `endpoint`
 * function returns the backend URL whose non-2xx response means "slug does not
 * exist". `shows` reuses the SAME `GET ${API_BASE_URL}/shows/<slug>` the page
 * itself calls (`app/shows/[slug]/page.tsx`). The spike flagged that a backend
 * HEAD / `/exists` endpoint would be cheaper than a full GET; using the
 * existing GET is acceptable for Phase 1 (latency noted as a follow-up). Adding
 * an entity in Phase 2 is one entry here (e.g. tags → `/tags/<slug>/detail`).
 */
const ENTITY_CHECKS: Record<string, (slug: string) => string> = {
  shows: (slug) => `${API_BASE_URL}/shows/${encodeURIComponent(slug)}`,
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

  // pathname is `/shows/<slug>` (matcher guarantees the `/shows/` prefix).
  // Split into ["", "shows", "<slug>", ...optional sub-segments].
  const segments = pathname.split('/')
  const entityType = segments[1]
  const slug = segments[2]

  const buildCheckUrl = ENTITY_CHECKS[entityType]

  // Not a Phase-1 entity, or a sub-route like `/shows/<slug>/edit`, or the
  // bare `/shows` list (no slug): leave untouched. Only the exact
  // `/<entity>/<slug>` detail shape is existence-checked.
  if (!buildCheckUrl || !slug || segments.length !== 3) {
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
    // page render and apply its own handling (the page's `getShow` reports 5xx
    // to Sentry, renders its own not-found on null, etc.). Producing a 404 here
    // on a transient backend blip would mask real outages as "not found".
    return NextResponse.next()
  } catch {
    // Network error reaching the backend: fail OPEN. The proxy must never take
    // the whole route down when the existence check itself fails.
    return NextResponse.next()
  }
}

export const config = {
  /**
   * Intercept ONLY `/shows/...` (Phase 1). The `/shows/` prefix already scopes
   * the match away from everything else — the homepage, other entity routes
   * (`/venues/...`), `api`, `_next/static`, `_next/image`, metadata files, and
   * the `/_psy-not-found` rewrite target — so none of those can be blocked or
   * re-intercepted. (A root-level matcher would need a negative-lookahead to
   * exclude `api`/`_next`/metadata; scoping to `/shows/` makes that
   * unnecessary.) The `proxy()` body further narrows to the exact
   * `/shows/<slug>` detail shape — bare `/shows` and `/shows/<slug>/<sub>` pass
   * through untouched.
   *
   * `missing` excludes RSC prefetch requests (`next-router-prefetch` /
   * `purpose: prefetch` headers) so client-side route prefetches don't fire a
   * backend existence lookup on every hovered link.
   */
  matcher: [
    {
      source: '/shows/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
  ],
}
