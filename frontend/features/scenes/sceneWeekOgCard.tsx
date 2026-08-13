import { ImageResponse } from 'next/og'
import { OG_COLORS, OG_FONT_FAMILY, OG_SIZE } from '@/lib/og/brand'
import {
  loadBrandFontsOrDefault,
  ogCacheControl,
  ogFallbackCard,
} from '@/lib/og/response'
import { fitFontSize, fitItemList, monoBaselineLift } from '@/lib/og/textFit'
import { fetchSceneWeek } from './sceneWeekApi'
// The windows come from the shared module the FETCH reads them from, not from
// a week-local alias. The card's cache-control and the data's revalidate have
// to be the same number — see the header comment at the bottom of this file —
// and one editable name per window is what keeps them that way.
import {
  ARCHIVED_PERIOD_REVALIDATE,
  CURRENT_PERIOD_REVALIDATE,
} from './scenePeriodApi'
import {
  countShows,
  formatShowCountLine,
  formatWeekRangeCompact,
  resolveRequestedWeek,
} from './sceneWeek'
import {
  CITY_SIZE_MAX,
  CITY_SIZE_MIN,
  CITY_STATE_GAP,
  CITY_TRACKING_EM,
  COUNT_SIZE,
  FOOTER_SIZE,
  HEADLINE_GAP,
  PAD_X,
  PAD_Y,
  RANGE_SIZE,
  RANGE_TRACKING,
  ROOMS_MAX_WIDTH,
  STATE_SIZE,
  STATE_TRACKING,
  WORDMARK,
  cityMaxWidth,
} from './sceneWeekOgLayout'

/** Degenerate case for the footer: not even one room name fits. */
const roomsOverflowLabel = (count: number) =>
  `${count} ${count === 1 ? 'room' : 'rooms'} tracked`

/**
 * Render the weekly-city share card.
 *
 * Shared by the rolling `/week` route and the archived `/{iso-week}` route so
 * the two cannot drift; the only difference between them is which week the
 * backend resolves.
 */
export async function renderSceneWeekOgCard(
  slug: string,
  week?: string
): Promise<ImageResponse | Response> {
  const requestedWeek = resolveRequestedWeek(week)

  // A junk segment gets a real 404, not a card. Rendering one would mean paying
  // the most expensive response this route has — wasm instantiation, four font
  // parses and a PNG encode — for a URL whose page 404s anyway, on an endpoint
  // anyone can hit with unlimited distinct paths.
  if (requestedWeek === null) return new Response(null, { status: 404 })

  const [{ fonts, degraded }, data] = await Promise.all([
    loadBrandFontsOrDefault(),
    fetchSceneWeek(slug, requestedWeek, 'og-image'),
  ])

  if (!data) return ogFallbackCard(fonts)

  const total = countShows(data)
  const range = formatWeekRangeCompact(data.start_date, data.end_date)
  const countLine = formatShowCountLine(total, data.is_current_week)

  // The city has to survive being skimmed at thumbnail size, but "Columbus" and
  // "Portland" are genuinely ambiguous to someone arriving cold from a shared
  // link — so the state rides alongside at metadata scale rather than blunting
  // the headline, matching the page's h1 (PSY-1576 addendum).
  const state = data.state?.trim() ?? ''
  const citySize = fitFontSize(
    data.city,
    'satoshiBold',
    cityMaxWidth(state),
    CITY_SIZE_MAX,
    CITY_SIZE_MIN,
    CITY_TRACKING_EM
  )

  const roomsLine = fitItemList(
    (data.tracked_venues ?? []).map(v => v.name),
    'satoshiRegular',
    FOOTER_SIZE,
    ROOMS_MAX_WIDTH,
    roomsOverflowLabel
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
        }}
      >
        <div
          style={{
            display: 'flex',
            fontFamily: OG_FONT_FAMILY.mono,
            fontSize: RANGE_SIZE,
            color: OG_COLORS.primary,
            letterSpacing: RANGE_TRACKING,
          }}
        >
          {range}
        </div>

        {/* `overflow: hidden` is the backstop for a text run this card cannot
            measure exactly — a non-Latin city name falls outside the subset and
            is deliberately over-estimated, but if that estimate is ever wrong
            the card should clip, not bleed off the canvas. */}
        <div style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <div style={{ display: 'flex', alignItems: 'flex-end', gap: CITY_STATE_GAP }}>
            <div
              style={{
                fontFamily: OG_FONT_FAMILY.sans,
                fontWeight: 700,
                fontSize: citySize,
                color: OG_COLORS.foreground,
                letterSpacing: CITY_TRACKING_EM * citySize,
              }}
            >
              {data.city}
            </div>
            {state && (
              <div
                style={{
                  fontFamily: OG_FONT_FAMILY.mono,
                  fontSize: STATE_SIZE,
                  color: OG_COLORS.mutedForeground,
                  letterSpacing: STATE_TRACKING,
                  marginBottom: monoBaselineLift(citySize, STATE_SIZE),
                }}
              >
                {state}
              </div>
            )}
          </div>
          <div
            style={{
              display: 'flex',
              fontFamily: OG_FONT_FAMILY.sans,
              fontWeight: 500,
              fontSize: COUNT_SIZE,
              color: OG_COLORS.foreground,
              marginTop: HEADLINE_GAP,
            }}
          >
            {countLine}
          </div>
        </div>

        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            fontSize: FOOTER_SIZE,
            overflow: 'hidden',
          }}
        >
          {/* Rendered even when there are no rooms to name: `space-between`
              needs a left-hand child, or the wordmark slides over to it. */}
          <div
            style={{
              display: 'flex',
              fontFamily: OG_FONT_FAMILY.sans,
              fontWeight: 400,
              color: OG_COLORS.mutedForeground,
            }}
          >
            {roomsLine ?? ''}
          </div>
          <div
            style={{
              display: 'flex',
              fontFamily: OG_FONT_FAMILY.mono,
              color: OG_COLORS.primary,
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
      // Only a week that has actually ENDED may be cached hard, and the backend
      // is what says so — the same `is_past_week` the data fetch above picks
      // its own window from, so the rendered card and the numbers on it can
      // never disagree about how long they are good for. A card drawn without
      // the brand fonts is held to the short window regardless: its fit budgets
      // were computed from Satoshi's metrics, so it may be visually wrong, and
      // a wrong card should expire rather than sit in the CDN for a day.
      headers: {
        'cache-control': ogCacheControl(
          // `=== true` for the same reason the data fetch uses it: this reads an
          // untrusted payload, and a non-boolean must not pin a live card in
          // the CDN for a day.
          !degraded && data.is_past_week === true
            ? ARCHIVED_PERIOD_REVALIDATE
            : CURRENT_PERIOD_REVALIDATE
        ),
      },
    }
  )
}
