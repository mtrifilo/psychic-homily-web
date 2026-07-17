'use client'

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Ref,
} from 'react'
import Link from 'next/link'
import { useQueryClient } from '@tanstack/react-query'
import { ArrowRight, Loader2, RotateCcw, Shuffle } from 'lucide-react'

import { ArtistContextPanel } from '@/components/graph/ArtistContextPanel'
import { GraphSectionErrorBoundary } from '@/components/graph/GraphSectionErrorBoundary'
import { GraphSkeleton } from '@/components/graph/GraphSkeleton'
import { GRAPH_BOX_HEIGHT_CLASS, GRAPH_BOX_MIN_HEIGHT_CLASS } from '@/components/graph/GraphStateCard'
import {
  GRAPH_BREAKPOINT_PX,
  useContainerWidth,
} from '@/components/graph/useContainerWidth'
import {
  ArtistSearch,
  ArtistGraphVisualization,
  artistSearchQueryOptions,
  useArtistGraph,
  useArtistGraphCard,
  useFetchArtistGraph,
  useReducedMotion,
  type Artist,
  type ArtistGraph,
  type ArtistGraphNode,
  type ArtistGraphSelection,
} from '@/features/artists'
import {
  collapseTrail,
  pushTrail,
  resetTrail,
  truncateTrail,
  type TraversalEntry,
} from '@/components/graph/graphTraversalHistory'
import { useRandomArtistTarget } from '@/features/discovery/useRandomArtistTarget'
import { useScenes } from '@/features/scenes/hooks/useScenes'
import { pickSceneEscapeHatches } from './sceneEscapeHatches'

interface GraphAnchor {
  id: number
  slug: string
  name: string
}

// These are the three artists in the approved /graph concept trail. Keeping
// the example corpus beside the surface makes the editorial choice explicit;
// it is copy, not a hidden ranking rule or production-data dependency.
const CURATED_EXAMPLES = ['Diners', 'Gatecreeper', 'Playboy Manbaby'] as const
const RANDOM_GRAPH_ATTEMPTS = 3

// Refinement-board pill for "A random rabbit hole" (PSY-1474 F2): primary-
// tinted border/fill, pill radius, 13px medium. Shared by the serendipity
// footer and the empty-state escape hatch so the affordance reads the same.
const SHUFFLE_PILL_CLASS =
  'inline-flex items-center gap-1.5 rounded-full border border-primary/40 bg-primary/10 px-3.5 py-1.5 text-[13px] font-medium text-primary transition-colors hover:border-primary/60 hover:bg-primary/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-60'

// Trail chip hit-area (PSY-1474 F3): 4px 8px padding, hover background.
const TRAIL_CHIP_CLASS =
  'rounded-md bg-muted/50 px-2 py-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground'

function anchorFromArtist(artist: Artist): GraphAnchor {
  return { id: artist.id, slug: artist.slug, name: artist.name }
}

function anchorFromNode(node: ArtistGraphSelection): GraphAnchor {
  return { id: node.id, slug: node.slug, name: node.name }
}

