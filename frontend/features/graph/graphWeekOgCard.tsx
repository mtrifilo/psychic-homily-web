import { ImageResponse } from 'next/og'
import { OG_COLORS, OG_FONT_FAMILY, OG_SIZE } from '@/lib/og/brand'
import {
  OG_FALLBACK_CACHE_SECONDS,
  loadBrandFontsOrDefault,
  ogCacheControl,
  ogFallbackCard,
} from '@/lib/og/response'

import { fetchGraphOverview, GRAPH_OVERVIEW_REVALIDATE } from './graphOverviewApi'
import { buildSceneMap } from './sceneMap'
import { formatGraphWeekCounts, formatGraphWeekRange, resolveGraphWeek } from './graphWeek'
import {
  COUNTS_TRACKING,
  EYEBROW_SIZE,
  EYEBROW_TEXT,
  EYEBROW_TRACKING,
  HEADLINE_GAP,
  HEADLINE_LINE_HEIGHT,
  HEADLINE_SIZE,
  HEADLINE_TEXT,
  MOTIF_CONNECTOR_OPACITY,
  MOTIF_CONNECTOR_WIDTH,
  MOTIF_DOT_OPACITY,
  MOTIF_DOT_RADIUS,
  MOTIF_FADE_CLEAR_STOP,
  MOTIF_FADE_OPAQUE_STOP,
  MOTIF_NEW_DOT_OPACITY,
  MOTIF_NEW_DOT_RADIUS,
  PAD_X,
  PAD_Y,
  RANGE_GAP,
  RANGE_SIZE,
  RANGE_TRACKING,
  TEXT_WIDTH,
  buildGraphWeekMotif,
  fitCountsSize,
  type GraphWeekMotif,
} from './graphWeekOgLayout'

/**
 * The "this week in the graph" share card (PSY-1738), from the approved mock.
 *
 * Rendered SERVER-SIDE from the same nightly snapshot the map draws, never
 * captured from a canvas: the window is a property of the snapshot, so the card
 * has to be derivable from it alone or the two surfaces can disagree about how
 * many artists arrived.
 *
 * Split out of the route for the family's reason — the route file stays the
 * five lines of Next's file convention — and because this is the only place
 * `next/og` appears in the feature, which keeps the geometry and the window
 * maths in modules a unit test can import.
 */
