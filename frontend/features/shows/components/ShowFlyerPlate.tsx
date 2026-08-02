'use client'

import { useCallback } from 'react'
import type { ShowResponse } from '../types'

/**
 * The show's flyer, at its own aspect ratio and uncropped.
 *
 * The poster carries the mood; the typeset bill beside it carries the
 * machine-readable truth. That split is why nothing here crops, overlays or
 * letterboxes the image: a flyer cropped to a grid ratio is a worse version of
 * the thing it is showing, and the information a crop would cost is already
 * rendered as text a few pixels away.
 *
 * Sizing is bounded on both axes instead of fixed on either. `max-w-full`
 * keeps a wide flyer inside its column; `max-h-[70vh]` keeps a pathologically
 * tall one (a full tour poster, a stitched Instagram story) from pushing the
 * whole page below the fold. Because both are MAXIMA on an `h-auto w-auto`
 * image, the browser scales to fit and the aspect ratio survives, with no
 * letterbox bars of the kind `object-contain` inside a fixed box produces.
 *
 * No intrinsic `width`/`height` is known before the bytes arrive, so the plate
 * does reflow once on load. Accepted rather than papered over: the alternative
 * is reserving a guessed aspect ratio, which is a visible wrong-shaped box on
 * every flyer that is not that shape.
 */
export function ShowFlyerPlate({
  src,
  credit,
  onError,
  className,
}: {
  /** A normalised absolute http(s) URL. See {@link flyerImageSrc}. */
  src: string
  /** Display name of the source the listing (and so the flyer) came from. */
  credit?: string | null
  /** Called when the image fails to load, so the caller can collapse the column. */
  onError: () => void
  /** Applied to the plate wrapper, for the caller's grid placement. */
  className?: string
}) {
  // The `onError` prop alone is not enough, and the gap is the common case
  // rather than a corner: this page is server-rendered, so the browser starts
  // fetching the flyer while parsing the HTML. A dead hotlink can therefore
  // 404 BEFORE React hydrates and attaches the handler, and that error event
  // is gone for good. What survives is the element's own state, so the ref
  // reads it once on mount: `complete` with a zero `naturalWidth` is exactly
  // "finished, and decoded nothing".
  //
  // A callback ref rather than an effect: this is reading the DOM node the
  // moment it exists, which is what a ref is for.
  const reportPreHydrationFailure = useCallback(
    (node: HTMLImageElement | null) => {
      if (node?.complete && node.naturalWidth === 0) onError()
    },
    [onError]
  )

  return (
    <div data-testid="show-flyer-plate" className={className}>
      {/* eslint-disable-next-line @next/next/no-img-element -- flyer URLs are
          hotlinked venue/promoter hosts, outside next/image's remotePatterns
          allowlist (see next.config.ts). Same call as LibraryWallGrid. */}
      <img
        ref={reportPreHydrationFailure}
        src={src}
        /* Empty on purpose. The image restates the bill that is typeset
           immediately beside it, and its actual content (the poster art) is
           not something an alt string can hand over. A name like "Flyer for
           X" would announce a thing and then have nothing to say about it. The
           credit line below is real text and is read normally. */
        alt=""
        data-testid="show-flyer-image"
        onError={onError}
        className="block h-auto max-h-[70vh] w-auto max-w-full rounded-sm border border-border/60"
      />
      {credit && (
        <p
          data-testid="show-flyer-credit"
          className="mt-2 font-mono text-[11px] tracking-wide text-muted-foreground"
        >
          flyer via {credit}
        </p>
      )}
    </div>
  )
}

/**
 * The flyer URL to render, or null when there is nothing renderable.
 *
 * `image_url` is writable by any email-verified user (a show's submitter can
 * PUT it) and the backend stores it untrimmed, so this normalises rather than
 * trusts: whitespace is stripped, the value must parse as a URL, and the
 * scheme must be http(s). Anything else collapses the plate instead of
 * emitting an `<img src>` built from an attacker-chosen string.
 *
 * Returns `url.href`, not the input, for the same reason `lib/seo/jsonld.ts`
 * does: `"https://x/a\nb"` parses, and the raw form would put control
 * characters into the markup.
 */
export function flyerImageSrc(show: Pick<ShowResponse, 'image_url'>): string | null {
  const raw = show.image_url?.trim()
  if (!raw) return null
  try {
    const url = new URL(raw)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : null
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
