import type { components } from '@/types/api'
import type { ShowResponse } from '../types'

/**
 * The show fields these helpers read.
 *
 * The key NAMES are checked against the generated contract rather than only
 * against the hand-written mirror in `../types`. `bun run api:types` is a CI
 * gate, so if the backend renames `source_venue` the `Extract` below stops
 * yielding that key and this module fails to compile, instead of silently
 * returning null forever. The field TYPES still come from the mirror, which is
 * deliberately looser than the generated schema (see the note in `../types`)
 * and is what every caller actually holds.
 */
type ShowFlyerFields = Pick<
  ShowResponse,
  Extract<
    keyof components['schemas']['ShowResponse'],
    'image_url' | 'source_venue' | 'venues'
  >
>

/**
 * The flyer URL to render, or null when there is nothing renderable.
 *
 * `image_url` is writable by any email-verified user (a show's submitter can
 * PUT it) and neither end validates it as a URL, so this decides rather than
 * trusts: the value must parse, and the scheme must be https. Anything else
 * collapses the plate instead of emitting an `<img src>` built from an
 * attacker-chosen string. The `.trim()` is belt and braces, not the guard.
 * `new URL` already ignores surrounding whitespace; the guard is the parse and
 * the scheme check.
 *
 * HTTPS ONLY, though the backend accepts `http` (`utils/url.go`) and rows with
 * it exist. The app's own CSP is `img-src 'self' data: blob: https:` with no
 * `upgrade-insecure-requests` (next.config.ts), so an http flyer cannot load
 * on any deployed page: it would emit an `<img>` whose only possible outcome
 * is a CSP violation and a collapsed column one event later. Deciding it here
 * skips the round trip. `lib/og/remoteImage.ts` is https-only for the same
 * column and says so.
 *
 * A SCHEME-LESS value ("flyers.example/poster.jpg", the shape a submitter
 * actually types) is deliberately NOT repaired into "https://…", even though
 * `ShowHeader` does exactly that repair for `ticket_url` a few lines away. A
 * ticket link is a destination the reader chooses to follow and can see before
 * they do; a flyer is a subresource every viewer's browser fetches
 * automatically, so guessing a host for it is a guess about a
 * security-relevant value. The real fix for a submitter who types a bare host
 * is validation at the WRITE boundary, which this ticket does not touch.
 *
 * `absoluteHttpUrl` in `lib/seo/jsonld.ts` is a near-twin of this function
 * reading the same column for the same page's structured data, and the two are
 * NOT shared: this one is stricter (https only, for the CSP reason above)
 * because it feeds a browser fetch, and that one stays http-or-https because it
 * feeds a machine-readable claim nothing fetches under our CSP. They can
 * therefore disagree about a http:// flyer, which is intended. If they ever
 * need to agree, share the code rather than re-syncing two copies by hand.
 *
 * Returns `url.href`, not the input, for the same reason `absoluteHttpUrl`
 * does: `"https://x/a\nb"` parses, and the raw form would put control
 * characters into the markup.
 */
export function flyerImageSrc(
  show: Pick<ShowFlyerFields, 'image_url'>
): string | null {
  const raw = show.image_url?.trim()
  if (!raw) return null
  try {
    const url = new URL(raw)
    return url.protocol === 'https:' ? url.href : null
  } catch {
    return null
  }
}

/**
 * The display NAME of the venue a discovery run scraped this listing from, or
 * null when we cannot say honestly.
 *
 * The only provenance the show row carries is `source_venue`, the venue SLUG a
 * discovery run scraped the listing from. That is a slug, not a display name,
 * so it is resolved against the show's own venues and the venue's NAME is what
 * gets printed. A slug that matches nothing (a venue swapped after import, a
 * hand-entered value) credits nobody rather than printing "flyer via
 * valley-bar". A user-submitted show has no source at all, so it gets no
 * credit line.
 *
 * Deliberately conservative: a credit is a factual claim about where the
 * listing came from, and there is no stronger provenance column to make one
 * from. Two consumers, one rule and ONE name: the flyer plate's "flyer via
 * X" caption and the provenance footer's "Listing from X calendar" line must
 * never resolve the same slug to two different names, so they call the same
 * function rather than aliases of it.
 */
export function sourceVenueName(
  show: Pick<ShowFlyerFields, 'source_venue' | 'venues'>
): string | null {
  const sourceSlug = show.source_venue?.trim()
  if (!sourceSlug) return null
  const sourceVenue = show.venues.find(venue => venue.slug === sourceSlug)
  return sourceVenue?.name?.trim() || null
}
