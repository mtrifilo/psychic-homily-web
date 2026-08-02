import { NextRequest, NextResponse } from 'next/server'
import * as Sentry from '@sentry/nextjs'
import {
  MAX_BODY_BYTES,
  SECRET_HEADER,
  isAuthorized,
  parseRevalidateBody,
  revalidateEntities,
} from '@/lib/internal-revalidate'

/**
 * Push ISR revalidation for writes that never pass through the API proxy
 * (PSY-1691). See lib/internal-revalidate.ts for the why and the rules.
 *
 *   POST /api/internal/revalidate
 *   x-internal-secret: <INTERNAL_API_SECRET>
 *   { "entities": [ { "type": "show", "slug": "big-gig-2026-08-01" } ] }
 *   → 200 { "revalidated": 1, "skipped": 0, "paths": 8 }
 *
 * No route-segment config: reading `request.headers` already makes this
 * handler dynamic, and `cacheComponents: true` (next.config.ts) rejects
 * `runtime` / `dynamic` exports at build time. The default Node runtime is
 * what the constant-time compare's `node:crypto` needs.
 */
export async function POST(request: NextRequest): Promise<NextResponse> {
  try {
    if (!isAuthorized(request.headers.get(SECRET_HEADER))) {
      return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
    }

    // Cheap pre-read rejection; parseRevalidateBody re-checks the real size
    // because Content-Length is caller-supplied and optional.
    const declaredLength = Number(request.headers.get('content-length'))
    if (Number.isFinite(declaredLength) && declaredLength > MAX_BODY_BYTES) {
      return NextResponse.json(
        { error: `Body exceeds ${MAX_BODY_BYTES} bytes` },
        { status: 400 }
      )
    }

    const parsed = parseRevalidateBody(await request.text())
    if ('error' in parsed) {
      return NextResponse.json({ error: parsed.error }, { status: 400 })
    }

    return NextResponse.json(revalidateEntities(parsed.entities))
  } catch (error) {
    // A handler bug must not look like a caller bug. Callers treat any
    // non-2xx as "revalidation did not happen" and move on.
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'isr-revalidation', source: 'internal-revalidate' },
    })
    return NextResponse.json({ error: 'Revalidation failed' }, { status: 500 })
  }
}
