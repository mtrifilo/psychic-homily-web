import type { ShowResponse } from '../types'

/**
 * The flyer URL to render, or null when there is nothing renderable.
 *
 * `image_url` is writable by any email-verified user (a show's submitter can
 * PUT it) and neither end validates it as a URL, so this decides rather than
 * trusts: the value must parse, and the scheme must be http(s). Anything else
 * collapses the plate instead of emitting an `<img src>` built from an
 * attacker-chosen string. The `.trim()` is belt and braces, not the guard.
 * `new URL` already ignores surrounding whitespace; the guard is the parse and
 * the scheme check.
 *
 * A SCHEME-LESS value ("flyers.example/poster.jpg", the shape a submitter
 * actually types) is deliberately NOT repaired into "https://…", even though
 * `ShowHeader` does exactly that repair for `ticket_url` a few lines away. Two
 * reasons to keep them different. A ticket link is a destination the reader
 * chooses to follow and can see before they do; a flyer is a subresource every
 * viewer's browser fetches automatically, so guessing a host for it is a guess
 * about a security-relevant value. And `lib/seo/jsonld.ts` publishes the same
 * column as a machine-readable image claim under the same strict rule, so
 * repairing it here alone would make the page and its structured data disagree
 * about whether this show has a flyer. The real fix for a submitter who types
 * a bare host is validation at the WRITE boundary, which this ticket does not
 * touch.
 *
 * Returns `url.href`, not the input, for the same reason `absoluteHttpUrl` in
 * `lib/seo/jsonld.ts` does: `"https://x/a\nb"` parses, and the raw form would
 * put control characters into the markup.
 */
export function flyerImageSrc(
  show: Pick<ShowResponse, 'image_url'>
): string | null {
  const raw = show.image_url?.trim()
  if (!raw) return null
  try {
    const url = new URL(raw)
    return url.protocol === 'http:' || url.protocol === 'https:'
      ? url.href
      : null
  } catch {
    return null
  }
}

/**
 * Who to credit the flyer to, or null when we cannot say honestly.
 *
 * The only provenance the show row carries is `source_venue`, the venue SLUG a
 * discovery run scraped the listing from. That is a slug, not a display name,
 * so it is resolved against the show's own venues and the venue's NAME is what
 * gets printed. A slug that matches nothing (a venue swapped after import, a
 * hand-entered value) credits nobody rather than printing "flyer via
 * valley-bar". A user-submitted show has no source at all, so it gets no
 * credit line.
 *
 * Deliberately conservative: a credit is a factual claim about where an image
 * came from, and there is no per-image provenance column to make a stronger
 * one from.
 */
export function flyerCredit(
  show: Pick<ShowResponse, 'source_venue' | 'venues'>
): string | null {
  const sourceSlug = show.source_venue?.trim()
  if (!sourceSlug) return null
  const sourceVenue = show.venues.find(venue => venue.slug === sourceSlug)
  return sourceVenue?.name?.trim() || null
}
