'use client'

import { useSceneNewArtists } from '../hooks'
import { formatFirstListed, formatNewBandShow } from '../sceneNewBands'
import { EntityNameLink, SceneSectionHeading } from './sceneChrome'
import type { SceneDetail, SceneNewArtistRow } from '../types'

/**
 * The roster's most recently listed bands, named and linked.
 *
 * Names, never a number: no tile, no sparkline, no trend arrow, and no `0`. The
 * module HIDES COMPLETELY when there is nothing to name, because a scene with
 * no bands based in it is a normal scene, not a scene with a zero to report.
 *
 * The heading carries NO period claim. `GET /scenes/{slug}/new-artists` is
 * unwindowed: it returns the roster ordered by `first_listed_at` DESC, so any
 * trailing-period wording ("this month") is false on a scene whose newest band
 * was listed months ago. Per-row `first_listed_at` IS an exact fact, and the
 * rows print it as one.
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
      {/* Unqualified by period, and unqualified by place: the page's H1 names
          the city, and the payload backs no window claim. */}
      <SceneSectionHeading title="Latest additions" />

      <ul className="mt-2">
        {bands.map(band => (
          <NewBandRow key={band.id} band={band} />
        ))}
      </ul>
    </section>
  )
}
