import { ImageResponse } from 'next/og'
import { OG_COLORS, OG_FONT_FAMILY, OG_SIZE } from '@/lib/og/brand'
import {
  loadBrandFontsOrDefault,
  ogCacheControl,
  ogFallbackCard,
} from '@/lib/og/response'
import { fitFontSize, fitItemList, monoBaselineLift } from '@/lib/og/textFit'
import {
  ARCHIVED_WEEK_REVALIDATE,
  CURRENT_WEEK_REVALIDATE,
  fetchSceneWeek,
} from './sceneWeekApi'
import { countShows, formatWeekRangeCompact, looksLikeISOWeek } from './sceneWeek'
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

/**
 * Next requires a STATIC alt, so it cannot say "this week" — the same card
 * component also serves archived weeks, where that would be false.
 */
export const SCENE_WEEK_OG_ALT =
  'Weekly show listing: city, dates, show count, and the rooms we track'

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
): Promise<ImageResponse> {
  // The archived route's segment is dynamic, so it also catches any unmatched
  // child path under a scene. A junk segment must reach the fallback card, NOT
  // silently fall through to the current week — that would hand a URL whose
  // page 404s a confident-looking card for a week it never asked for.
  const validWeek = week !== undefined && !looksLikeISOWeek(week) ? null : week

  const [fonts, data] = await Promise.all([
    loadBrandFontsOrDefault(),
    validWeek === null ? null : fetchSceneWeek(slug, validWeek, 'og-image'),
  ])

  if (!data) return ogFallbackCard(fonts)

  const total = countShows(data)
  const range = formatWeekRangeCompact(data.start_date, data.end_date)

  // "this week" is only true of the rolling week. An archived card carries its
  // date range directly above the count, so dropping the phrase reads correctly
  // for a week shared months later instead of claiming to be current.
  const period = data.is_current_week ? ' this week' : ''
  const countLine =
    total === 0
      ? `No shows${period}`
      : `${total} ${total === 1 ? 'show' : 'shows'}${period}`

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
    CITY_SIZE_MIN
  )

  const roomsLine = fitItemList(
    data.tracked_venues ?? [],
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

        <div style={{ display: 'flex', flexDirection: 'column' }}>
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
      // Matches the freshness of the data the card was drawn from, so the
      // picture and the numbers on it never disagree about how stale they are.
      headers: {
        'cache-control': ogCacheControl(
          data.is_current_week ? CURRENT_WEEK_REVALIDATE : ARCHIVED_WEEK_REVALIDATE
        ),
      },
    }
  )
}
