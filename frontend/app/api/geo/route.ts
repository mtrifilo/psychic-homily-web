import { NextRequest, NextResponse } from 'next/server'
import { decodeGeoHeaders } from '@/lib/geo-default'
import type { CityState } from '@/components/filters/CityFilters'

/**
 * IP-geo default-city route handler (PSY-946).
 *
 * Returns the visitor's `{ city, state }` suggestion decoded from the Vercel
 * edge geo headers, or `null` when geo is unavailable / ambiguous / non-US-CA.
 * This is the CLIENT-FETCH transport for the geo default on `/shows` (ISR) and
 * home (static), which can't read `next/headers` server-side without
 * un-ISR-ing themselves. /explore uses the server `next/headers()` path
 * instead — see the two-read-paths note in `lib/geo-default.ts`.
 *
 * The response echoes only the geo HEADERS Vercel already injected for THIS
 * request through the same US/CA gate /explore uses; it exposes nothing the
 * caller's own request didn't already carry, so the unauthenticated public
 * endpoint leaks nothing and needs no rate limiting (it's strictly cheaper
 * than the page render it precedes).
 *
 * Dynamic without segment configs: reading `request.headers` already opts this
 * handler into per-request (dynamic) rendering. We CANNOT set
 * `export const runtime = 'edge'` or `export const dynamic = 'force-dynamic'`
 * — Next 16's `cacheComponents: true` (this repo's PPR mode, next.config.ts)
 * REJECTS those route-segment configs at build time ("not compatible with
 * nextConfig.cacheComponents"). The header read plus the no-store Cache-Control
 * below give the same guarantee (dynamic, never cached) without the configs.
 */

interface GeoResponse {
  geo: CityState | null
}

export function GET(request: NextRequest): NextResponse<GeoResponse> {
  const geo = decodeGeoHeaders(request.headers)

  return NextResponse.json(
    { geo },
    {
      // CRITICAL: one visitor's city must NEVER be served to another. The
      // response varies by the caller's IP-derived geo headers, which are not
      // part of the cache key, so any shared cache (Vercel CDN, a corporate
      // proxy, the browser bfcache) could poison a different visitor.
      // `private` forbids shared caches; `no-store` forbids storing at all
      // (incl. the browser's own disk cache) — the client's sessionStorage
      // layer in `useGeoDefaultCity` provides the intended per-session reuse,
      // so we don't rely on HTTP caching at all.
      headers: {
        'Cache-Control': 'private, no-store, max-age=0, must-revalidate',
      },
    },
  )
}
