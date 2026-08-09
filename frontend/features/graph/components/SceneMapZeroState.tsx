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

import { useCallback, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { ArrowRight, Play, Share2 } from 'lucide-react'

import { EntityContextPanel } from '@/components/graph/EntityContextPanel'
import { GraphSectionErrorBoundary } from '@/components/graph/GraphSectionErrorBoundary'
import { GraphStateCard, GRAPH_BOX_HEIGHT_CLASS } from '@/components/graph/GraphStateCard'
import { isolateShelfCaption } from '@/components/graph/isolateShelf'

import { groupNodesByRegion, type SceneMap, type SceneMapNode } from '../sceneMap'
import {
  GRAPH_WEEK_PATH,
  graphWeekSummary,
  isGraphWeekShareworthy,
  resolveGraphWeek,
} from '../graphWeek'
import type { SceneReplayController } from '../useSceneReplay'
import { MAP_CARD_CHIP_CLASS, ReplayScrubber } from './ReplayScrubber'
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
  /**
   * Width to draw the canvas at, or null for no canvas — the host owns that
   * gate (it also decides whether the hero shows), so the threshold rule lives
   * in exactly one place and the two cannot disagree.
   */
  canvasWidth: number | null
  /** Re-root the Observatory on an artist — identical to picking one in search. */
  onSelectArtist: (anchor: SceneMapArtistAnchor) => void
  /**
   * The growth replay's transport (PSY-1737), owned by the host because the
   * page header reads the same clock. Omitted in the tests that only exercise
   * the card, and unusable below the canvas breakpoint either way.
   */
  replay?: SceneReplayController
}