function RotatingExample({
  onPick,
  disabled,
}: {
  onPick: (name: string) => void
  disabled?: boolean
}) {
  const reducedMotion = useReducedMotion()
  const [index, setIndex] = useState(0)
  // Pausing while hovered/focused removes the click-vs-rotation race: the
  // name the user is aiming at can't swap out from under the pointer (or a
  // paused screen reader) mid-crossfade.
  const [isPaused, setIsPaused] = useState(false)

  useEffect(() => {
    if (reducedMotion || isPaused) return
    const timer = window.setInterval(() => {
      setIndex(current => (current + 1) % CURATED_EXAMPLES.length)
    }, 4000)
    return () => window.clearInterval(timer)
  }, [reducedMotion, isPaused])

  const active = CURATED_EXAMPLES[index]

  return (
    <p className="text-sm text-muted-foreground">
      Try searching for{' '}
      {/* Mouse pause lives on the WRAPPER: disabled buttons don't dispatch
          mouseleave, so tracking hover on the button itself would strand
          isPaused=true after a failed lookup froze the rotation forever. */}
      <span
        onMouseEnter={() => setIsPaused(true)}
        onMouseLeave={() => setIsPaused(false)}
      >
        <button
          type="button"
          onClick={() => onPick(active)}
          disabled={disabled}
          aria-label={`Search for ${active}`}
          aria-busy={disabled || undefined}
          onFocus={() => setIsPaused(true)}
          onBlur={() => setIsPaused(false)}
          className="inline-grid text-left align-baseline font-medium text-foreground underline-offset-4 transition-colors hover:text-primary hover:underline focus-visible:text-primary focus-visible:underline focus-visible:outline-none disabled:opacity-60"
        >
          {/* All examples share one grid cell so the line crossfades in place
              (and reserves the widest name's width — no layout jitter). Under
              reduced motion the rotation is frozen AND the fade is disabled. */}
          {CURATED_EXAMPLES.map((name, exampleIndex) => (
            <span
              key={name}
              aria-hidden="true"
              className={`col-start-1 row-start-1 ${
                reducedMotion ? '' : 'transition-opacity duration-500 motion-reduce:transition-none'
              } ${exampleIndex === index ? 'opacity-100' : 'pointer-events-none opacity-0'}`}
            >
              {name}
            </span>
          ))}
        </button>
        {disabled && (
          <Loader2 className="ml-1.5 inline size-3.5 animate-spin align-[-2px] text-muted-foreground" aria-hidden="true" />
        )}
      </span>
    </p>
  )
}

// One component for both random-rabbit-hole pills (serendipity footer +
// empty-state escape hatch) so the copy, icon, and busy treatment can't drift.
function ShufflePill({ onClick, busy }: { onClick: () => void; busy: boolean }) {
  return (
    <button type="button" onClick={onClick} disabled={busy} className={SHUFFLE_PILL_CLASS}>
      {busy ? (
        <>
          <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
          Finding a rabbit hole…
        </>
      ) : (
        <>
          A random rabbit hole <Shuffle className="size-3.5" aria-hidden="true" />
        </>
      )}
    </button>
  )
}

function TrailChip({
  entry,
  index,
  onJump,
  buttonRef,
}: {
  entry: TraversalEntry
  index: number
  onJump: (entry: TraversalEntry, index: number) => void
  buttonRef?: Ref<HTMLButtonElement>
}) {
  return (
    <button
      ref={buttonRef}
      type="button"
      onClick={() => onJump(entry, index)}
      className={TRAIL_CHIP_CLASS}
    >
      {entry.name}
    </button>
  )
}

function Trail({
  trail,
  current,
  onJump,
  onReset,
  resetButtonRef,
}: {
  trail: TraversalEntry[]
  current: GraphAnchor
  onJump: (entry: TraversalEntry, index: number) => void
  onReset: () => void
  resetButtonRef?: Ref<HTMLButtonElement>
}) {
  // Plain local state: the PARENT remounts this component (key = a counter
  // bumped on every trail mutation), so any hop, jump, search, or reset
  // resets the disclosure to collapsed. Deriving "should re-collapse" from
  // trail contents was tried and rejected — id signatures alias when a
  // truncate-then-rehop rebuilds an identical sequence.
  const [isExpanded, setIsExpanded] = useState(false)
  const firstRevealedChipRef = useRef<HTMLButtonElement>(null)
  const segments = collapseTrail(trail)
  const isCollapsed = segments.hidden.length > 0 && !isExpanded

  const handleExpand = () => {
    setIsExpanded(true)
    // The disclosure button unmounts on expand; hand focus to the first
    // revealed chip so keyboard users keep their place in the nav.
    window.requestAnimationFrame(() => firstRevealedChipRef.current?.focus())
  }

  return (
    <nav
      aria-label="Graph traversal history"
      className="flex min-h-9 flex-wrap items-center gap-1.5 border-b border-border/50 px-4 py-2 text-xs"
    >
      <span className="mr-1 font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
        Trail
      </span>
      {isCollapsed ? (
        <>
          {segments.leading.map(({ entry, index }) => (
            <TrailChip key={`${entry.id}-${index}`} entry={entry} index={index} onJump={onJump} />
          ))}
          <button
            type="button"
            aria-label={`Show ${segments.hidden.length} more trail entries`}
            onClick={handleExpand}
            className={TRAIL_CHIP_CLASS}
          >
            … {segments.hidden.length} more
          </button>
          {segments.trailing.map(({ entry, index }) => (
            <TrailChip key={`${entry.id}-${index}`} entry={entry} index={index} onJump={onJump} />
          ))}
        </>
      ) : (
        trail.map((entry, index) => (
          <TrailChip
            key={`${entry.id}-${index}`}
            entry={entry}
            index={index}
            onJump={onJump}
            buttonRef={isExpanded && index === 1 ? firstRevealedChipRef : undefined}
          />
        ))
      )}
      {trail.length > 0 && (
        <ArrowRight className="size-3 text-muted-foreground/50" aria-hidden="true" />
      )}
      <span className="font-medium text-foreground" aria-current="page">
        {current.name}
      </span>
      <button
        ref={resetButtonRef}
        type="button"
        onClick={onReset}
        className="ml-auto inline-flex items-center gap-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground hover:text-foreground"
      >
        <RotateCcw className="size-3" aria-hidden="true" />
        Reset
      </button>
    </nav>
  )
}

