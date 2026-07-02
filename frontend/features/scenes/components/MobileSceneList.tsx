'use client'

import { useMemo, useState } from 'react'
import { ScenePreviewContent } from './ScenePreviewContent'
import { compareScenesByActivity } from './globeScale'
import type { SceneListItem } from '../types'

/**
 * <640px: the WebGL globe + canvas gestures aren't usable (PSY-511/1086 gate),
 * so serve the scenes as a list — still the geographic-discovery payoff, just
 * not spatial. Lists ALL scenes (incl. ones the globe can't place), liveliest
 * first. Each row expands in place (accordion — the app's in-list expansion
 * idiom, e.g. StationShowsDirectory; the Radix Sheet is modal and overkill
 * here) into the same payoff the desktop preview panel shows: playable embed,
 * this-week shows, top local artists, scene link (PSY-1311).
 */
export function MobileSceneList({
  scenes,
  loading,
}: {
  scenes: SceneListItem[]
  loading: boolean
}) {
  // One row open at a time: the payoff includes an audio embed, and stacking
  // several players in one scroll column is noise, not discovery.
  const [expandedSlug, setExpandedSlug] = useState<string | null>(null)

  // Liveliest first — the API returns its own order, but the top of a mobile
  // list is prime space and should be the most active scenes. Shared comparator
  // so this can't drift from AtlasSearch / globe label ordering.
  const sorted = useMemo(
    () => [...scenes].sort(compareScenesByActivity),
    [scenes],
  )

  return (
    <div className="h-full w-full overflow-y-auto bg-background p-4">
      <h1 className="text-lg font-semibold">Scenes</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        The globe is best on a larger screen. Browse the scenes below.
      </p>
      {loading ? (
        <p className="mt-4 text-sm text-muted-foreground">Loading…</p>
      ) : (
        <ul className="mt-4 flex flex-col divide-y divide-border">
          {sorted.map((s) => {
            const expanded = expandedSlug === s.slug
            const detailId = `scene-row-${s.slug}`
            return (
              <li key={s.slug}>
                {/* aria-controls only while the detail region exists — a
                    dangling id reference fails aria-valid-attr-value. */}
                <button
                  type="button"
                  aria-expanded={expanded}
                  aria-controls={expanded ? detailId : undefined}
                  onClick={(e) => {
                    const row = e.currentTarget
                    setExpandedSlug(expanded ? null : s.slug)
                    // Switching from a taller expanded row ABOVE this one
                    // shrinks the layout under an unchanged scrollTop, yanking
                    // the tapped row (and its fresh preview) out of view — pin
                    // it after the accordion re-lays out. `?.` guards jsdom.
                    if (!expanded) {
                      requestAnimationFrame(() =>
                        row.scrollIntoView?.({ block: 'nearest' }),
                      )
                    }
                  }}
                  className="flex w-full items-center justify-between gap-3 py-3 text-left"
                >
                  <span className="font-medium">
                    {s.city}, {s.state}
                  </span>
                  <span className="flex items-center gap-2 font-mono text-xs text-muted-foreground">
                    {s.upcoming_show_count} upcoming
                    <span aria-hidden>{expanded ? '−' : '+'}</span>
                  </span>
                </button>
                {/* Mounted only while expanded, so the roster/shows queries
                    fire per opened scene, never for the whole list. */}
                {expanded && (
                  <div id={detailId} className="pb-4">
                    <ScenePreviewContent scene={s} />
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
