'use client'

import { useSceneNewArtists } from '../hooks'
import { formatFirstListed, formatNewBandShow } from '../sceneNewBands'
import { EntityNameLink, SceneSectionHeading } from './sceneChrome'
import type { SceneDetail, SceneNewArtistRow } from '../types'

/**
 * Named new bands — Scene Pulse's replacement, not its restyling.
 *
 * The pulse's `new_artists_30d` was the one number on it that mapped to a
 * question a participant actually asks, and it was a bare integer with no
 * names. This is that number with its names attached, and nothing else: no
 * tile, no sparkline, no trend arrow, and no `0`.
 *
 * The module HIDES COMPLETELY when the window holds nothing. A scene with no
 * new bands is a normal scene, not a scene with a zero to report, and the
 * weekly digest — which computes exactly this list — has never printed one.
 */

/** How many bands the section names before deferring to `+N more`. */
const NEW_BANDS_LIMIT = 5

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
  const { data } = useSceneNewArtists({ slug: scene.slug, limit: NEW_BANDS_LIMIT })

  const bands = data?.artists ?? []
  // One `return null` covers loading, error and a genuinely empty window, and
  // that is deliberate rather than lazy: all three mean "we have nothing to
  // name here", and the two that are temporary must not flash a heading over
  // empty space on the way past.
  if (bands.length === 0) return null

  const total = data?.total ?? bands.length
  const withheld = Math.max(total - bands.length, 0)

  return (
    <section className="border-t border-border pt-4">
      <SceneSectionHeading
        title={`New / first listed in ${scene.city}`}
        note="last 30 days"
      />

      <ul className="mt-2">
        {bands.map(band => (
          <NewBandRow key={band.id} band={band} />
        ))}
      </ul>

      {/* `+N more` and no link, deliberately. The mock draws
          `[All new bands in {city} →]` beside it, and no route serves that
          list — the endpoint is a window, not a page. A control pointing at a
          URL this site 404s is a worse answer than no control, the same call
          Wave 1A made about the mock's dated-archive chip. */}
      {withheld > 0 && (
        <p className="mt-2 font-mono text-[11px] text-muted-foreground">
          +{withheld} more
        </p>
      )}
    </section>
  )
}
