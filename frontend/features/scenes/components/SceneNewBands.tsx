'use client'

import { useSceneNewArtists } from '../hooks'
import { formatFirstListed, formatNewBandShow } from '../sceneNewBands'
import { EntityNameLink, SceneSectionHeading } from './sceneChrome'
import type { SceneDetail, SceneNewArtistRow } from '../types'

/**
 * Named new bands — Scene Pulse's replacement, not its restyling.
 *
 * The pulse's `new_artists_30d` was a bare integer with no names, answering a
 * question participants do ask. This is names instead of a number, and nothing
 * else: no tile, no sparkline, no trend arrow, and no `0`.
 *
 * The module HIDES COMPLETELY when there is nothing to name. A scene with no
 * bands based in it is a normal scene, not a scene with a zero to report.
 *
 * The rows are the roster's most recently listed bands — PSY-1844 removed the
 * endpoint's trailing window, so nothing here may describe the list as covering
 * a period. The section's wording is PSY-1851's to write; this component only
 * stopped saying what had become untrue.
 */

function NewBandRow({ band }: { band: SceneNewArtistRow }) {
  const firstListed = formatFirstListed(band.first_listed_at)
  const clauses = [
    firstListed && `first listed ${firstListed}`,
    formatNewBandShow(band.show),
  ].filter(Boolean)

  return (
    <li className="flex flex-wrap items-baseline gap-x-4 gap-y-0.5 border-b border-border/40 py-2 last:border-b-0">
      <EntityNameLink name={band.name} slug={band.slug} basePath="/artists" />
      <span className="font-mono text-xs text-muted-foreground">
        {clauses.join(' · ')}
      </span>
    </li>
  )
}

export function SceneNewBands({ scene }: { scene: SceneDetail }) {
  // No `limit`: the endpoint's own default owns the cap, the rule this hook
  // documents and `useSceneShows` already follows. Passing one here would be a
  // third copy of a number the backend already decides.
  const { data } = useSceneNewArtists({ slug: scene.slug })

  const bands = data?.artists ?? []
  // One `return null` covers loading, error and a scene with no roster, and
  // that is deliberate rather than lazy: all three mean "we have nothing to
  // name here", and the two that are temporary must not flash a heading over
  // empty space on the way past.
  if (bands.length === 0) return null

  return (
    <section className="border-t border-border pt-4">
      {/* No period qualifier. This read "last 30 days" until PSY-1844 removed
          the window; the payload can no longer support any such claim. */}
      <SceneSectionHeading title={`New / first listed in ${scene.city}`} />

      <ul className="mt-2">
        {bands.map(band => (
          <NewBandRow key={band.id} band={band} />
        ))}
      </ul>
    </section>
  )
}