function AccessibleGraphList({
  graph,
  onSelect,
  collapsible = false,
}: {
  graph: ArtistGraph
  onSelect: (node: ArtistGraphNode, trigger: HTMLButtonElement) => void
  collapsible?: boolean
}) {
  const nodes = graph.nodes
  const list = (
    <ul className="mt-3 divide-y divide-border/50" aria-label={`Artists connected to ${graph.center.name}`}>
      {nodes.map(node => (
        <li key={node.id}>
          <button
            type="button"
            onClick={event => onSelect(node, event.currentTarget)}
            className="flex w-full items-center justify-between gap-3 py-3 text-left hover:text-primary"
          >
            <span className="font-medium">{node.name}</span>
            <span className="text-xs text-muted-foreground">
              {[node.city, node.state].filter(Boolean).join(', ') || 'Artist'}
            </span>
          </button>
        </li>
      ))}
    </ul>
  )

  if (collapsible) {
    return (
      <details className="mt-3 rounded-lg border border-border/50 bg-muted/10 px-4 py-3">
        <summary className="cursor-pointer text-sm font-medium">Browse connections as a list</summary>
        <p className="mt-2 text-sm text-muted-foreground">
          Choose an artist for context, then center the graph there to keep exploring.
        </p>
        {list}
      </details>
    )
  }

  return (
    <div className="rounded-lg border border-border/50 bg-muted/10 p-4">
      <h2 className="font-display text-xl font-medium">Connections for {graph.center.name}</h2>
      <p className="mt-1 text-sm text-muted-foreground">
        Choose an artist for context, then center the graph there to keep exploring.
      </p>
      {list}
    </div>
  )
}

/**
 * Escape hatches for the no-connections empty state (PSY-1474 F4): two scene
 * links anchored on the artist's metro plus the random rabbit hole. Mounted
 * only while the empty state is visible, so the scenes list is fetched only
 * when a hatch can actually render (it's cached for 10 minutes anyway).
 */
function EmptyGraphEscapeHatches({
  city,
  state,
  onShuffle,
  isShuffleBusy,
}: {
  city?: string
  state?: string
  onShuffle: () => void
  isShuffleBusy: boolean
}) {
  const scenesQuery = useScenes()
  const scenes = useMemo(
    () => pickSceneEscapeHatches(scenesQuery.data?.scenes ?? [], city, state),
    [scenesQuery.data, city, state],
  )

  return (
    <div className="flex flex-wrap items-center justify-center gap-2">
      {scenes.map(scene => (
        <Link
          key={scene.slug}
          href={`/scenes/${scene.slug}`}
          className="inline-flex items-center gap-1 rounded-full border border-border/60 bg-muted/30 px-3.5 py-1.5 text-[13px] font-medium text-muted-foreground transition-colors hover:border-border hover:bg-muted hover:text-foreground"
        >
          The {scene.city} scene <ArrowRight className="size-3.5" aria-hidden="true" />
        </Link>
      ))}
      <ShufflePill onClick={onShuffle} busy={isShuffleBusy} />
    </div>
  )
}

