import { ImageResponse } from 'next/og'
import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import { resolveShowTimezone } from '@/lib/utils/formatters'
import { OG_COLORS, OG_CONTENT_TYPE, OG_FONT_FAMILY, OG_SIZE } from '@/lib/og/brand'
import {
  OG_FALLBACK_CACHE_SECONDS,
  loadBrandFontsOrDefault,
  ogCacheControl,
  ogFallbackCard,
} from '@/lib/og/response'
import { loadRemoteImage } from '@/lib/og/remoteImage'
import {
  CONTENT_WIDTH,
  DATE_SIZE,
  DOMAIN_SIZE,
  HEADLINE_GAP,
  PAD_X,
  PAD_Y,
  PLATE_BOX_HEIGHT,
  PLATE_BOX_WIDTH,
  PLATE_GAP,
  SOLD_OUT_SIZE,
  SUPPORT_SIZE,
  TEXT_WIDTH_WITH_PLATE,
  TITLE_LINE_HEIGHT,
  TITLE_SIZE_MAX,
  VENUE_MAX_WIDTH,
  WORDMARK,
  buildVenueLine,
  fitPlate,
  fitTitleSize,
  fitVenueSize,
  venueOverflows,
} from '@/features/shows/showOgLayout'

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
 * fit: the city is folded into the venue line.
 *
 * Geometry and the fit rules live in `showOgLayout`, free of `next/og`, so the
 * budgets can be unit-tested against the values the card actually uses.
 */
interface ShowData {
  title?: string
  event_date: string
  is_sold_out: boolean
  is_cancelled: boolean
  image_url?: string | null
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

  // Sequential rather than parallel with the show fetch, because the URL to
  // fetch comes out of it. Bounded by `loadRemoteImage`'s own deadline, and any
  // failure returns null, which is simply the text-only card.
  const flyer = await loadRemoteImage(show.image_url)
  // One nullable value for "there is a flyer", carrying everything drawing it
  // needs. Two (the bytes and the geometry) would have to be re-narrowed
  // together at every use.
  const plate = flyer
    ? { ...fitPlate(flyer.width, flyer.height), src: flyer.data }
    : null
  // A flyer was expected and did not arrive — a timeout, a dead link, a format
  // the rasteriser cannot draw. The card is still correct, so it renders, but it
  // must not be cached like a complete one: `ogCacheControl` sets
  // stale-while-revalidate equal to s-maxage, so a past show would hold a
  // momentary blip for up to 48 hours. Same reasoning `ogFallbackCard` writes
  // down for its own error state.
  const flyerMissing = Boolean(show.image_url) && !flyer
  // The text column narrows only when there is actually a plate beside it, so a
  // flyer-less show renders exactly the card it rendered before.
  const textWidth = plate ? TEXT_WIDTH_WITH_PLATE : CONTENT_WIDTH

  const headliner =
    show.artists?.find(a => a.is_headliner)?.name || show.artists?.[0]?.name || 'Live Music'
  const venue = show.venues?.[0]
  const venueName = venue?.name || 'TBA'
  const showDate = formatDate(show.event_date, venue?.state, venue?.timezone)
  const displayTitle = show.title || `${headliner} at ${venueName}`
  const otherArtists = show.artists?.filter(a => a.name !== headliner).map(a => a.name)

  // City folded into the venue line — the structural change. At 300px a
  // separate city line was 5.5px, i.e. decoration that was carrying meaning.
  const venueLine = buildVenueLine(venueName, venue?.city, venue?.state)

