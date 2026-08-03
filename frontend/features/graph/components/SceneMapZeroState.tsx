'use client'

/**
 * The /graph zero-state: the Map of the Scene (PSY-1725).
 *
 * Everything below the card's search row when no artist has been chosen —
 * the map canvas, the isolate band, the freshness footer, the list view that
 * carries the map for anyone not using the canvas, and the label-hub panel.
 *
 * The map is what the page opens ON, not a state it can fail into: whenever
 * there is no readable snapshot the host falls back to the shipped search-first
 * hero instead (see `GraphObservatory`), so this component is only ever handed
 * a map it can draw.
 */

import { useCallback, useMemo, useState } from 'react'
import Link from 'next/link'
import { ArrowRight } from 'lucide-react'

import { EntityContextPanel } from '@/components/graph/EntityContextPanel'
import { GraphSectionErrorBoundary } from '@/components/graph/GraphSectionErrorBoundary'
import { GRAPH_BOX_HEIGHT_CLASS } from '@/components/graph/GraphStateCard'

import type { SceneMap, SceneMapNode } from '../sceneMap'
import { SceneMapCanvas } from './SceneMapCanvas'

/** Anchor shape the host re-roots on — the same one artist search produces. */
export interface SceneMapArtistAnchor {
  id: number
  slug: string
  name: string
}

/** DOM id of the list disclosure — the mobile pitch line's link target. */
export const SCENE_MAP_LIST_ANCHOR = 'scene-map-list'

/** DOM id of the canvas's sr-only description. */
const SCENE_MAP_GUIDANCE_ID = 'scene-map-guidance'

export interface SceneMapZeroStateProps {
  map: SceneMap
  /** Measured width of the card body; null until the first measure lands. */
  containerWidth: number | null
  /** Below this the canvas is not mounted at all (shared graph breakpoint). */
  canvasBreakpointPx: number
  /** Re-root the Observatory on an artist — identical to picking one in search. */
  onSelectArtist: (anchor: SceneMapArtistAnchor) => void
}

export function SceneMapZeroState({
  map,
  containerWidth,
  canvasBreakpointPx,
  onSelectArtist,
}: SceneMapZeroStateProps) {
  const [selectedHub, setSelectedHub] = useState<SceneMapNode | null>(null)
  // Narrowed to a number, not a boolean, so the canvas's width prop needs no
  // non-null assertion at the mount site.
  const canvasWidth =
    containerWidth !== null && containerWidth >= canvasBreakpointPx ? containerWidth : null

  const handleSelectArtist = useCallback(
    (node: SceneMapNode) => {
      onSelectArtist({ id: node.id, slug: node.slug, name: node.name })
    },
    [onSelectArtist],
  )

  // Second click on the open hub puts it away — the same toggle grammar the
  // canvas selection uses everywhere else on this page.
  const handleSelectHub = useCallback((node: SceneMapNode) => {
    setSelectedHub(current => (current?.id === node.id ? null : node))
  }, [])

  const closeHub = useCallback(() => setSelectedHub(null), [])

  return (
    <div className="relative">
      {canvasWidth !== null ? (
        <GraphSectionErrorBoundary
          sentryTag="graph-scene-map"
          fallback={
            <div
              role="status"
              className={`flex items-center justify-center text-sm text-muted-foreground ${GRAPH_BOX_HEIGHT_CLASS}`}
            >
              The map is unavailable. Browse the scene as a list below.
            </div>
          }
        >
          <SceneMapCanvas
            map={map}
            containerWidth={canvasWidth}
            onSelectArtist={handleSelectArtist}
            onSelectHub={handleSelectHub}
            selectedHubId={selectedHub?.id ?? null}
            onBackgroundClick={closeHub}
            ariaLabel={`A map of ${map.artistCount} connected artists in ${map.regions.length} regions.`}
            describedById={SCENE_MAP_GUIDANCE_ID}
          />
          <p id={SCENE_MAP_GUIDANCE_ID} className="sr-only">
            Select an artist to center the graph on them. Select a label to see
            its details. Browse the map as a list below.
          </p>
        </GraphSectionErrorBoundary>
      ) : (
        <MapPitchLine map={map} />
      )}

      {selectedHub && (
        <EntityContextPanel
          className={canvasWidth !== null ? 'absolute right-5 top-5 z-40' : 'mt-3'}
          entityType="label"
          name={selectedHub.name}
          slug={selectedHub.slug}
          primary={{
            kind: 'emphasis',
            text: `${selectedHub.degree} ${selectedHub.degree === 1 ? 'artist' : 'artists'} on the map`,
          }}
          onClose={closeHub}
        />
      )}

      <IsolateBand count={map.isolateCount} />
      <SceneMapList map={map} onSelectArtist={handleSelectArtist} />
      <FreshnessFooter map={map} />
    </div>
  )
}

/**
 * The sub-640px branch (PSY-1472 idiom): no map thumbnail — a canvas that small
 * fails tap targets and shows a smear rather than a scene — but the same live
 * counts, and a way into the list that carries them.
 */
function MapPitchLine({ map }: { map: SceneMap }) {
  return (
    <p className="px-2 py-4 text-center text-sm text-muted-foreground">
      {map.artistCount.toLocaleString()} artists are already connected across{' '}
      {map.regions.length.toLocaleString()} regions of the scene.{' '}
      <a
        href={`#${SCENE_MAP_LIST_ANCHOR}`}
        className="text-primary underline-offset-4 hover:underline"
      >
        Browse the map as a list
      </a>
      .
    </p>
  )
}

/**
 * Artists the backbone left with no edge at all. Reported as a number rather
 * than drawn (a few thousand unconnected dots would say only "nothing is known
 * about these yet"), with the one action that changes it.
 */