export function GraphObservatory() {
  const { refCallback, containerWidth } = useContainerWidth()
  const [center, setCenter] = useState<GraphAnchor | null>(null)
  const [trail, setTrail] = useState<TraversalEntry[]>([])
  // Bumped on every trail mutation; keys <Trail> so its local disclosure
  // state resets (re-collapses) on any hop, jump, search, or reset.
  const [trailEpoch, setTrailEpoch] = useState(0)
  const [selectedNode, setSelectedNode] = useState<ArtistGraphSelection | null>(null)
  const [selectionSource, setSelectionSource] = useState<'canvas' | 'list' | null>(null)
  const [lookupError, setLookupError] = useState<string | null>(null)
  // At most one async discovery lookup is in flight (guarded by the
  // generation counter); the tag says which button owns the busy treatment.
  const [pendingLookup, setPendingLookup] = useState<'shuffle' | 'example' | null>(null)
  const queryClient = useQueryClient()
  const panelRef = useRef<HTMLElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const listTriggerRef = useRef<HTMLButtonElement | null>(null)
  const resetButtonRef = useRef<HTMLButtonElement>(null)
  const lookupGeneration = useRef(0)

  const graphQuery = useArtistGraph({
    artistId: center?.id ?? 0,
    enabled: center !== null,
  })
  const cardQuery = useArtistGraphCard({
    artistId: selectedNode?.id ?? null,
    enabled: selectedNode !== null,
  })
  const {
    refetch: refetchShuffle,
    isFetching: isShuffleFetching,
  } = useRandomArtistTarget()
  const fetchArtistGraph = useFetchArtistGraph()

  // Cancels any in-flight async lookup (random rabbit hole OR example
  // search) — bumping the generation makes the pending promise a no-op.
  const cancelPendingLookup = useCallback(() => {
    lookupGeneration.current += 1
    setPendingLookup(null)
  }, [])

  const startAt = useCallback((next: GraphAnchor) => {
    setCenter(next)
    setTrail(resetTrail())
    setTrailEpoch(epoch => epoch + 1)
    setSelectedNode(null)
    setSelectionSource(null)
    setLookupError(null)
    listTriggerRef.current = null
  }, [])

  const handleArtistSelect = useCallback(
    (artist: Artist) => {
      cancelPendingLookup()
      startAt(anchorFromArtist(artist))
    },
    [cancelPendingLookup, startAt],
  )

  const handleCenterHere = useCallback(() => {
    if (!center || !selectedNode || selectedNode.id === center.id) return
    cancelPendingLookup()
    const shouldRestoreFocus = selectionSource === 'list'
    setTrail(previous => pushTrail(previous, center))
    setTrailEpoch(epoch => epoch + 1)
    setCenter(anchorFromNode(selectedNode))
    setSelectedNode(null)
    setSelectionSource(null)
    listTriggerRef.current = null
    if (shouldRestoreFocus) {
      window.requestAnimationFrame(() => resetButtonRef.current?.focus())
    }
  }, [cancelPendingLookup, center, selectedNode, selectionSource])

  const handleTrailJump = useCallback((entry: TraversalEntry, index: number) => {
    cancelPendingLookup()
    setTrail(previous => truncateTrail(previous, index))
    setTrailEpoch(epoch => epoch + 1)
    setCenter(entry)
    setSelectedNode(null)
    setSelectionSource(null)
    listTriggerRef.current = null
    window.requestAnimationFrame(() => resetButtonRef.current?.focus())
  }, [cancelPendingLookup])

  const handleReset = useCallback(() => {
    cancelPendingLookup()
    setCenter(null)
    setTrail(resetTrail())
    setTrailEpoch(epoch => epoch + 1)
    setSelectedNode(null)
    setSelectionSource(null)
    setLookupError(null)
    listTriggerRef.current = null
    window.requestAnimationFrame(() => searchInputRef.current?.focus())
  }, [cancelPendingLookup])

  const handleCanvasSelect = useCallback((node: ArtistGraphSelection) => {
    cancelPendingLookup()
    listTriggerRef.current = null
    setSelectedNode(node)
    setSelectionSource('canvas')
  }, [cancelPendingLookup])

  const handleListSelect = useCallback((node: ArtistGraphSelection, trigger: HTMLButtonElement) => {
    cancelPendingLookup()
    listTriggerRef.current = trigger
    setSelectedNode(node)
    setSelectionSource('list')
  }, [cancelPendingLookup])

  const handlePanelClose = useCallback(() => {
    const trigger = selectionSource === 'list' ? listTriggerRef.current : null
    setSelectedNode(null)
    setSelectionSource(null)
    listTriggerRef.current = null
    if (trigger) {
      window.requestAnimationFrame(() => trigger.focus())
    }
  }, [selectionSource])

  useEffect(() => {
    if (!selectedNode || selectionSource !== 'list') return
    const frame = window.requestAnimationFrame(() => {
      panelRef.current?.focus({ preventScroll: true })
      panelRef.current?.scrollIntoView?.({ block: 'nearest' })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [selectedNode, selectionSource])

  const handleShuffle = useCallback(async () => {
    const requestGeneration = lookupGeneration.current + 1
    lookupGeneration.current = requestGeneration
    setLookupError(null)
    setPendingLookup('shuffle')
    try {
      for (let attempt = 0; attempt < RANDOM_GRAPH_ATTEMPTS; attempt += 1) {
        const result = await refetchShuffle()
        if (requestGeneration !== lookupGeneration.current) return
        const target = result.isError ? undefined : result.data
        if (!target?.artist_id || !target.artist_slug || !target.artist_name) break

        const candidateGraph = await fetchArtistGraph(target.artist_id)
        if (requestGeneration !== lookupGeneration.current) return
        const hasCenterLink = candidateGraph.links.some(link =>
          link.source_id === target.artist_id || link.target_id === target.artist_id,
        )
        if (!hasCenterLink) continue

        startAt({
          id: target.artist_id,
          slug: target.artist_slug,
          name: target.artist_name,
        })
        return
      }
    } catch {
      // The shared random-target and graph requests both cross the network;
      // collapse either failure into the same recoverable inline state.
    } finally {
      if (requestGeneration === lookupGeneration.current) {
        setPendingLookup(null)
      }
    }
    if (requestGeneration === lookupGeneration.current) {
      setLookupError('No rabbit hole is available right now — try again in a moment.')
    }
  }, [fetchArtistGraph, refetchShuffle, startAt])

  // Zero-state clickable example (PSY-1474 F1): resolve the curated name via
  // the shared artist-search query options (same cache key and lifetimes as
  // the search box), then center the graph on the best match. Shares the
  // random-lookup generation counter so search/reset/shuffle clicks cancel a
  // pending example lookup too.
  const handleExampleSearch = useCallback(async (name: string) => {
    const requestGeneration = lookupGeneration.current + 1
    lookupGeneration.current = requestGeneration
    setLookupError(null)
    setPendingLookup('example')
    try {
      const searchOptions = artistSearchQueryOptions(name)
      const result = await queryClient.fetchQuery(searchOptions)
      if (requestGeneration !== lookupGeneration.current) return
      const artists = result.artists ?? []
      // Exact (case-insensitive) match only: the button's accessible name
      // promises a specific artist — silently substituting a fuzzy hit would
      // center a graph the user didn't ask for.
      const match = artists.find(
        artist => artist.name.toLowerCase() === name.toLowerCase(),
      )
      if (match) {
        startAt(anchorFromArtist(match))
        return
      }
      // Drop the cached miss so a retry (or the search box, which shares
      // this key) actually re-queries instead of serving the fresh empty
      // result for the next five minutes.
      queryClient.removeQueries({ queryKey: searchOptions.queryKey, exact: true })
      setLookupError(`Couldn’t find ${name} right now — try the search box.`)
    } catch {
      if (requestGeneration === lookupGeneration.current) {
        setLookupError(`Couldn’t find ${name} right now — try the search box.`)
      }
    } finally {
      if (requestGeneration === lookupGeneration.current) {
        setPendingLookup(null)
      }
    }
  }, [queryClient, startAt])

  const isShuffleBusy = isShuffleFetching || pendingLookup === 'shuffle'
  const graph = graphQuery.data
  const hasCenterConnections = graph?.links.some(link =>
    link.source_id === graph.center.id || link.target_id === graph.center.id,
  ) ?? false
  const activeTypes = useMemo(
    () => new Set(graph?.links.map(link => link.type) ?? []),
    [graph],
  )
  const isCanvasUsable = containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX

  return (
    <div className="mx-auto w-full max-w-[1600px] px-4 py-6 sm:px-6 lg:px-8">
      <header className="mb-5 flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-primary">Graph Observatory</p>
          <h1 className="font-display text-3xl font-medium tracking-tight sm:text-4xl">
            Follow the threads.
          </h1>
        </div>
        <p className="max-w-xl text-sm text-muted-foreground sm:text-right">
          Search for an artist, inspect their connections, and hop outward without losing your trail.
        </p>
      </header>

      <section className="overflow-visible rounded-xl border border-border/60 bg-card shadow-sm">
        <div className="relative z-50 flex flex-col gap-3 border-b border-border/50 p-3 sm:flex-row sm:items-center">
          <ArtistSearch
            ref={searchInputRef}
            onSelect={handleArtistSelect}
            placeholder="Search an artist to begin…"
            className="max-w-2xl flex-1"
          />
          {center && (
            <p className="shrink-0 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
              Centered on <span className="text-foreground">{center.name}</span>
            </p>
          )}
        </div>

        {center && (
          <Trail
            key={trailEpoch}
            trail={trail}
            current={center}
            onJump={handleTrailJump}
            onReset={handleReset}
            resetButtonRef={resetButtonRef}
          />
        )}

        {!center ? (
          // Deliberately NOT on GRAPH_BOX_HEIGHT_CLASS: this is the zero-state
          // HERO (search-input sibling), not a graph state card — its heights
          // come from the approved /graph concept, and F1 reuses the shipped
          // zero-state layout unchanged.
          <div
            className="flex min-h-[420px] flex-col items-center justify-center gap-4 px-6 text-center sm:min-h-[560px]"
            style={{
              backgroundImage: 'radial-gradient(circle, color-mix(in srgb, var(--muted-foreground) 18%, transparent) 1px, transparent 1px)',
              backgroundSize: '22px 22px',
            }}
          >
            <button
              type="button"
              onClick={handleShuffle}
              disabled={isShuffleBusy}
              aria-label="Take a random rabbit hole"
              className="flex size-14 items-center justify-center rounded-full border border-primary/40 bg-primary/10 text-primary transition-colors hover:border-primary/60 hover:bg-primary/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-60"
              style={{ boxShadow: '0 0 50px color-mix(in srgb, var(--primary) 18%, transparent)' }}
            >
              {isShuffleBusy ? (
                <Loader2 className="size-5 animate-spin" aria-hidden="true" />
              ) : (
                <Shuffle className="size-5" aria-hidden="true" />
              )}
            </button>
            <div className="space-y-1">
              <h2 className="font-display text-2xl font-medium">Pick a name. See what it touches.</h2>
              <RotatingExample onPick={handleExampleSearch} disabled={pendingLookup === 'example'} />
              {lookupError && (
                <p role="status" className="text-xs text-destructive">{lookupError}</p>
              )}
            </div>
          </div>
        ) : (
          <div ref={refCallback} className={`relative p-3 ${GRAPH_BOX_MIN_HEIGHT_CLASS}`}>
            {containerWidth === null || graphQuery.isPending ? (
              <GraphSkeleton className={GRAPH_BOX_HEIGHT_CLASS}>
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                  Mapping {center.name}…
                </div>
              </GraphSkeleton>
            ) : graphQuery.isError && !graph ? (
              <div role="alert" className={`flex flex-col items-center justify-center gap-3 text-center ${GRAPH_BOX_HEIGHT_CLASS}`}>
                <p className="text-sm text-muted-foreground">This graph couldn’t load.</p>
                <button
                  type="button"
                  onClick={() => graphQuery.refetch()}
                  className="text-sm text-primary hover:underline underline-offset-4"
                >
                  Try again
                </button>
              </div>
            ) : graph && !hasCenterConnections ? (
              // min-height (not the fixed-height contract): this card is
              // content-driven — the escape hatches wrap to several rows on
              // narrow phones, and growing beats trapping the third hatch in
              // an inner scroll region.
              <div role="status" className={`flex flex-col items-center justify-center gap-3 px-6 py-4 text-center ${GRAPH_BOX_MIN_HEIGHT_CLASS}`}>
                <div className="flex flex-col items-center gap-3">
                  <div>
                    <h2 className="font-display text-2xl font-medium">No mapped connections yet.</h2>
                    <p className="mt-1 max-w-md text-sm text-muted-foreground">
                      {graph.center.name} is in the catalog, but nothing links to it yet. Try a nearby scene or a random rabbit hole.
                    </p>
                  </div>
                  <EmptyGraphEscapeHatches
                    city={graph.center.city}
                    state={graph.center.state}
                    onShuffle={handleShuffle}
                    isShuffleBusy={isShuffleBusy}
                  />
                  <Link
                    href={`/artists/${graph.center.slug}`}
                    className="inline-flex items-center gap-1 text-sm text-primary hover:underline underline-offset-4"
                  >
                    Open {graph.center.name}’s page <ArrowRight className="size-3.5" aria-hidden="true" />
                  </Link>
                </div>
              </div>
            ) : graph ? (
              <>
                {graphQuery.isError && (
                  <div role="status" className="mb-3 flex items-center justify-between gap-3 rounded-md border border-border/50 bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
                    <span>Showing saved connections while the latest graph is unavailable.</span>
                    <button
                      type="button"
                      onClick={() => graphQuery.refetch()}
                      className="shrink-0 text-primary hover:underline underline-offset-4"
                    >
                      Try again
                    </button>
                  </div>
                )}
                {isCanvasUsable ? (
                  <GraphSectionErrorBoundary
                    sentryTag="graph-observatory"
                    fallback={(
                      <div role="status" className={`flex items-center justify-center text-sm text-muted-foreground ${GRAPH_BOX_HEIGHT_CLASS}`}>
                        The interactive graph is unavailable. Browse its connections below.
                      </div>
                    )}
                  >
                    <ArtistGraphVisualization
                      data={graph}
                      activeTypes={activeTypes}
                      containerWidth={containerWidth}
                      onSelect={handleCanvasSelect}
                      onBackgroundClick={handlePanelClose}
                      showLegend={false}
                      canvasDescribedById="observatory-graph-guidance"
                      canvasAriaLabel={`Artist relationship graph for ${graph.center.name}. Use the Browse connections list below to select an artist.`}
                    />
                  </GraphSectionErrorBoundary>
                ) : (
                  <AccessibleGraphList graph={graph} onSelect={handleListSelect} />
                )}
                {isCanvasUsable && (
                  <AccessibleGraphList graph={graph} onSelect={handleListSelect} collapsible />
                )}
                <p id="observatory-graph-guidance" className="sr-only">
                  Select an artist node for details. Use Center here in the details panel to re-root the graph.
                </p>
                {selectedNode && (
                  <ArtistContextPanel
                    className={isCanvasUsable ? 'absolute right-5 top-5 z-40' : 'mt-3'}
                    artistName={selectedNode.name}
                    artistSlug={selectedNode.slug}
                    card={cardQuery.data}
                    isError={cardQuery.isError}
                    onCenter={selectedNode.id !== center.id ? handleCenterHere : undefined}
                    onClose={handlePanelClose}
                    panelRef={panelRef}
                  />
                )}
              </>
            ) : null}
          </div>
        )}
      </section>

      <div className="mt-4 flex flex-wrap items-center gap-x-6 gap-y-3 border-t border-border/50 pt-4 text-sm">
        <span className="font-display text-base font-medium">No artist in mind?</span>
        <Link href="/scenes" className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground">
          Your scene <ArrowRight className="size-3.5" aria-hidden="true" />
        </Link>
        <Link href="/shows" className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground">
          Tonight’s shows <ArrowRight className="size-3.5" aria-hidden="true" />
        </Link>
        <ShufflePill onClick={handleShuffle} busy={isShuffleBusy} />
        {/* On the zero state the error renders beside the affordance the user
            clicked (in the hero); once a graph is centered, this footer slot
            is the only shuffle entry point, so it renders here instead. */}
        {lookupError && center && (
          <p role="status" className="basis-full text-xs text-destructive">{lookupError}</p>
        )}
      </div>
    </div>
  )
}
