'use client'

import type { ComponentProps, PointerEvent, TouchEvent } from 'react'
import { useRouter } from 'next/navigation'
import { AtlasMapReadyLink, type AtlasPrefetch } from '@/lib/atlasMapReadyLink'
import { useCoarsePointer } from '@/lib/hooks/common/useCoarsePointer'
import { useHydrated } from '@/lib/hooks/common/useHydrated'

/**
 * How a chrome link prefetches right now:
 * - `'after-map'` or `'never'`: the caller's Atlas policy, on a fine pointer.
 *   Next prefetches the link as usual (viewport, hover, touchstart) except
 *   where AtlasMapReadyLink holds it on the Atlas.
 * - `'on-touch'`: on a coarse pointer, and on any pointer through the
 *   hydration render, on every route. The link never prefetches on viewport
 *   entry; it prefetches its route when a touch, a pen or a mouse button lands
 *   on it, Atlas map or not, because that is the start of a navigation. That
 *   prefetch is `router.prefetch`, which Next queues at its default priority:
 *   it starts only once the in-flight prefetches (a page's own content links
 *   among them) fall below Next's default concurrency limit, a limit Next's
 *   own hover and touch prefetch may exceed. The policy follows the pointer,
 *   not the width: a coarse pointer at xl and wider (no bottom tab bar) takes
 *   it too, without the tab bar's pending state.
 */
export type ChromePrefetch = AtlasPrefetch | 'on-touch'

/**
 * The prefetch policy for a chrome link. Until hydration has run, the pointer
 * is unknown (useCoarsePointer reads false on the server snapshot), so the
 * hydration render takes `'on-touch'`: a link Next observes on that commit could
 * start a viewport prefetch on a phone before the next commit drops it.
 */
export function chromePrefetch(
  hydrated: boolean,
  coarsePointer: boolean,
  atlasPrefetch: AtlasPrefetch
): ChromePrefetch {
  return !hydrated || coarsePointer ? 'on-touch' : atlasPrefetch
}

/**
 * `next/link` for the app chrome; the lint rule on `components/layout` keeps
 * the chrome from importing `next/link`'s Link directly. `atlasPrefetch` is
 * the policy on a fine pointer (see {@link chromePrefetch} for the rest):
 * - `atlasPrefetch="after-map"` for the primary navigation: the bottom tab
 *   bar, PrimaryNav (top-nav mode) and the side rail's links (side-nav mode).
 * - `'never'` (the default) for everything else. Each prefetch fetches that
 *   route's own JS, and on the Atlas the long tail of menu, footer and banner
 *   links would compete with the map's downloads for routes most Atlas
 *   visitors never open.
 *
 * Two opt-outs, for a caller that needs fewer prefetches than that:
 * - `viewportPrefetch={false}`: on a coarse pointer, the `'on-touch'` policy
 *   unchanged (a touch still prefetches the route). On a fine pointer, no
 *   prefetch at all: the link renders Next's `prefetch={false}`, which turns
 *   off its hover and touchstart prefetch along with the viewport one.
 * - `prefetch={false}`: no prefetch on any pointer, on a touch too.
 */
export function ChromeLink({
  atlasPrefetch = 'never',
  viewportPrefetch = true,
  href,
  prefetch,
  onTouchStart,
  onPointerDown,
  ...props
}: Omit<ComponentProps<typeof AtlasMapReadyLink>, 'atlasPrefetch' | 'href'> & {
  href: string
  atlasPrefetch?: AtlasPrefetch
  viewportPrefetch?: boolean
}) {
  const router = useRouter()
  const policy = chromePrefetch(useHydrated(), useCoarsePointer(), atlasPrefetch)
  const onTouch = policy === 'on-touch'
  const prefetchOnTouch = onTouch && prefetch !== false
  // Next's own prefetch is off on touch (the handlers below replace it) and
  // wherever the caller dropped the speculative prefetch.
  const nextPrefetchOff = onTouch || !viewportPrefetch

  return (
    <AtlasMapReadyLink
      {...props}
      href={href}
      // prefetch={false} turns off Next's own viewport, hover and touchstart
      // prefetch.
      prefetch={nextPrefetchOff ? false : prefetch}
      // With prefetch already off, 'never' spares the link the map signal.
      atlasPrefetch={nextPrefetchOff ? 'never' : policy}
      onTouchStart={(event: TouchEvent<HTMLAnchorElement>) => {
        onTouchStart?.(event)
        if (prefetchOnTouch) router.prefetch(href)
      }}
      onPointerDown={(event: PointerEvent<HTMLAnchorElement>) => {
        onPointerDown?.(event)
        // A finger reaches the router through touchstart; this covers a pen,
        // or a mouse or trackpad on a coarse-pointer device, which send none.
        if (prefetchOnTouch && event.pointerType !== 'touch') {
          router.prefetch(href)
        }
      }}
    />
  )
}