export function SceneMapZeroState({
  map,
  canvasWidth,
  onSelectArtist,
  replay,
}: SceneMapZeroStateProps) {
  const [selectedHub, setSelectedHub] = useState<SceneMapNode | null>(null)

  // NO REPLAY BELOW THE CANVAS BREAKPOINT (locked scope edge). Sub-640px keeps
  // the PSY-1472 teaser idiom: there is no map down there, so there is nothing
  // to watch grow. The rule lives HERE, once, beside the canvas gate it mirrors
  // — the host must not be able to disagree with it.
  const canReplay = canvasWidth !== null && replay?.timeline != null
  const isReplaying = canReplay && replay.isActive
  const replayTimeline = replay?.timeline ?? null

  // Memoised so the canvas gets a STABLE object. Its paint callbacks list this
  // as a dependency, and a fresh object per render would rebuild every one of
  // them on every render — including the one the library treats as its
  // repaint trigger.
  const readFrame = replay?.readFrame
  const canvasReplay = useMemo(
    () => (canReplay && replayTimeline && readFrame ? { timeline: replayTimeline, readFrame } : null),
    [canReplay, replayTimeline, readFrame],
  )

  const exitReplay = replay?.exit
  useEffect(() => {
    if (!isReplaying || !exitReplay) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      // BUBBLE phase, and it defers to anything already handled. The graph's
      // inspector panels dismiss through Radix's layer stack, which
      // preventDefaults in the CAPTURE phase — so an Escape aimed at an open
      // hub card closes the card and leaves the run alone, which is the same
      // deference the fullscreen overlay keeps (PSY-1355).
      if (event.defaultPrevented) return
      exitReplay()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [isReplaying, exitReplay])

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
            <GraphStateCard
              className={GRAPH_BOX_HEIGHT_CLASS}
              message="The map is unavailable. Browse the scene as a list below."
            />
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
            replay={canvasReplay}
            isReplayActive={isReplaying}
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
          // The same home city the canvas captions (PSY-1736), so the panel and
          // the dot agree. Null when the label has none — the panel drops the
          // line rather than saying "location unknown".
          meta={selectedHub.homeCity}
          primary={{
            kind: 'emphasis',
            text: `${selectedHub.degree} ${selectedHub.degree === 1 ? 'artist' : 'artists'} on the map`,
          }}
          onClose={closeHub}
        />
      )}

      <IsolateBand count={map.isolateCount} dimmed={isReplaying} />
      <SceneMapList map={map} onSelectArtist={handleSelectArtist} />
      {/* The scrubber REPLACES the freshness strip, and only while a run is in
          flight — the at-rest map keeps exactly the chrome it shipped with. */}
      {isReplaying && replayTimeline ? (
        <ReplayScrubber replay={replay} timeline={replayTimeline} />
      ) : (
        <FreshnessFooter
          map={map}
          onWatchItGrow={canReplay ? replay.start : undefined}
        />
      )}
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
function IsolateBand({ count, dimmed }: { count: number; dimmed: boolean }) {
  if (count <= 0) return null
  return (
    <p
      className={`border-t border-border/50 px-4 py-2.5 text-xs text-muted-foreground transition-opacity duration-300 motion-reduce:transition-none ${
        // Recedes during a run (locked scope edge): the count of artists NOT on
        // the map is a fact about now, and the map on screen is not showing now.
        // Dimmed rather than hidden — the row disappearing would jump the whole
        // card's layout at the moment the replay wants attention on the canvas.
        dimmed ? 'opacity-45' : ''
      }`}
    >
      {isolateShelfCaption(count)}.{' '}
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

/**
 * When the map was built, what is on it — and the one way into the growth
 * replay.
 *
 * The entry point is a chip in this row rather than a control on the canvas
 * (locked): the at-rest map stays chrome-free, and the invitation sits with the
 * other facts about the snapshot, which is where a visitor is already reading
 * when they wonder how it got this way.
 */
function FreshnessFooter({
  map,
  onWatchItGrow,
}: {
  map: SceneMap
  /** Absent when this snapshot has no watchable history, or below the canvas breakpoint. */
  onWatchItGrow?: () => void
}) {
  // The share affordance sits in THIS row rather than on the canvas, for the
  // same reason the replay chip does: the at-rest map stays chrome-free, and the
  // invitation belongs beside the other facts about the snapshot.
  //
  // Gated on the week being SHAREWORTHY, not merely resolvable. The page itself
  // renders either way — it answers 200 with an empty state when there is no
  // snapshot, because it cannot answer 404 (see `app/graph/this-week/page.tsx`)
  // — so this gate is not protecting anyone from a dead link. It is a taste
  // rule: a card reading `+0 ARTISTS · +0 CONNECTIONS` is truthful and is not
  // something to invite anyone to post. Derived from the same
  // `resolveGraphWeek` the card uses, so the chip and the card can never
  // disagree about whether there is a week.
  //
  // Owned HERE rather than by the host, unlike the replay chip: that gate is
  // shared with the page header's status line and the canvas, so it has to live
  // upstream of all three. This one has a single consumer and needs only `map`,
  // which is already a prop.
  const shareableWeek = useMemo(() => {
    const week = resolveGraphWeek(map)
    return week && isGraphWeekShareworthy(week) ? week : null
  }, [map])

  const lastMapped = Number.isNaN(map.lastMapped.getTime())
    ? null
    : map.lastMapped.toLocaleDateString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      })
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-t border-border/50 px-4 py-2.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
      <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
        {onWatchItGrow && (
          <button
            type="button"
            onClick={onWatchItGrow}
            className={MAP_CARD_CHIP_CLASS}
          >
            <Play className="size-3 fill-current" aria-hidden="true" />
            Watch it grow
          </button>
        )}
        {shareableWeek && (
          // A plain link, not a copy-to-clipboard or a share-sheet button: the
          // thing worth sharing is a URL, and handing someone the page it points
          // at lets them see the card before they post it.
          <Link
            href={GRAPH_WEEK_PATH}
            className={`${MAP_CARD_CHIP_CLASS} shrink-0`}
            aria-label={`Share this week in the graph. ${graphWeekSummary(shareableWeek)}`}
          >
            <Share2 className="size-3" aria-hidden="true" />
            This week
          </Link>
        )}
        <span>Mapped nightly{lastMapped ? ` · Last mapped ${lastMapped}` : ''}</span>
      </span>
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
 * A closed `<details>` HIDES its children, it does not skip rendering them —
 * React builds the DOM for everything inside regardless. At catalog scale that
 * is tens of thousands of elements mounted, invisibly, on the page every
 * visitor lands on. So the open region is tracked in state and only ITS members
 * are rendered; a closed region still shows its own name and count, which come
 * from the group rather than from the rows.
 *
 * No per-region cap, though: a capped list would quietly offer LESS than the
 * canvas does, which is the one thing this list must not do.
 */
function SceneMapList({
  map,
  onSelectArtist,
}: {
  map: SceneMap
  onSelectArtist: (node: SceneMapNode) => void
}) {
  const groups = useMemo(() => groupNodesByRegion(map), [map])
  const [openKey, setOpenKey] = useState<string | null>(null)

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
            <details
              className="rounded-lg border border-border/50 bg-muted/10 px-3 py-2"
              open={openKey === group.key}
              // Closing one region must not close the one being opened.
              // Re-rendering with `open={false}` makes the browser fire a
              // `toggle` on the region that just closed — so a naive handler
              // reads that as "nothing is open" and immediately shuts the
              // region the visitor actually clicked. Every region after the
              // first would need two clicks, on the surface that IS the map on
              // a phone. The functional updater only clears when the group
              // reporting closed is still the one we think is open, and cannot
              // read a stale `openKey` from its closure.
              onToggle={event => {
                const isOpen = event.currentTarget.open
                setOpenKey(current =>
                  isOpen ? group.key : current === group.key ? null : current,
                )
              }}
            >
              <summary className="cursor-pointer text-sm">
                <span className="font-medium">{group.label}</span>{' '}
                <span className="text-xs text-muted-foreground">
                  {group.nodes.length.toLocaleString()}{' '}
                  {group.nodes.length === 1 ? 'artist' : 'artists'}
                </span>
              </summary>
              <ul className="mt-2 divide-y divide-border/50">
                {openKey === group.key && group.nodes.map(node => (
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
