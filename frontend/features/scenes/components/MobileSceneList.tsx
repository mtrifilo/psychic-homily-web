'use client'

import { useMemo, useState } from 'react'
import { ScenePreviewContent } from './ScenePreviewContent'
import { compareScenesByActivity } from './globeScale'
import type { SceneListItem } from '../types'

/**
 * <640px: the WebGL globe + canvas gestures aren't usable (PSY-511/1086 gate),
 * so serve the scenes as a list — still the geographic-discovery payoff, just
 * not spatial. Lists ALL scenes (incl. ones the globe can't place), liveliest
 * first. Each row expands in place — the app expands in place rather than
 * modally (cf. StationShowsDirectory's in-place view-all; the only Sheet
 * primitive is modal and would bury the list) — into the same payoff the
 * desktop preview panel shows: playable embed, this-week shows, top local
 * artists, scene link (PSY-1311).
 */
export function MobileSceneList({
  scenes,
  loading,
  followedSlugs = null,
}: {
  scenes: SceneListItem[]
  loading: boolean
  /** Slugs of scenes the viewer follows (PSY-1340) — starred in the list. */
  followedSlugs?: ReadonlySet<string> | null
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
                {/* h2-wrapped button = the WAI-ARIA APG accordion header
                    pattern: keeps the h1 → h2 → (preview's h3s) heading order
                    and makes each scene name a heading-navigation stop.
                    aria-controls only while the detail region exists — a
                    dangling id reference fails aria-valid-attr-value. */}
                <h2>
                  <button
                    type="button"
                    aria-expanded={expanded}
                    aria-controls={expanded ? detailId : undefined}
                    onClick={(e) => {
                      // The whole <li> (row + freshly mounted preview), not
                      // the button: a tap near the bottom of the viewport
                      // mounts the preview below the fold, and a taller
                      // expanded row ABOVE collapsing shrinks the layout under
                      // an unchanged scrollTop — both leave the payoff
                      // off-screen unless the full item is pinned after the
                      // accordion re-lays out. `?.` guards jsdom.
                      const item = e.currentTarget.closest('li')
                      setExpandedSlug(expanded ? null : s.slug)
                      if (!expanded) {
                        requestAnimationFrame(() =>
                          item?.scrollIntoView?.({ block: 'nearest' }),
                        )
                      }
                    }}
                    className="flex w-full items-center justify-between gap-3 py-3 text-left"
                  >
                    <span className="font-medium">
                      {followedSlugs?.has(s.slug) && (
                        <span
                          aria-label="Followed scene"
                          role="img"
                          className="mr-1.5 text-primary"
                        >
                          ★
                        </span>
                      )}
                      {s.city}, {s.state}
                    </span>
                    {/* Both headline counts, matching the desktop panel's
                        "N upcoming · M venues" line — the ticket's payoff
                        parity includes the counts, and this row is the only
                        place the mobile surface shows them. */}
                    <span className="flex items-center gap-2 font-mono text-xs text-muted-foreground">
                      {s.upcoming_show_count} upcoming · {s.venue_count} venues
                      <span aria-hidden>{expanded ? '−' : '+'}</span>
                    </span>
                  </button>
                </h2>
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
