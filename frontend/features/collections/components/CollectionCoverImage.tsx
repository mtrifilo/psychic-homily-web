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
// The hook FILE, not the `@/lib/hooks/common` barrel — see the note at the
// bottom of that barrel.
import { usePreAttachImageFailureRef } from '@/lib/hooks/common/usePreAttachImageFailureRef'

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

  // One of this component's callers puts it in server-rendered HTML: the
  // CollectionDetail header, because `/collections/[slug]` prefetches the
  // collection on the server and hydrates it. There the browser is already
  // fetching the cover before React sees the element, so a dead URL 404s with
  // no handler attached and the slot would stay blank instead of showing the
  // caller's fallback. The other callers (browse card, featured card and
  // archive, the add-to-collection dialog) fetch in the browser, where
  // `onError` alone would do; the read is harmless there. See the hook for
  // the mechanism and its caveats.
  const preAttachFailureRef = usePreAttachImageFailureRef(markFailed)

  return (
    <div className={cn('overflow-hidden', className)}>
      {showImage ? (
        /* eslint-disable-next-line @next/next/no-img-element */
        <img
          // Deliberately NOT keyed on the URL. The ref is identity-stable, so
          // a URL change does not re-run it against a reused node whose state
          // still describes the previous image, and a failure on the new URL
          // is caught by `onError`, which React attaches at element creation.
          // Keying would only cost the browser's seamless swap: it keeps
          // painting the current cover until the replacement has fully loaded
          // rather than blanking the tile for a third-party fetch.
          ref={preAttachFailureRef}
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