function IsolateBand({ count }: { count: number }) {
  if (count <= 0) return null
  return (
    <p className="border-t border-border/50 px-4 py-2.5 text-xs text-muted-foreground">
      +{count.toLocaleString()} not yet connected{' '}
      {count === 1 ? 'artist' : 'artists'}.{' '}
      <Link
        href="/contribute"
        className="inline-flex items-center gap-1 text-primary underline-offset-4 hover:underline"
      >
        Add a show or a label to put one on the map
        <ArrowRight className="size-3" aria-hidden="true" />
      </Link>
    </p>
  )
}

/** When the map was built, and what is on it. */
function FreshnessFooter({ map }: { map: SceneMap }) {
  const lastMapped = Number.isNaN(map.lastMapped.getTime())
    ? null
    : map.lastMapped.toLocaleDateString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      })
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-t border-border/50 px-4 py-2.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
      <span>Mapped nightly{lastMapped ? ` · Last mapped ${lastMapped}` : ''}</span>
      <span>
        {map.artistCount.toLocaleString()} connected ·{' '}
        {map.labelCount.toLocaleString()} labels ·{' '}
        {map.regions.length.toLocaleString()} regions
      </span>
    </div>
  )
}

/**
 * The map as a browsable list — the non-canvas half of the surface.
 *
 * Deliberately this page's own shipped `AccessibleGraphList` idiom (a
 * `<details>` disclosure over grouped buttons) rather than the `role="tree"`
 * primitive: that tree models EXPAND-ON-DEMAND ego traversal and stamps every
 * row `aria-expanded`, which on a static region listing would advertise an
 * expansion that does not exist. Regions here are the map's own grouping, and
 * an artist row does exactly what its dot does — re-root the Observatory.
 *
 * Regions render collapsed, so the DOM cost is one row per region until a
 * visitor asks for members. No per-region cap: a capped list would quietly be
 * LESS than the canvas offers, which is the one thing this list must not be.
 */
function SceneMapList({
  map,
  onSelectArtist,
}: {
  map: SceneMap
  onSelectArtist: (node: SceneMapNode) => void
}) {
  const groups = useMemo(() => groupNodesByRegion(map), [map])

  return (
    <details
      id={SCENE_MAP_LIST_ANCHOR}
      className="border-t border-border/50 px-4 py-3"
    >
      <summary className="cursor-pointer text-sm font-medium">
        Browse the map as a list
      </summary>
      <p className="mt-2 text-sm text-muted-foreground">
        Choose an artist to center the graph there and start a trail.
      </p>
      <ul className="mt-3 space-y-3">
        {groups.map(group => (
          <li key={group.key}>
            <details className="rounded-lg border border-border/50 bg-muted/10 px-3 py-2">
              <summary className="cursor-pointer text-sm">
                <span className="font-medium">{group.label}</span>{' '}
                <span className="text-xs text-muted-foreground">
                  {group.nodes.length.toLocaleString()}{' '}
                  {group.nodes.length === 1 ? 'artist' : 'artists'}
                </span>
              </summary>
              <ul className="mt-2 divide-y divide-border/50">
                {group.nodes.map(node => (
                  <li key={node.id}>
                    <button
                      type="button"
                      onClick={() => onSelectArtist(node)}
                      className="flex w-full items-center justify-between gap-3 py-2 text-left text-sm hover:text-primary"
                    >
                      <span>{node.name}</span>
                      {node.hasUpcomingShow && (
                        <span className="shrink-0 text-xs text-muted-foreground">
                          Upcoming show
                        </span>
                      )}
                    </button>
                  </li>
                ))}
              </ul>
            </details>
          </li>
        ))}
      </ul>
    </details>
  )
}

interface SceneMapListGroup {
  key: string
  label: string
  nodes: SceneMapNode[]
}

/**
 * Group the map's artists under the regions the canvas names, biggest region
 * first and members by centrality — the same ordering the map expresses
 * visually through hull size and label tier.
 *
 * Label hubs carry no community by design (a hub anchors a roster, it does not
 * live in one), so they are NOT listed: a hub's dot opens a context card rather
 * than re-rooting, and every artist a hub connects is already listed under its
 * own region. Artists whose community has no region entry fall into a trailing
 * group so the list total can never come up short of the map.
 */
export function groupNodesByRegion(map: SceneMap): SceneMapListGroup[] {
  const byCommunity = new Map<number, SceneMapNode[]>()
  for (const node of map.nodes) {
    if (node.kind !== 'artist') continue
    const bucket = byCommunity.get(node.community)
    if (bucket) bucket.push(node)
    else byCommunity.set(node.community, [node])
  }
  for (const bucket of byCommunity.values()) {
    bucket.sort((a, b) => a.rank - b.rank || a.name.localeCompare(b.name))
  }

  const groups: SceneMapListGroup[] = []
  const named = new Set<number>()
  for (const region of map.regions) {
    const nodes = byCommunity.get(region.community)
    if (!nodes || nodes.length === 0) continue
    named.add(region.community)
    groups.push({ key: `region-${region.community}`, label: region.label, nodes })
  }
  groups.sort((a, b) => b.nodes.length - a.nodes.length || a.label.localeCompare(b.label))

  const ungrouped: SceneMapNode[] = []
  for (const [community, nodes] of byCommunity) {
    if (named.has(community)) continue
    ungrouped.push(...nodes)
  }
  if (ungrouped.length > 0) {
    ungrouped.sort((a, b) => a.rank - b.rank || a.name.localeCompare(b.name))
    groups.push({ key: 'ungrouped', label: 'Elsewhere on the map', nodes: ungrouped })
  }
  return groups
}
