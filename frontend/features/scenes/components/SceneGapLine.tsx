'use client'

import Link from 'next/link'
import { useSceneGaps } from '../hooks'
import { gapLineCopy } from '../sceneGapLine'
import { sceneCityListHref } from '../sceneWindow'
import { SCENE_ACCENT_LINK_CLASS } from './sceneChrome'
import type { SceneDetail } from '../types'

/**
 * The scene's one-line contribution hook: the gap stated as content, linking to
 * the artist list for the scene's city.
 *
 * HIDES at zero, and hides while loading and on error. A scene whose bands are
 * all reachable has nothing to ask for, and the two temporary states must not
 * flash a request for help the payload may not warrant.
 *
 * THE COUNT AND THE DESTINATION ARE TWO DIFFERENT POPULATIONS, and the link is
 * an approximate way in rather than the list of the bands the number counts.
 * `/artists` has no missing-link filter, so it shows every band it matches, not
 * only the ones with a gap. It matches on `city = ? AND state = ?` exactly and
 * case-sensitively (services/catalog/artist.go GetArtists), while the count is
 * taken over the scene roster, which matches case-insensitively and by METRO
 * membership. So the destination can also be NARROWER than the number: a Mesa
 * band counts under Phoenix and `?cities=Phoenix,AZ` does not return it, and a
 * scene whose display city is a metro principal name no artist row stores
 * verbatim lands on an empty list. Closing that gap is the follow-up ticket's
 * job, not something this line can spell around.
 *
 * The destination is still the widest surface a reader can act from: an
 * authenticated reader edits an artist's links directly or through suggest-edit
 * from the artist page, and every band with a gap is reachable from some city's
 * list.
 *
 * Accent-toned, which is the locked mock's one departure from the page's
 * micro-caps register: this is the only line on the page asking the reader for
 * something.
 *
 * A bare div, not the siblings' `section`: the mock draws this module without a
 * heading, and a sectioning element with no accessible name names nothing.
 */
export function SceneGapLine({ scene }: { scene: SceneDetail }) {
  const { data } = useSceneGaps(scene.slug)

  const count = data?.artists_missing_listen_link ?? 0
  if (count <= 0) return null

  return (
    <div className="border-t border-border pt-4">
      <Link
        href={sceneCityListHref('/artists', scene.city, scene.state)}
        className={SCENE_ACCENT_LINK_CLASS}
      >
        {gapLineCopy(count, scene.city)}
      </Link>
    </div>
  )
}
