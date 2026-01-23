import { NextRequest, NextResponse } from 'next/server'

interface OEmbedResponse {
  html?: string
  type?: string
  version?: string
  title?: string
  author_name?: string
  author_url?: string
  provider_name?: string
  provider_url?: string
  thumbnail_url?: string
  width?: number
  height?: number
}

type Provider = 'bandcamp' | 'spotify' | null

function detectProvider(url: string): Provider {
  try {
    const parsed = new URL(url)
    if (parsed.hostname.includes('bandcamp.com')) {
      return 'bandcamp'
    }
    if (
      parsed.hostname.includes('spotify.com') ||
      parsed.hostname.includes('open.spotify.com')
    ) {
      return 'spotify'
    }
    return null
  } catch {
    return null
  }
}

function getOEmbedEndpoint(provider: Provider, url: string): string | null {
  switch (provider) {
    case 'bandcamp':
      return `https://bandcamp.com/api/oembed/1/url?url=${encodeURIComponent(url)}&format=json`
    case 'spotify':
      return `https://open.spotify.com/oembed?url=${encodeURIComponent(url)}`
    default:
      return null
  }
}

export async function GET(request: NextRequest) {
  const { searchParams } = new URL(request.url)
  const url = searchParams.get('url')

  if (!url) {
    return NextResponse.json(
      { error: 'Missing url parameter' },
      { status: 400 }
    )
  }

  const provider = detectProvider(url)
  if (!provider) {
    return NextResponse.json(
      { error: 'Unsupported provider. Only Bandcamp and Spotify are supported.' },
      { status: 400 }
    )
  }

  const oembedEndpoint = getOEmbedEndpoint(provider, url)
  if (!oembedEndpoint) {
    return NextResponse.json(
      { error: 'Failed to construct oEmbed endpoint' },
      { status: 500 }
    )
  }

  try {
    const response = await fetch(oembedEndpoint, {
      headers: {
        Accept: 'application/json',
      },
    })

    if (!response.ok) {
      return NextResponse.json(
        { error: `oEmbed request failed with status ${response.status}` },
        { status: response.status }
      )
    }

    const data: OEmbedResponse = await response.json()

    return NextResponse.json(data, {
      headers: {
        'Cache-Control': 'public, max-age=3600, stale-while-revalidate=86400',
      },
    })
  } catch (error) {
    console.error('oEmbed fetch error:', error)
    return NextResponse.json(
      { error: 'Failed to fetch oEmbed data' },
      { status: 500 }
    )
  }
}
