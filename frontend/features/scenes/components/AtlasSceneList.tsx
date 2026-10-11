'use client'

import { useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { ScenePreviewContent } from './ScenePreviewContent'
import { compareScenesByActivity } from './globeScale'
import type { AtlasSceneListReason } from '../atlasViewport'
import type { SceneListItem } from '../types'

/** The line under the heading that tells the visitor why there is no map. */
const REASON_LINES: Record<AtlasSceneListReason, string> = {
  'no-webgl2': 'This browser can’t draw the map, so here are the scenes as a list.',
  'reduced-motion':
    'You’ve asked for less motion, so the map stays off and the scenes are listed.',
  'map-failed': 'The map didn’t start. Here are the scenes as a list.',
}

/**
 * The Atlas's stand-in for the map wherever atlasSceneListReason gives a
 * reason the map should not run: the scenes as a list, still the
 * geographic-discovery payoff, just not spatial. The line under the heading
 * states the reason. Lists ALL scenes (incl. ones the globe can't
 * place), liveliest first. Each row expands in place (the app expands in
 * place rather than modally, cf. StationShowsDirectory's in-place view-all;
 * the only Sheet primitive is modal and would bury the list) into the same
 * payoff the map's scene preview shows: playable embed, next-7-days shows,
 * top local artists, scene link.
 */
export function AtlasSceneList({
  scenes,
  loading,
  reason,
  onRetryMap,
  followedSlugs = null,
}: {
  scenes: SceneListItem[]
  loading: boolean
  /** Why the map is not running. */
  reason: AtlasSceneListReason
  /**
   * Asks for the map again, offered as a button under the line whenever it
   * is given. The caller passes it only with the `'map-failed'` reason, where
   * a retry can succeed.
   */
  onRetryMap?: () => void
  /** Slugs of scenes the viewer follows (PSY-1340) — starred in the list. */
  followedSlugs?: ReadonlySet<string> | null
}) {
  // One row open at a time: the payoff includes an audio embed, and stacking
  // several players in one scroll column is noise, not discovery.
  const [expandedSlug, setExpandedSlug] = useState<string | null>(null)

  // Liveliest first: the API returns its own order, but the top of the list
  // is prime space and should be the most active scenes. Shared comparator
  // so this can't drift from AtlasSearch / globe label ordering.
  const sorted = useMemo(
    () => [...scenes].sort(compareScenesByActivity),
    [scenes],
  )

  return (
    <div
      data-testid="atlas-scene-list"
      className="h-full w-full overflow-y-auto bg-background p-4"
    >
      <h1 className="text-lg font-semibold">Scenes</h1>
      <p className="mt-1.5 text-sm text-muted-foreground">{REASON_LINES[reason]}</p>
      {onRetryMap && (
        <Button
          variant="outline"
          onClick={onRetryMap}
          className="mt-3 h-11 w-full rounded-lg border-foreground font-medium dark:border-foreground"
        >
          Try the map again
        </Button>
      )}
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
                    {/* Both headline counts, matching the map preview's
                        "N upcoming · M venues" line: this row is the only
                        place the list shows them. */}
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
