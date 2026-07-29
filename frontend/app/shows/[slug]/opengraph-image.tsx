import { ImageResponse } from 'next/og'
import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import { resolveShowTimezone } from '@/lib/utils/formatters'
import { OG_COLORS, OG_CONTENT_TYPE, OG_FONT_FAMILY, OG_SIZE } from '@/lib/og/brand'
import {
  loadBrandFontsOrDefault,
  ogCacheControl,
  ogFallbackCard,
} from '@/lib/og/response'
import { fitFontSize, measureMono } from '@/lib/og/textFit'

export const runtime = 'edge'
export const alt = 'Show details'
export const size = OG_SIZE
export const contentType = OG_CONTENT_TYPE

/**
 * Retuned for the size this card is actually consumed at.
 *
 * A 1200×630 card renders about 300px wide in an iMessage bubble or a Discord
 * embed — a 4× downscale — and group chats are the highest-intent surface for
 * live music, so that is the size the hierarchy has to survive. The previous
 * tuning put three elements under 7px effective: the support act at 6.5px, and
 * the city and domain both at 5.5px.
 *
 * The structural fix is removing an element rather than shrinking everything to
 * fit: the city is folded into the venue line. Everything that remains clears
 * ~8px effective. Divide by 4 to check: date 8.5 · title 19.5 · support 8.5 ·
 * venue 9.5 · domain 8.
 *
 * Design: Figma `XakQQ0nYGqnt77PrHKO9IE`, node `1192:15` (the PROPOSED column of
 * the side-by-side), with its true 300px render at `1192:40`.
 */
const PAD_X = 70
const PAD_Y = 60
const CONTENT_WIDTH = OG_SIZE.width - PAD_X * 2

const DATE_SIZE = 34
const TITLE_SIZE_MAX = 78
const TITLE_LINE_HEIGHT = 84
const SUPPORT_SIZE = 34
const VENUE_SIZE = 38
/** 26px is 6.5px at the share size — below that the line stops being readable,
 *  so a truly enormous venue+city clips instead of shrinking into decoration. */
const VENUE_SIZE_MIN = 26
const DOMAIN_SIZE = 32
/** Keeps the venue line clear of the wordmark on the opposite edge. */
const FOOTER_GAP = 40

/**
 * The title is the one element whose length varies without bound, and it wraps
 * rather than shrinking to one line. Two lines is what the layout budgets; a
 * longer bill steps the size down instead of pushing the venue off the canvas.
 */
const TITLE_MAX_LINES = 2
const TITLE_SIZE_MIN = 48
/**
 * Wrapped text never fills its lines completely — a line breaks at the last
 * word that fits, so some of the width is always left behind. 0.9 is the
 * allowance, checked against a real long-title render rather than assumed.
 */
const WRAP_EFFICIENCY = 0.9

const WORDMARK = 'psychichomily.com'

interface ShowData {
  title?: string
  event_date: string
  is_sold_out: boolean
  is_cancelled: boolean
  venues: Array<{ name: string; city: string; state: string; timezone?: string | null }>
  artists: Array<{ name: string; is_headliner?: boolean | null }>
}

function formatDate(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    timeZone: resolveShowTimezone(state, timezone), // venue-local for share card (PSY-986)
  })
}

