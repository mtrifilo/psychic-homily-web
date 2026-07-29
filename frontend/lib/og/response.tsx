import { ImageResponse } from 'next/og'
import * as Sentry from '@sentry/nextjs'
import {
  OG_COLORS,
  OG_FONT_FAMILY,
  OG_SIZE,
  loadBrandFonts,
  type OgFont,
} from './brand'

/**
 * A font load that may have degraded.
 *
 * `degraded` is not cosmetic bookkeeping: every fit budget on a card is
 * computed from Satoshi's metrics, so a card drawn in Satori's fallback face
 * is measured against the wrong numbers and can overrun. Callers use this to
 * refuse to cache such a card for long.
 */
export interface BrandFontsResult {
  fonts: OgFont[] | undefined
  degraded: boolean
}

/**
 * One-shot reporting latch.
 *
 * A broken font asset fails on EVERY request, and the loader deliberately does
 * not retain the rejection, so without this a persistent failure converts
 * request volume 1:1 into Sentry events — burning the quota exactly when the
 * signal matters. One report per isolate is enough to raise the alarm.
 */
let fontFailureReported = false

/**
 * Load the brand fonts, degrading to Satori's bundled default instead of
 * throwing.
 *
 * The fonts are route assets, so this only fails if a deployment is broken —
 * but an unhandled rejection here would take the whole card down with a 500,
 * losing the branded fallback the failure path exists to guarantee. An
 * off-brand card still beats a broken link preview.
 */
export async function loadBrandFontsOrDefault(): Promise<BrandFontsResult> {
  try {
    return { fonts: await loadBrandFonts(), degraded: false }
  } catch (error) {
    if (!fontFailureReported) {
      fontFailureReported = true
      Sentry.captureException(error, {
        tags: { service: 'og-image' },
        extra: { stage: 'load-brand-fonts' },
      })
    }
    return { fonts: undefined, degraded: true }
  }
}

/**
 * `Cache-Control` for a rendered card.
 *
 * Not optional politeness — without it these routes are re-rendered on every
 * single unfurl. `next/og` builds its own outer response and overwrites
 * `@vercel/og`'s long-lived default with `public, max-age=0, must-revalidate`,
 * so the CDN is forbidden from serving a stored copy. Every Bluesky, Discord,
 * Slack or iMessage preview then boots the edge function, instantiates the
 * ~1.4MB resvg wasm, re-runs Satori layout and re-encodes a 1200×630 PNG.
 *
 * Caching the DATA fetch (`next: { revalidate }`) only saves the API round
 * trip, which is the cheap half. This caches the expensive half. `next/og`
 * merges caller headers over its default, so passing this wins.
 */
export function ogCacheControl(seconds: number): string {
  return `public, max-age=0, s-maxage=${seconds}, stale-while-revalidate=${seconds}`
}

/**
 * How long a failed render may be served from cache.
 *
 * Short on purpose: a fallback card is an error state, and caching one for as
 * long as a real card would turn a momentary API blip into a day of branded
 * blank previews for that URL.
 */
export const OG_FALLBACK_CACHE_SECONDS = 60

/**
 * The branded card shown when the data behind a share card cannot be had.
 *
 * Shared by the whole card family so a failure looks deliberate rather than
 * broken — a wordmark on the brand background reads as "nothing to show here",
 * where a 500 gives the unfurler nothing at all and some clients then cache the
 * miss.
 */
export function ogFallbackCard(fonts: OgFont[] | undefined): ImageResponse {
  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          backgroundColor: OG_COLORS.background,
          color: OG_COLORS.foreground,
          fontFamily: OG_FONT_FAMILY.sans,
          fontSize: 64,
          fontWeight: 700,
        }}
      >
        Psychic Homily
      </div>
    ),
    {
      ...OG_SIZE,
      fonts,
      headers: { 'cache-control': ogCacheControl(OG_FALLBACK_CACHE_SECONDS) },
    }
  )
}
