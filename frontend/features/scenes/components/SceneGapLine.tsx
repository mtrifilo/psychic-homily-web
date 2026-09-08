'use client'

import Link from 'next/link'
import { useSceneGaps } from '../hooks'
import { gapLineCopy } from '../sceneGapLine'
import { sceneCityListHref } from '../sceneWindow'
// Deep import, not the '@/features/artists' barrel: this module is rendered by
// a scene page and the barrel pulls the artist feature's component graph with
// it. Same reason SceneGraphVisualization deep-imports its hook.
import {
  ARTIST_MISSING_LISTEN,
  ARTIST_MISSING_PARAM,
} from '@/features/artists/api'
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
 * The link carries `?missing=listen`, which is what makes the destination the
 * bands this number counts rather than every band in the city: under that param
 * the artist list scopes by the scene's roster (metro-aware, case-insensitive)
 * and drops its upcoming-show gate, so its total is this count. Dropping the
 * param on the destination page widens it back to the whole city.
 *
 * A reader acts from there: an authenticated reader edits an artist's links
 * directly or through suggest-edit from the artist page.
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
        href={sceneCityListHref('/artists', scene.city, scene.state, {
          [ARTIST_MISSING_PARAM]: ARTIST_MISSING_LISTEN,
        })}
        className={SCENE_ACCENT_LINK_CLASS}
      >
        {gapLineCopy(count, scene.city)}
      </Link>
    </div>
  )
}
