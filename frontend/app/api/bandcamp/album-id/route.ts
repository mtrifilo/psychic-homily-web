import { NextRequest, NextResponse } from 'next/server'

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

  try {
    const response = await fetch(url, {
      headers: {
        'User-Agent': 'Mozilla/5.0 (compatible; MusicEmbed/1.0)',
      },
    })

    if (!response.ok) {
      return NextResponse.json(
        { error: `Failed to fetch Bandcamp page: ${response.status}` },
        { status: response.status }
      )
    }

    const html = await response.text()

    // Extract album ID from the page - look for album=NUMBERS pattern
    const match = html.match(/album=(\d+)/)
    if (!match) {
      return NextResponse.json(
        { error: 'Could not find album ID on page' },
        { status: 404 }
      )
    }

    const albumId = match[1]

    return NextResponse.json(
      { albumId },
      {
        headers: {
          'Cache-Control': 'public, max-age=86400, stale-while-revalidate=604800',
        },
      }
    )
  } catch (error) {
    console.error('Bandcamp album ID extraction error:', error)
    return NextResponse.json(
      { error: 'Failed to extract album ID' },
      { status: 500 }
    )
  }
}
