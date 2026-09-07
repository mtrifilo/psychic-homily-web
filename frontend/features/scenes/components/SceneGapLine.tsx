'use client'

import Link from 'next/link'
import { useSceneGaps } from '../hooks'
import { gapLineCopy } from '../sceneGapLine'
import { sceneCityListHref } from '../sceneWindow'
import { SCENE_ACCENT_LINK_CLASS } from './sceneChrome'
import type { SceneDetail } from '../types'

/**
 * The scene's one-line contribution hook: the gap stated as content, linking to
 * the bands it is about.
 *
 * HIDES at zero, and hides while loading and on error. A scene whose bands are
 * all reachable has nothing to ask for, and the two temporary states must not
 * flash a request for help the payload may not warrant.
 *
 * The destination is the city-filtered artist list, the widest surface where a
 * non-admin can act: link editing is trusted-tier, and every band this line
 * counts is somewhere in that list. It is BROADER than the number in two ways,
 * both inherent to `/artists` having no missing-link filter: the list is every
 * band in the city rather than only the ones with a gap, and the roster this
 * count is taken over is metro-scoped, so a Mesa band counts under Phoenix
 * while `?cities=Phoenix,AZ` does not show it.
 *
 * Accent-toned rather than muted, which is the locked mock's one departure from
 * the page's micro-caps register: this is the only line on the page asking the
 * reader for something.
 */
export function SceneGapLine({ scene }: { scene: SceneDetail }) {
  const { data } = useSceneGaps(scene.slug)

  const count = data?.artists_missing_listen_link ?? 0
  if (count <= 0) return null

  return (
    <section className="border-t border-border pt-4">
      <Link
        href={sceneCityListHref('/artists', scene.city, scene.state)}
        className={SCENE_ACCENT_LINK_CLASS}
      >
        {gapLineCopy(count, scene.city)}
      </Link>
    </section>
  )
}
