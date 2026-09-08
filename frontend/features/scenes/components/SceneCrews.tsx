'use client'

import { cn } from '@/lib/utils'
// Deep-imported rather than through `@/features/tags`, the same rule
// SceneCollections follows: that barrel is `'use client'` and carries the whole
// tag surface, so naming it here would pull it into this route's chunk.
import {
  getCategoryChipClasses,
  TAG_CATEGORY_CREW,
} from '@/features/tags/types'
import { useSceneCrews } from '../hooks'
import { EntityNameLink } from './sceneChrome'
import type { SceneDetail } from '../types'

/**
 * The chip, composed once. Everything ABOUT the crew category (unfilled,
 * hairline, square, mono uppercase, muted) comes from the shared treatment;
 * the size is this surface's, because that treatment carries no font size
 * precisely so each surface keeps its own, and this page's micro-caps register
 * is 11px.
 */
const CREW_CHIP_CLASS = cn(
  'inline-flex items-center border px-2 py-1 text-[11px] leading-none',
  getCategoryChipClasses(TAG_CATEGORY_CREW)
)

/** Hover and focus, which the chip wears only when it is a link. */
const CREW_CHIP_LINK_CLASS = cn(
  CREW_CHIP_CLASS,
  'transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring'
)

/**
 * The music bookers whose tag sits on shows in this scene: promoters, DIY
 * crews, named series.
 *
 * A crew is a TAG, not an entity, so a chip lands on `/tags/{slug}` rather
 * than on a page of its own, and a crew whose slug cannot address one is
 * still named rather than dropped.
 *
 * The row is absent, with no heading and no scaffold, when the endpoint
 * returns nothing and while the request is in flight or failed. Most scenes
 * carry no crew tag, and a heading over empty space would report a gap in the
 * catalog as a fact about the town.
 *
 * The rows are drawn in the order they arrive, uncapped, wrapping. The
 * endpoint ranks them by how many of the scene's shows carry each tag; the
 * counts themselves are not drawn, because the reader is being offered a way
 * into the crew rather than a leaderboard.
 */
export function SceneCrews({ scene }: { scene: SceneDetail }) {
  const crews = useSceneCrews(scene.slug).data?.crews
  if (!crews?.length) return null

  return (
    <ul
      aria-label={`Crews booking in ${scene.city}`}
      className="mt-3 flex flex-wrap gap-1.5"
    >
      {crews.map(crew => (
        <li key={crew.slug || crew.name}>
          <EntityNameLink
            name={crew.name}
            slug={crew.slug}
            basePath="/tags"
            className={CREW_CHIP_LINK_CLASS}
            unlinkedClassName={CREW_CHIP_CLASS}
          />
        </li>
      ))}
    </ul>
  )
}