  const titleSize = fitTitleSize(displayTitle, textWidth)
  // Beside a plate the footer STACKS, so the venue line gets the whole column
  // rather than sharing a row with the wordmark — kept inline it would have
  // 274px of a 640px column, and an ordinary venue name would clip on every
  // single flyer card. See TEXT_WIDTH_WITH_PLATE.
  const venueBudget = plate ? textWidth : VENUE_MAX_WIDTH
  const venueSize = fitVenueSize(venueLine, venueBudget)
  // Past the minimum size the line still overruns. It must CLIP rather than
  // run under the wordmark and push it off the canvas — the two elements this
  // retune exists to make legible are the two that would be destroyed.
  const venueClips = venueOverflows(venueLine, venueBudget)

  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'stretch',
          // Unconditional: without a plate this row has one in-flow child (the
          // CANCELLED overlay is absolute), and a gap between one child is
          // nothing.
          gap: PLATE_GAP,
          backgroundColor: OG_COLORS.background,
          padding: `${PAD_Y}px ${PAD_X}px`,
          position: 'relative',
        }}
      >
        {/* The plate: a fixed band, with the artwork letterboxed inside it at
            its own ratio. Fixed rather than shrink-to-fit so the text column's
            width — and therefore every fit budget above — is the same number
            for a portrait poster and a landscape one. */}
        {plate && (
          <div
            style={{
              display: 'flex',
              width: PLATE_BOX_WIDTH,
              height: PLATE_BOX_HEIGHT,
              flexShrink: 0,
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            {/* A bare <img> on purpose: Satori renders this, not the browser,
                so next/image has no meaning inside an OG card. */}
            <img
              /* Raw bytes, not a data URI — see `RemoteImage.data`. Satori's
                 resolver takes an ArrayBuffer here; React's DOM typings only
                 know about the browser's string `src`, hence the cast. */
              src={plate.src as unknown as string}
              alt=""
              width={plate.width}
              height={plate.height}
              style={{ borderRadius: 8 }}
            />
          </div>
        )}

        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'space-between',
            flex: 1,
            // Without this the column takes its intrinsic content width and
            // pushes the plate off the canvas instead of wrapping its own text.
            minWidth: 0,
          }}
        >
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
                  backgroundColor: OG_COLORS.destructive,
                  color: OG_COLORS.destructiveForeground,
                  fontSize: SOLD_OUT_SIZE,
                  padding: '4px 14px',
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
          {/* `flex: 1, minHeight: 0` is what makes the clip real. Without a
              basis this box is content-driven, so `overflow: hidden` never
              clipped vertically — a very long title simply grew the box and
              pushed the venue and wordmark off the canvas entirely. Titles are
              VARCHAR(500) in the schema and the discovery pipeline writes them
              without the handlers' 255 cap, so this is a reachable input. */}
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: HEADLINE_GAP,
              flex: 1,
              minHeight: 0,
              justifyContent: 'center',
              overflow: 'hidden',
            }}
          >
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

          {/* Two whole shapes rather than four coupled ternaries: the wordmark
              costs ~326px in mono, which the 640px plate column cannot spare
              beside a venue line, so beside a plate the footer stacks. */}
          <div
            style={{
              display: 'flex',
              overflow: 'hidden',
              ...(plate
                ? {
                    flexDirection: 'column',
                    justifyContent: 'flex-end',
                    alignItems: 'flex-start',
                    gap: 8,
                  }
                : {
                    flexDirection: 'row',
                    justifyContent: 'space-between',
                    alignItems: 'flex-end',
                  }),
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
                ...(venueClips ? { minWidth: 0, overflow: 'hidden' } : {}),
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
                // Past the venue's minimum size the row is over budget, and
                // `space-between` would push the RIGHT-hand element out. The
                // wordmark is the one thing that must never be what disappears.
                flexShrink: 0,
              }}
            >
              {WORDMARK}
            </div>
          </div>
        </div>

        {/* LAST on purpose. Satori paints in document order, so an overlay
            declared before the plate would be painted UNDER an opaque flyer and
            vanish over the left half of the card — losing the single most
            load-bearing fact on it. Over the text it reads the same as before,
            at 0.3 opacity. */}
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
                color: OG_COLORS.destructive,
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
      </div>
    ),
    {
      ...OG_SIZE,
      fonts,
      // Same rule the weekly card writes down: anything that can STILL CHANGE
      // gets the short window; only genuinely settled content caches hard. A
      // show that has not happened yet can be cancelled or sell out from the
      // admin at any moment, and `ogCacheControl` sets stale-while-revalidate
      // equal to s-maxage — so an hour here would mean up to two hours of
      // serving a card that says a sold-out show is available.
      //
      // A card drawn without the brand fonts is held shorter still: its fit
      // budgets assumed Satoshi's metrics, so it may be visually wrong. A card
      // whose flyer failed to load is held on the same footing — it is an
      // incomplete render, not a settled one.
      headers: {
        'cache-control': ogCacheControl(
          degraded || flyerMissing
            ? OG_FALLBACK_CACHE_SECONDS
            : showHasPassed(show.event_date)
              ? SETTLED_REVALIDATE
              : LIVE_REVALIDATE
        ),
      },
    }
  )
}

/** A show that has not happened yet can be cancelled or sell out at any time. */
const LIVE_REVALIDATE = 900
/** Once it is past, nothing about it changes. */
const SETTLED_REVALIDATE = 86400
/** The data fetch cannot know which it is, so it always asks for the fresh one. */
const SHOW_REVALIDATE = LIVE_REVALIDATE

/**
 * Compared in UTC, which can run a day ahead of the venue's own clock. The
 * error direction is "treat it as still live", i.e. cache it less — the
 * harmless one.
 */
function showHasPassed(eventDate: string): boolean {
  const at = new Date(eventDate).getTime()
  if (Number.isNaN(at)) return false
  return at < Date.now() - 24 * 60 * 60 * 1000
}

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
  // A string is not enough: `event_date: "n/a"` passes a typeof check and then
  // renders the literal "Invalid Date" as the card's date line — the exact
  // wrong-but-plausible outcome this guard exists to prevent.
  if (Number.isNaN(new Date(show.event_date).getTime())) return null
  return show
}
