import { ImageResponse } from 'next/og'
import * as Sentry from '@sentry/nextjs'
import {
  OG_COLORS,
  OG_FONT_FAMILY,
  OG_SIZE,
  loadBrandFonts,
  type OgFont,
} from './brand'
import { isShowPast, type ShowTimingInput } from '@/lib/utils/showTiming'

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
 * One-shot reporting latches.
 *
 * A broken font asset fails on EVERY request, and the loader deliberately does
 * not retain the rejection, so without these a persistent failure converts
 * request volume 1:1 into Sentry events — burning the quota exactly when the
 * signal matters. One report per isolate is enough to raise the alarm.
 *
 * The fallback gets its own latch because it is a different incident: the cards
 * still look right, so nothing about the output would reveal that every
 * non-Latin name has quietly gone back to being fetched from Google mid-render.
 * It is only visible if something says so.
 */
let fontFailureReported = false
let fallbackMissingReported = false

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
    const { fonts, fallbackMissing } = await loadBrandFonts()
    if (fallbackMissing && !fallbackMissingReported) {
      fallbackMissingReported = true
      Sentry.captureException(new Error('OG script-coverage fallback font failed to load'), {
        tags: { service: 'og-image' },
        extra: { stage: 'load-fallback-fonts', impact: 'non-Latin text now fetches from Google' },
      })
    }
    // NOT `degraded`. That flag means the fit budgets may be wrong, and they are
    // computed from the brand faces, which loaded fine. A missing fallback costs
    // script coverage, not layout accuracy, so the card is still cacheable for
    // its normal window.
    return { fonts, degraded: false }
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
 * A day of margin between "the show is over" and "this card will never change
 * again".
 *
 * NOT a timezone allowance — deriving the boundary in the venue's own zone
 * removed the need for that. It is a CACHE-INVALIDATION one. `ogCacheControl`
 * sets stale-while-revalidate equal to s-maxage, so committing a card to the
 * long window commits the CDN to it for up to twice that: an admin who cancels
 * a show, fixes its date, or finally attaches a flyer the morning after would
 * not reach an iMessage or Discord unfurl for two days. Shows are edited most
 * right after they happen, so the margin covers exactly that window.
 *
 * Carried over from the rule this replaced, which subtracted the same day for
 * a different stated reason. Changing the number is a product call about how
 * stale a share preview may be, not a refactor.
 */
const SETTLED_MARGIN_MS = 24 * 60 * 60 * 1000

/**
 * Whether a show's card can never change again, and so may cache hard.
 *
 * Exported rather than inlined at the route so it can be tested at all: the
 * brand fonts are route assets that do not resolve under vitest, so every card
 * rendered there is `degraded` and takes the short window before this branch is
 * reached. A copy of the rule living in a test would pin the copy, not the
 * route — this is the one definition both use.
 *
 * Reads the venue-local boundary a margin into the past, which is the same
 * question the pre-`isShowPast` rule asked ("did this end more than a day ago")
 * with the UTC-versus-venue skew taken out of it.
 *
 * An unreadable `event_date` counts as settled here only because it cannot
 * reach this function: the route's `asShow` rejects one before rendering, and
 * a card that survives to be classified always has a parseable date. Do not
 * relax that guard without revisiting this, since a 48-hour freeze is the
 * expensive direction to be wrong in.
 */
export function isShowCardSettled(
  show: ShowTimingInput,
  now: Date = new Date()
): boolean {
  return isShowPast(show, new Date(now.getTime() - SETTLED_MARGIN_MS))
}

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
