'use client'

import { cn } from '@/lib/utils'
// Deep-imported, not through a barrel — see the note in
// features/scenes/components/index.ts.
import {
  getCategoryChipClasses,
  TAG_CATEGORY_CREW,
} from '@/features/tags/types'
import { useSceneCrews } from '../hooks'
import { EntityNameLink } from './sceneChrome'
import type { SceneDetail } from '../types'

/**
 * The chip, composed once.
 *
 * The crew category's own look (no fill, square corners, mono uppercase
 * letterspaced, muted) comes from the shared treatment. This surface adds the
 * border width, the 8px/4px padding and the 11px size: the shared treatment
 * carries no font size precisely so each surface keeps its own density, and
 * this page's micro-caps register is 11px.
 *
 * `break-words` is the guard on a crew name long enough to exceed the content
 * column on a narrow screen. `tags.name` allows 100 characters, and a single
 * unbroken token that cannot fit a line would otherwise widen the page.
 */
const CREW_CHIP_CLASS = cn(
  'inline-flex max-w-full items-center break-words border px-2 py-1 text-[11px] leading-none',
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
 * catalog as a fact about the town. Figma `1402:788` draws the row as chips
 * alone, with no heading over them even where there is data, which is why the
 * label below is the only naming it gets.
 *
 * `role="list"` is explicit on this element because the label depends on it:
 * a list whose markers are off loses its list semantics in WebKit, and a
 * generic element drops the `aria-label` with them.
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
      role="list"
      aria-label={`Crews booking in ${scene.city}, ${scene.state}`}
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