export async function renderGraphWeekOgCard(): Promise<ImageResponse | Response> {
  const [{ fonts, degraded }, overview] = await Promise.all([
    loadBrandFontsOrDefault(),
    fetchGraphOverview('og-image'),
  ])

  // Three different absences, one answer. No snapshot yet (a fresh install's
  // 503), a payload we cannot decode, and a snapshot we cannot date all leave a
  // reader in the same place — and the branded fallback is the family's answer
  // to it, because an unfurler handed a 500 shows nothing at all and some
  // clients then cache the miss.
  const map = overview ? buildSceneMap(overview) : null
  const week = map ? resolveGraphWeek(map) : null
  if (!map || !week) return ogFallbackCard(fonts)

  const counts = formatGraphWeekCounts(week)
  const range = formatGraphWeekRange(week.start, week.end)
  const countsSize = fitCountsSize(counts)
  const motif = buildGraphWeekMotif(map, week)

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
        {/* PAINT ORDER IS THE WHOLE COMPOSITION. Satori paints in document
            order with no z-index, so the motif must come first, its fade
            second, and the text last — reorder any two and the text either
            sits under the dots or the dots vanish entirely. */}
        <MapMotif motif={motif} />
        <div
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            display: 'flex',
            // Opaque brand background under the text, clear over the motif's
            // dense middle. Without it the eyebrow's tail and the counts line
            // cross into the dots and stop being readable at 300px.
            backgroundImage: `linear-gradient(to right, ${OG_COLORS.background} ${MOTIF_FADE_OPAQUE_STOP}%, rgba(13, 8, 5, 0) ${MOTIF_FADE_CLEAR_STOP}%)`,
          }}
        />

        <div
          style={{
            display: 'flex',
            fontFamily: OG_FONT_FAMILY.mono,
            fontSize: EYEBROW_SIZE,
            letterSpacing: EYEBROW_TRACKING,
            color: OG_COLORS.foreground,
          }}
        >
          {EYEBROW_TEXT}
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', width: TEXT_WIDTH }}>
          <div
            style={{
              display: 'flex',
              fontFamily: OG_FONT_FAMILY.sans,
              fontWeight: 700,
              fontSize: HEADLINE_SIZE,
              lineHeight: `${HEADLINE_LINE_HEIGHT}px`,
              color: OG_COLORS.foreground,
            }}
          >
            {HEADLINE_TEXT}
          </div>
          {/* `whiteSpace: nowrap` is what the step-down in `fitCountsSize`
              exists for: this line must stay one run, so it shrinks rather
              than wrapping `+12 ARTISTS` across two lines. */}
          <div
            style={{
              display: 'flex',
              marginTop: HEADLINE_GAP,
              fontFamily: OG_FONT_FAMILY.mono,
              fontSize: countsSize,
              letterSpacing: COUNTS_TRACKING,
              color: OG_COLORS.primary,
              whiteSpace: 'nowrap',
            }}
          >
            {counts}
          </div>
          <div
            style={{
              display: 'flex',
              marginTop: RANGE_GAP,
              fontFamily: OG_FONT_FAMILY.mono,
              fontSize: RANGE_SIZE,
              letterSpacing: RANGE_TRACKING,
              color: OG_COLORS.mutedForeground,
              whiteSpace: 'nowrap',
            }}
          >
            {range}
          </div>
        </div>
      </div>
    ),
    {
      ...OG_SIZE,
      fonts,
      // The card is a property of the SNAPSHOT, which changes once a night, so
      // it may be held for the same window the data is. A card drawn without the
      // brand fonts is held to the short window regardless: its fit budgets were
      // computed from Satoshi's metrics, so it may be visually wrong, and a
      // wrong card should expire rather than sit in the CDN for an hour.
      headers: {
        'cache-control': ogCacheControl(
          degraded ? OG_FALLBACK_CACHE_SECONDS : GRAPH_OVERVIEW_REVALIDATE
        ),
      },
    }
  )
}

/**
 * The map motif, as ONE inline `<svg>`.
 *
 * Satori serialises an `<svg>` subtree to a data URI and hands it to resvg as an
 * image, so this is a single element to lay out however many dots it holds —
 * where the same dots as absolutely-positioned `<div>`s would be a thousand
 * boxes through Yoga on every render. The geometry is already projected and
 * capped by `buildGraphWeekMotif`; nothing here computes anything.
 *
 * `preserveAspectRatio="none"` is deliberate: the viewBox IS the card, one unit
 * per px, so there is no aspect ratio to preserve and no scaling to reason
 * about. The fit happened in the projection.
 */
function MapMotif({ motif }: { motif: GraphWeekMotif }) {
  return (
    <svg
      width={OG_SIZE.width}
      height={OG_SIZE.height}
      viewBox={`0 0 ${OG_SIZE.width} ${OG_SIZE.height}`}
      preserveAspectRatio="none"
      style={{ position: 'absolute', top: 0, left: 0 }}
    >
      {/* Connectors first so a dot always sits ON TOP of its own lines. */}
      <g
        stroke={OG_COLORS.primary}
        strokeWidth={MOTIF_CONNECTOR_WIDTH}
        strokeOpacity={MOTIF_CONNECTOR_OPACITY}
      >
        {motif.connectors.map((line, index) => (
          <line key={index} x1={line.x1} y1={line.y1} x2={line.x2} y2={line.y2} />
        ))}
      </g>
      <g fill={OG_COLORS.mutedForeground} fillOpacity={MOTIF_DOT_OPACITY}>
        {motif.dots.map((dot, index) => (
          <circle key={index} cx={dot.x} cy={dot.y} r={MOTIF_DOT_RADIUS} />
        ))}
      </g>
      <g fill={OG_COLORS.primary} fillOpacity={MOTIF_NEW_DOT_OPACITY}>
        {motif.newDots.map((dot, index) => (
          <circle key={index} cx={dot.x} cy={dot.y} r={MOTIF_NEW_DOT_RADIUS} />
        ))}
      </g>
    </svg>
  )
}