export default async function Image({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params

  const [{ fonts, degraded }, show] = await Promise.all([
    loadBrandFontsOrDefault(),
    fetchShow(slug),
  ])

  if (!show) return ogFallbackCard(fonts)

  const headliner =
    show.artists?.find(a => a.is_headliner)?.name || show.artists?.[0]?.name || 'Live Music'
  const venue = show.venues?.[0]
  const venueName = venue?.name || 'TBA'
  const showDate = formatDate(show.event_date, venue?.state, venue?.timezone)
  const displayTitle = show.title || `${headliner} at ${venueName}`
  const otherArtists = show.artists?.filter(a => a.name !== headliner).map(a => a.name)

  // City folded into the venue line — the structural change. At 300px a
  // separate city line was 5.5px, i.e. decoration that was carrying meaning.
  const venueLine = venue?.city
    ? `${venueName} · ${venue.city}, ${venue.state}`
    : venueName

  const titleSize = fitFontSize(
    displayTitle,
    'satoshiBold',
    CONTENT_WIDTH * TITLE_MAX_LINES * WRAP_EFFICIENCY,
    TITLE_SIZE_MAX,
    TITLE_SIZE_MIN
  )

  // The footer is a single row, so a long venue line and the wordmark compete
  // for one width. Step the venue down rather than let it wrap — folding the
  // city in made this line much longer, and "The Fillmore New Orleans · New
  // Orleans, LA" wraps at full size, dropping the state onto its own line and
  // shoving the wordmark off the edge.
  //
  // The wordmark is measured in MONO because that is the face it renders in;
  // measuring it as sans under-reserves its width by ~60px.
  const venueSize = fitFontSize(
    venueLine,
    'satoshiMedium',
    CONTENT_WIDTH - measureMono(WORDMARK, DOMAIN_SIZE) - FOOTER_GAP,
    VENUE_SIZE,
    VENUE_SIZE_MIN
  )

  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          backgroundColor: OG_COLORS.background,
          padding: `${PAD_Y}px ${PAD_X}px`,
          position: 'relative',
        }}
      >
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
            }}
          >
            <div
              style={{
                color: '#ef4444',
                fontFamily: OG_FONT_FAMILY.sans,
                fontSize: 96,
                fontWeight: 700,
                letterSpacing: '0.1em',
                opacity: 0.3,
                transform: 'rotate(-15deg)',
              }}
            >
              CANCELLED
            </div>
          </div>
        )}

        <div style={{ display: 'flex', alignItems: 'center', gap: 20 }}>
          <div
            style={{
              display: 'flex',
              fontFamily: OG_FONT_FAMILY.mono,
              fontSize: DATE_SIZE,
              color: OG_COLORS.primary,
            }}
          >
            {showDate}
          </div>
          {show.is_sold_out && (
            <div
              style={{
                display: 'flex',
                fontFamily: OG_FONT_FAMILY.sans,
                fontWeight: 700,
                backgroundColor: '#ef4444',
                color: '#ffffff',
                fontSize: 22,
                padding: '6px 16px',
                borderRadius: 6,
                letterSpacing: '0.05em',
              }}
            >
              SOLD OUT
            </div>
          )}
        </div>

        {/* Clipped rather than allowed to bleed: the title is measured, but a
            script outside the font subset cannot be measured exactly. */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12, overflow: 'hidden' }}>
          <div
            style={{
              display: 'flex',
              fontFamily: OG_FONT_FAMILY.sans,
              fontWeight: 700,
              fontSize: titleSize,
              lineHeight: `${Math.round((TITLE_LINE_HEIGHT / TITLE_SIZE_MAX) * titleSize)}px`,
              color: OG_COLORS.foreground,
            }}
          >
            {displayTitle}
          </div>
          {otherArtists && otherArtists.length > 0 && (
            <div
              style={{
                display: 'flex',
                fontFamily: OG_FONT_FAMILY.sans,
                fontWeight: 400,
                fontSize: SUPPORT_SIZE,
                color: OG_COLORS.mutedForeground,
              }}
            >
              with {otherArtists.slice(0, 4).join(', ')}
              {otherArtists.length > 4 ? ` + ${otherArtists.length - 4} more` : ''}
            </div>
          )}
        </div>

        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'flex-end',
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              display: 'flex',
              fontFamily: OG_FONT_FAMILY.sans,
              fontWeight: 500,
              fontSize: venueSize,
              color: OG_COLORS.foreground,
              whiteSpace: 'nowrap',
            }}
          >
            {venueLine}
          </div>
          <div
            style={{
              display: 'flex',
              fontFamily: OG_FONT_FAMILY.mono,
              fontSize: DOMAIN_SIZE,
              color: OG_COLORS.primary,
              whiteSpace: 'nowrap',
            }}
          >
            {WORDMARK}
          </div>
        </div>
      </div>
    ),
    {
      ...OG_SIZE,
      fonts,
      // A show's details do not change often, but they DO change — a cancelled
      // or sold-out show must not keep advertising itself for a day. An hour
      // matches the data revalidate below. A card drawn without the brand fonts
      // is held shorter still: its fit budgets assumed Satoshi's metrics.
      headers: {
        'cache-control': ogCacheControl(degraded ? 60 : SHOW_REVALIDATE),
      },
    }
  )
}

const SHOW_REVALIDATE = 3600

/**
 * Deliberate fail-open, classified during the PSY-1630 sweep: every failure
 * here returns null and the route falls through to the branded fallback card.
 * An OG image that renders generically is better than a share preview that
 * errors, and there is no reader-visible content behind this to protect.
 */
async function fetchShow(slug: string): Promise<ShowData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/shows/${encodeURIComponent(slug)}`, {
      next: { revalidate: SHOW_REVALIDATE },
    })
    // `await` inside the try is load-bearing: `return res.json()` would adopt
    // the promise after the block exits, so a malformed body would reject past
    // this catch and 500 the route instead of reaching the fallback card.
    if (res.ok) return asShow(await res.json())
    if (res.status >= 500) {
      Sentry.captureMessage(`Show OG: API returned ${res.status}`, {
        level: 'error',
        tags: { service: 'og-image' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      tags: { service: 'og-image' },
      extra: { slug },
    })
  }
  return null
}

/**
 * A 200 is not proof of the right shape. `event_date` goes straight into date
 * maths that throws on `undefined`, so a redirect or an error page served with
 * 200 would 500 the route rather than reaching the fallback card.
 */
function asShow(body: unknown): ShowData | null {
  const show = body as ShowData | null
  if (!show || typeof show !== 'object') return null
  if (typeof show.event_date !== 'string') return null
  return show
}
