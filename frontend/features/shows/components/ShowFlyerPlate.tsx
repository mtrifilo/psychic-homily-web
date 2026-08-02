'use client'

import { useCallback } from 'react'

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
 *
 * A `figure` with a `figcaption`, not a div with a paragraph, because the
 * image carries no alt text (see below). Without the figure grouping, the
 * credit would reach a screen reader as a loose line of prose crediting an
 * object that is not in the accessibility tree at all.
 */
export function ShowFlyerPlate({
  src,
  credit,
  onError,
  className,
}: {
  /** A normalised absolute http(s) URL, from `flyerImageSrc` in ./showFlyer. */
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
  // Known false positive, accepted: an SVG with no intrinsic width reports
  // naturalWidth 0 in Firefox even when it rendered fine, so an SVG flyer can
  // collapse the column there. The alternatives (sniffing the extension,
  // reading the content type) are guesses about a URL, and flyers are
  // photographs and scans, not vector art.
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
    <figure data-testid="show-flyer-plate" className={className}>
      {/* eslint-disable-next-line @next/next/no-img-element -- flyer URLs are
          hotlinked venue/promoter hosts, outside next/image's remotePatterns
          allowlist (see next.config.ts). Same call as LibraryWallGrid. */}
      <img
        ref={reportPreHydrationFailure}
        src={src}
        /* Empty on purpose. The image restates the bill that is typeset
           immediately beside it, and its actual content (the poster art) is
           not something an alt string can hand over. A name like "Flyer for
           X" would announce a thing and then have nothing to say about it.
           The figcaption below is real text and is read normally, and the
           figure grouping is what ties it to this image. */
        alt=""
        data-testid="show-flyer-image"
        onError={onError}
        // The plate is last in the document and only moves left at `md`, so on
        // a phone it is genuinely below the fold. Lazy means a reader who
        // bounces before scrolling never hands their IP to the third-party
        // host at all. In the desktop two-column layout the plate is in the
        // viewport, so the browser fetches it immediately and nothing is lost.
        //
        // No `referrerPolicy` here on purpose: the document already sends
        // `Referrer-Policy: strict-origin-when-cross-origin` (next.config.ts),
        // and restating it per-image is a second copy that would silently keep
        // the old value after someone tightens the header.
        loading="lazy"
        className="block h-auto max-h-[70vh] w-auto max-w-full rounded-sm border border-border/60"
      />
      {credit && (
        <figcaption
          data-testid="show-flyer-credit"
          className="mt-2 font-mono text-[11px] tracking-wide text-muted-foreground"
        >
          flyer via {credit}
        </figcaption>
      )}
    </figure>
  )
}
