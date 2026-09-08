'use client'

import Link from 'next/link'
import { cn } from '@/lib/utils'
// Deep-imported rather than through `@/features/tags`, the same rule
// SceneCollections follows: that barrel is `'use client'` and carries the whole
// tag surface, so naming it here would pull it into this route's chunk.
import {
  getCategoryChipClasses,
  TAG_CATEGORY_CREW,
} from '@/features/tags/types'
import { useSceneCrews } from '../hooks'
import { entityHref } from './sceneChrome'
import type { SceneCrewSummary, SceneDetail } from '../types'

/**
 * The chip's own geometry and density. Everything ABOUT the crew category
 * (unfilled, hairline, square, mono uppercase, muted) comes from
 * `getCategoryChipClasses`, which is composed after this so its shape wins.
 *
 * The size is this surface's, not the category's: the shared treatment carries
 * no font size precisely so each surface keeps its own, and this page's
 * micro-caps register is 11px.
 */
const CREW_CHIP_CLASS =
  'inline-flex items-center border px-2 py-1 text-[11px] leading-none'

/** Hover and focus, which the chip only wears when it is a link. */
const CREW_CHIP_LINK_CLASS =
  'transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring'

/**
 * One crew, linked to its tag page when it has a usable slug.
 *
 * An unlinkable crew is still NAMED. It is one of the crews booking in this
 * scene, and dropping it would misstate the row; the empty and dot-segment
 * slugs `entityHref` answers null for would otherwise land the reader on the
 * tag index rather than on the crew.
 *
 * The name is uppercased by CSS, so the accessible name and the link's text
 * content stay the stored casing.
 */
function CrewChip({ crew }: { crew: SceneCrewSummary }) {
  const chipClass = cn(
    CREW_CHIP_CLASS,
    getCategoryChipClasses(TAG_CATEGORY_CREW)
  )
  const href = entityHref('/tags', crew.slug)

  if (!href) return <span className={chipClass}>{crew.name}</span>

  return (
    <Link href={href} className={cn(chipClass, CREW_CHIP_LINK_CLASS)}>
      {crew.name}
    </Link>
  )
}

/**
 * The music bookers whose tag sits on shows in this scene: promoters, DIY
 * crews, named series.
 *
 * The row HIDES COMPLETELY when the endpoint returns nothing, and hides while
 * loading and on error. Most scenes carry no crew tag, and a heading over
 * empty space would report a gap in the catalog as a fact about the town.
 *
 * ORDER IS THE ENDPOINT'S — most of the scene's shows first, then name. The
 * ranking counts are not drawn: the reader is being offered a way into the
 * crew, not a leaderboard.
 *
 * UNCAPPED, and the row wraps rather than truncating. Nothing here decides
 * which crews of a town are worth naming.
 */
export function SceneCrews({ scene }: { scene: SceneDetail }) {
  const { data } = useSceneCrews(scene.slug)

  const crews = data?.crews ?? []
  if (crews.length === 0) return null

  return (
    <ul
      aria-label={`Crews booking in ${scene.city}`}
      className="mt-3 flex flex-wrap gap-1.5"
    >
      {crews.map(crew => (
        <li key={crew.slug || crew.name}>
          <CrewChip crew={crew} />
        </li>
      ))}
    </ul>
  )
}
