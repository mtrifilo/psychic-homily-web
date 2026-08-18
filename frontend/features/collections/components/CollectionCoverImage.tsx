'use client'

/**
 * CollectionCoverImage (PSY-554)
 *
 * Shared cover-image renderer for the collection detail header and the
 * browse list card. Internalizes three small but easy-to-forget concerns:
 *
 *   1. Null/empty `url` — render the supplied `fallback` instead of a
 *      broken or empty `<img>`.
 *   2. `<img>` `onError` — when the URL resolves to a 404 (or any load
 *      failure), swap to the same `fallback`. Without this, a stale or
 *      moved image leaves the cover slot blank with only alt text.
 *   3. A load failure that beat React to the element — the server-rendered
 *      case `onError` structurally cannot see. Read on mount from the
 *      element's own state; see the callback ref below.
 *
 * The component is intentionally layout-agnostic: callers supply the tile
 * shape (size, rounding, border, background) via `className` and the
 * fallback content as children-via-prop. Each cover surface keeps its
 * own visual language (h-16 mosaic on the browse card, h-24 typed
 * Library icon on the detail page) without this component picking sides.
 *
 * PSY-360's CollectionItemCard.tsx is the per-item-card analog; this
 * component covers the parallel "collection itself" cover sites.
 */

import { useCallback, useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
// The hook FILE, not the `@/lib/hooks/common` barrel — see the note in
// ShowFlyerPlate.tsx.
import { useImageFailedBeforeAttach } from '@/lib/hooks/common/useImageFailedBeforeAttach'

interface CollectionCoverImageProps {
  /** Cover URL from `Collection.cover_image_url`. May be null/empty/undefined. */
  url: string | null | undefined
  /** Alt text for the rendered `<img>`. Ignored when the fallback renders. */
  alt: string
  /**
   * Tile shape — size, rounding, border, background. The same classes
   * apply to both the image container and the fallback container so the
   * surrounding layout doesn't shift between states.
   */
  className?: string
  /**
   * What to render when `url` is null/empty OR the image fails to load.
   * Each cover site supplies its own (typed Lucide icon on detail,
   * entity-type mosaic on the browse card).
   */
  fallback: ReactNode
}

export function CollectionCoverImage({
  url,
  alt,
  className,
  fallback,
}: CollectionCoverImageProps) {
  const trimmed = url?.trim() ?? ''

  // Pin the errored flag to the URL it was recorded for so a later URL
  // change (e.g. after an edit) auto-resets without a useEffect. See
  // https://react.dev/learn/you-might-not-need-an-effect#resetting-all-state-when-a-prop-changes.
  const [errorState, setErrorState] = useState<{
    url: string
    errored: boolean
  }>({ url: trimmed, errored: false })

  const errored = errorState.errored && errorState.url === trimmed
  const showImage = trimmed.length > 0 && !errored

  const markFailed = useCallback(
    () => setErrorState({ url: trimmed, errored: true }),
    [trimmed]
  )

  // On this component the pre-attach gap is the common case rather than a
  // corner: `/collections/[slug]` prefetches the collection on the server and
  // hydrates it, so this `<img>` ships inside the initial HTML and the browser
  // is already fetching the cover before React ever sees the element. A dead
  // cover URL therefore 404s with no handler attached, and without the read
  // below the slot stays blank instead of showing the caller's fallback. See
  // the hook for the mechanism and its accepted Firefox/SVG false positive.
  const reportFailedBeforeAttach = useImageFailedBeforeAttach(markFailed)

  return (
    <div className={cn('overflow-hidden', className)}>
      {showImage ? (
        /* eslint-disable-next-line @next/next/no-img-element */
        <img
          // A new URL gets a NEW element rather than a reused one, the same
          // call ShowHeader makes for the flyer plate (PSY-1685). The ref
          // reads `complete` / `naturalWidth` off the node, and on a reused
          // element those still describe the PREVIOUS image at that moment
          // (the spec queues the image update as a microtask), so an edited
          // cover could be judged on the old one's failure. That would defeat
          // the URL-keyed state above, whose whole job is to give a
          // replacement its own chance.
          key={trimmed}
          ref={reportFailedBeforeAttach}
          src={trimmed}
          alt={alt}
          className="h-full w-full object-cover"
          onError={markFailed}
        />
      ) : (
        // Centered fallback container so an icon (or any short content)
        // sits in the middle of the tile. Mosaic-style fallbacks supply
        // their own grid wrapper inside the children.
        <div className="flex h-full w-full items-center justify-center">
          {fallback}
        </div>
      )}
    </div>
  )
}
