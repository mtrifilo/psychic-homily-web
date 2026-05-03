'use client'

/**
 * CollectionCoverImage (PSY-554)
 *
 * Shared cover-image renderer for the collection detail header and the
 * browse list card. Internalizes two small but easy-to-forget concerns:
 *
 *   1. Null/empty `url` — render the supplied `fallback` instead of a
 *      broken or empty `<img>`.
 *   2. `<img>` `onError` — when the URL resolves to a 404 (or any load
 *      failure), swap to the same `fallback`. Without this, a stale or
 *      moved image leaves the cover slot blank with only alt text.
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

import { useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

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

  // Track the URL alongside the error flag so the error state resets
  // automatically when the URL changes (e.g. after an edit). Storing
  // both in one piece of state — rather than syncing via useEffect —
  // follows the React-recommended "reset state on prop change" pattern
  // (https://react.dev/learn/you-might-not-need-an-effect#resetting-all-state-when-a-prop-changes).
  const [errorState, setErrorState] = useState<{
    url: string
    errored: boolean
  }>({ url: trimmed, errored: false })

  // If the URL changed since we last recorded an error, reset during
  // render — avoids a "load fallback briefly then flicker to image"
  // round-trip from a useEffect-based reset.
  const errored = errorState.url === trimmed && errorState.errored
  const showImage = trimmed.length > 0 && !errored

  return (
    <div className={cn('overflow-hidden', className)}>
      {showImage ? (
        /* eslint-disable-next-line @next/next/no-img-element */
        <img
          src={trimmed}
          alt={alt}
          className="h-full w-full object-cover"
          onError={() => setErrorState({ url: trimmed, errored: true })}
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
