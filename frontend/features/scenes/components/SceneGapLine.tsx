'use client'

import Link from 'next/link'
import { buildCitiesParam } from '@/components/filters/cityParams'
import { useSceneGaps } from '../hooks'
import type { SceneDetail } from '../types'

/**
 * `/artists?cities=Phoenix%2CAZ`, or null when the place cannot be named.
 *
 * `?cities=` is the ONLY city filter `/artists` reads, and its wire format is
 * `City,ST` — so the pair is serialized through `buildCitiesParam`, the single
 * home of that format, rather than respelled here.
 *
 * Null on a blank half, because `parseCitiesParam` drops a segment with one,
 * and a dropped segment leaves `/artists` UNFILTERED. A link that silently
 * lands on every band in the catalog would contradict the sentence that
 * carried the reader to it, which is worse than a line that does not link.
 */
export function sceneArtistsHref(city: string, state: string): string | null {
  const trimmedCity = city.trim()
  const trimmedState = state.trim()
  if (!trimmedCity || !trimmedState) return null

  const params = new URLSearchParams({
    cities: buildCitiesParam([{ city: trimmedCity, state: trimmedState }]),
  })
  return `/artists?${params.toString()}`
}

/**
 * `11 PHOENIX BANDS HAVE NO LISTEN LINK → HELP FINISH PHOENIX`.
 *
 * "LISTEN LINK", not the mock's "BANDCAMP LINK". The count behind this line is
 * bands with no music-platform link AT ALL: spotify, bandcamp, youtube and
 * soundcloud every one of them blank. A band with a Spotify page and no
 * Bandcamp is not in the number, so naming one platform would print a claim
 * about a set the number does not describe.
 */
export function gapLineCopy(count: number, city: string): string {
  const subject =
    count === 1 ? `1 ${city} band has` : `${count} ${city} bands have`
  return `${subject} no listen link → Help finish ${city}`
}

/**
 * The scene's one-line contribution hook: the gap stated as content, with a
 * link to the bands it is about.
 *
 * HIDES at zero, and hides while loading and on error. A scene whose bands are
 * all reachable has nothing to ask for, and the two temporary states must not
 * flash a request for help that may turn out to be unwarranted.
 *
 * The destination is the city-filtered artist list, which is the widest surface
 * where a non-admin can act: link editing is trusted-tier, and every band this
 * line counts is somewhere in that list. It is not yet the list of exactly
 * those bands, because `/artists` carries no missing-link filter.
 *
 * Two ways the destination is broader than the number, both inherent to that:
 * the roster this count is taken over is METRO-scoped, so a Mesa band counts
 * under Phoenix while `?cities=Phoenix,AZ` does not show it; and the list is
 * every band in the city rather than only the ones with a gap.
 *
 * Accent-toned rather than muted, which is the mock's one departure from the
 * page's mono micro-caps register: this is the only line on the page asking the
 * reader for something.
 */
export function SceneGapLine({ scene }: { scene: SceneDetail }) {
  const { data } = useSceneGaps(scene.slug)

  const count = data?.artists_missing_listen_link ?? 0
  if (count <= 0) return null

  const copy = gapLineCopy(count, scene.city)
  const href = sceneArtistsHref(scene.city, scene.state)
  const className =
    'font-mono text-[11px] uppercase tracking-widest text-primary'

  return (
    <section className="border-t border-border pt-4">
      {href ? (
        <Link href={href} className={`${className} hover:underline`}>
          {copy}
        </Link>
      ) : (
        <p className={className}>{copy}</p>
      )}
    </section>
  )
}
