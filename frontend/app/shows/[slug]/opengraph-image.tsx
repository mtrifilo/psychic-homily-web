import { ImageResponse } from 'next/og'
import * as Sentry from '@sentry/nextjs'

export const runtime = 'edge'
export const alt = 'Show details'
export const size = { width: 1200, height: 630 }
export const contentType = 'image/png'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface ShowData {
  title?: string
  event_date: string
  is_sold_out: boolean
  is_cancelled: boolean
  venues: Array<{ name: string; city: string; state: string }>
  artists: Array<{ name: string; is_headliner?: boolean | null }>
}

function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

export default async function Image({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params

  let show: ShowData | null = null
  try {
    const res = await fetch(`${API_BASE_URL}/shows/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      show = await res.json()
    }
  } catch (error) {
    Sentry.captureException(error, {
      tags: { service: 'og-image' },
      extra: { slug },
    })
  }

  if (!show) {
    return new ImageResponse(
      (
        <div
          style={{
            width: '100%',
            height: '100%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            backgroundColor: '#0d0805',
            color: '#eee7d9',
            fontSize: 48,
            fontWeight: 700,
          }}
        >
          Psychic Homily
        </div>
      ),
      { ...size }
    )
  }

  const headliner =
    show.artists?.find(a => a.is_headliner)?.name ||
    show.artists?.[0]?.name ||
    'Live Music'
  const venue = show.venues?.[0]
  const venueName = venue?.name || 'TBA'
  const location = venue ? `${venue.city}, ${venue.state}` : ''
  const showDate = formatDate(show.event_date)
  const displayTitle = show.title || `${headliner} at ${venueName}`
  const otherArtists = show.artists
    ?.filter(a => a.name !== headliner)
    .map(a => a.name)

  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          backgroundColor: '#0d0805',
          color: '#eee7d9',
          padding: '60px 70px',
          position: 'relative',
        }}
      >
        {/* Cancelled overlay */}
        {show.is_cancelled && (
          <div
            style={{
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              bottom: 0,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              zIndex: 10,
            }}
          >
            <div
              style={{
                color: '#ef4444',
                fontSize: 96,
                fontWeight: 800,
                letterSpacing: '0.1em',
                opacity: 0.3,
                transform: 'rotate(-15deg)',
              }}
            >
              CANCELLED
            </div>
          </div>
        )}

        {/* Top: Date + Sold Out badge */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '20px' }}>
          <div style={{ color: '#e89960', fontSize: 28, fontWeight: 600 }}>
            {showDate}
          </div>
          {show.is_sold_out && (
            <div
              style={{
                backgroundColor: '#ef4444',
                color: '#ffffff',
                fontSize: 18,
                fontWeight: 700,
                padding: '6px 16px',
                borderRadius: 6,
                letterSpacing: '0.05em',
              }}
            >
              SOLD OUT
            </div>
          )}
        </div>

        {/* Middle: Show title */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px', flex: 1, justifyContent: 'center' }}>
          <div
            style={{
              fontSize: displayTitle.length > 40 ? 52 : 64,
              fontWeight: 800,
              lineHeight: 1.1,
              letterSpacing: '-0.02em',
            }}
          >
            {displayTitle}
          </div>

          {/* Other artists */}
          {otherArtists && otherArtists.length > 0 && (
            <div style={{ fontSize: 26, color: '#b8a99a', fontWeight: 400 }}>
              with {otherArtists.slice(0, 4).join(', ')}
              {otherArtists.length > 4 ? ` + ${otherArtists.length - 4} more` : ''}
            </div>
          )}
        </div>

        {/* Bottom: Venue + branding */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end' }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
            <div style={{ fontSize: 30, fontWeight: 600 }}>{venueName}</div>
            {location && (
              <div style={{ fontSize: 22, color: '#b8a99a' }}>{location}</div>
            )}
          </div>
          <div style={{ fontSize: 22, color: '#e89960', fontWeight: 600 }}>
            psychichomily.com
          </div>
        </div>
      </div>
    ),
    { ...size }
  )
}
