import { NextRequest, NextResponse } from 'next/server'
import * as Sentry from '@sentry/nextjs'
import { resolveBandcampEmbed } from '@/lib/bandcamp'

// Resolves a Bandcamp album OR track URL to an embeddable { kind, id }. The
// route name predates standalone-track support; it now handles /track/ URLs and
// auto-corrects an /album/ <-> /track/ path mismatch (see lib/bandcamp).
export async function GET(request: NextRequest) {
  const { searchParams } = new URL(request.url)
  const url = searchParams.get('url')

  if (!url) {
    return NextResponse.json({ error: 'Missing url parameter' }, { status: 400 })
  }

  // Validate it's a Bandcamp URL
  if (!url.includes('bandcamp.com')) {
    return NextResponse.json({ error: 'Not a Bandcamp URL' }, { status: 400 })
  }

  const result = await resolveBandcampEmbed(url)

  if (!result.ok) {
    // Page loaded but no embed id (status 200) -> 404; network throw -> 500;
    // otherwise surface the upstream status (e.g. a real 404).
    const status =
      result.status === null ? 500 : result.status === 200 ? 404 : result.status
    Sentry.captureMessage(`Bandcamp embed resolve failed: ${result.error}`, {
      level: status >= 500 ? 'error' : 'warning',
      tags: { service: 'bandcamp-scraper' },
      extra: { url, status: result.status },
    })
    return NextResponse.json({ error: result.error }, { status })
  }

  return NextResponse.json(
    { kind: result.embed.kind, id: result.embed.id },
    {
      headers: {
        'Cache-Control': 'public, max-age=86400, stale-while-revalidate=604800',
      },
    }
  )
}
