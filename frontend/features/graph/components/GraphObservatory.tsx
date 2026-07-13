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
import { ArrowRight, Loader2, RotateCcw, Shuffle } from 'lucide-react'

import { ArtistContextPanel } from '@/components/graph/ArtistContextPanel'
import { GraphSectionErrorBoundary } from '@/components/graph/GraphSectionErrorBoundary'
import {
  GRAPH_BREAKPOINT_PX,
  useContainerWidth,
} from '@/components/graph/useContainerWidth'
import {
  ArtistSearch,
  ArtistGraphVisualization,
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
  pushTrail,
  resetTrail,
  truncateTrail,
  type TraversalEntry,
} from '@/components/graph/graphTraversalHistory'
import { useRandomArtistTarget } from '@/features/discovery/useRandomArtistTarget'

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

function anchorFromArtist(artist: Artist): GraphAnchor {
  return { id: artist.id, slug: artist.slug, name: artist.name }
}

function anchorFromNode(node: ArtistGraphSelection): GraphAnchor {
  return { id: node.id, slug: node.slug, name: node.name }
}

function RotatingExample() {
  const reducedMotion = useReducedMotion()
  const [index, setIndex] = useState(0)

  useEffect(() => {
    if (reducedMotion) return
    const timer = window.setInterval(() => {
      setIndex(current => (current + 1) % CURATED_EXAMPLES.length)
    }, 4000)
    return () => window.clearInterval(timer)
  }, [reducedMotion])

  return (
    <p className="text-sm text-muted-foreground">
      Try searching for <span className="font-medium text-foreground">{CURATED_EXAMPLES[index]}</span>
    </p>
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
  return (
    <nav
      aria-label="Graph traversal history"
      className="flex min-h-9 flex-wrap items-center gap-1.5 border-b border-border/50 px-4 py-2 text-xs"
    >
      <span className="mr-1 font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
        Trail
      </span>
      {trail.map((entry, index) => (
        <span key={`${entry.id}-${index}`} className="flex items-center gap-1.5">
          <button
            type="button"
            onClick={() => onJump(entry, index)}
            className="text-muted-foreground transition-colors hover:text-foreground hover:underline underline-offset-4"
          >
            {entry.name}
          </button>
          <ArrowRight className="size-3 text-muted-foreground/50" aria-hidden="true" />
        </span>
      ))}
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

export function GraphObservatory() {
  const { refCallback, containerWidth } = useContainerWidth()
  const [center, setCenter] = useState<GraphAnchor | null>(null)
  const [trail, setTrail] = useState<TraversalEntry[]>([])
  const [selectedNode, setSelectedNode] = useState<ArtistGraphSelection | null>(null)
  const [selectionSource, setSelectionSource] = useState<'canvas' | 'list' | null>(null)
  const [shuffleError, setShuffleError] = useState<string | null>(null)
  const [isFindingRandom, setIsFindingRandom] = useState(false)
  const panelRef = useRef<HTMLElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const listTriggerRef = useRef<HTMLButtonElement | null>(null)
  const resetButtonRef = useRef<HTMLButtonElement>(null)
  const randomRequestGeneration = useRef(0)

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

  const cancelRandomSearch = useCallback(() => {
    randomRequestGeneration.current += 1
    setIsFindingRandom(false)
  }, [])

  const startAt = useCallback((next: GraphAnchor) => {
    setCenter(next)
    setTrail(resetTrail())
    setSelectedNode(null)
    setSelectionSource(null)
    setShuffleError(null)
    listTriggerRef.current = null
  }, [])

  const handleArtistSelect = useCallback(
    (artist: Artist) => {
      cancelRandomSearch()
      startAt(anchorFromArtist(artist))
    },
    [cancelRandomSearch, startAt],
  )

  const handleCenterHere = useCallback(() => {
    if (!center || !selectedNode || selectedNode.id === center.id) return
    cancelRandomSearch()
    const shouldRestoreFocus = selectionSource === 'list'
    setTrail(previous => pushTrail(previous, center))
    setCenter(anchorFromNode(selectedNode))
    setSelectedNode(null)
    setSelectionSource(null)
    listTriggerRef.current = null
    if (shouldRestoreFocus) {
      window.requestAnimationFrame(() => resetButtonRef.current?.focus())
    }
  }, [cancelRandomSearch, center, selectedNode, selectionSource])

  const handleTrailJump = useCallback((entry: TraversalEntry, index: number) => {
    cancelRandomSearch()
    setTrail(previous => truncateTrail(previous, index))
    setCenter(entry)
    setSelectedNode(null)
    setSelectionSource(null)
    listTriggerRef.current = null
    window.requestAnimationFrame(() => resetButtonRef.current?.focus())
  }, [cancelRandomSearch])

  const handleReset = useCallback(() => {
    cancelRandomSearch()
    setCenter(null)
    setTrail(resetTrail())
    setSelectedNode(null)
    setSelectionSource(null)
    setShuffleError(null)
    listTriggerRef.current = null
    window.requestAnimationFrame(() => searchInputRef.current?.focus())
  }, [cancelRandomSearch])

  const handleCanvasSelect = useCallback((node: ArtistGraphSelection) => {
    cancelRandomSearch()
    listTriggerRef.current = null
    setSelectedNode(node)
    setSelectionSource('canvas')
  }, [cancelRandomSearch])

  const handleListSelect = useCallback((node: ArtistGraphSelection, trigger: HTMLButtonElement) => {
    cancelRandomSearch()
    listTriggerRef.current = trigger
    setSelectedNode(node)
    setSelectionSource('list')
  }, [cancelRandomSearch])

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
    const requestGeneration = randomRequestGeneration.current + 1
    randomRequestGeneration.current = requestGeneration
    setShuffleError(null)
    setIsFindingRandom(true)
    try {
      for (let attempt = 0; attempt < RANDOM_GRAPH_ATTEMPTS; attempt += 1) {
        const result = await refetchShuffle()
        if (requestGeneration !== randomRequestGeneration.current) return
        const target = result.isError ? undefined : result.data
        if (!target?.artist_id || !target.artist_slug || !target.artist_name) break

        const candidateGraph = await fetchArtistGraph(target.artist_id)
        if (requestGeneration !== randomRequestGeneration.current) return
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
      if (requestGeneration === randomRequestGeneration.current) {
        setIsFindingRandom(false)
      }
    }
    if (requestGeneration === randomRequestGeneration.current) {
      setShuffleError('No rabbit hole is available right now — try again in a moment.')
    }
  }, [fetchArtistGraph, refetchShuffle, startAt])

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
            trail={trail}
            current={center}
            onJump={handleTrailJump}
            onReset={handleReset}
            resetButtonRef={resetButtonRef}
          />
        )}

        {!center ? (
          <div
            className="flex min-h-[420px] flex-col items-center justify-center gap-4 px-6 text-center sm:min-h-[560px]"
            style={{
              backgroundImage: 'radial-gradient(circle, color-mix(in srgb, var(--muted-foreground) 18%, transparent) 1px, transparent 1px)',
              backgroundSize: '22px 22px',
            }}
          >
            <div
              className="flex size-14 items-center justify-center rounded-full border border-primary/40 bg-primary/10"
              style={{ boxShadow: '0 0 50px color-mix(in srgb, var(--primary) 18%, transparent)' }}
            >
              <Shuffle className="size-5 text-primary" aria-hidden="true" />
            </div>
            <div className="space-y-1">
              <h2 className="font-display text-2xl font-medium">Pick a name. See what it touches.</h2>
              <RotatingExample />
            </div>
          </div>
        ) : (
          <div ref={refCallback} className="relative min-h-[240px] p-3 sm:min-h-[400px]">
            {containerWidth === null || graphQuery.isPending ? (
              <div className="flex h-[400px] items-center justify-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                Mapping {center.name}…
              </div>
            ) : graphQuery.isError && !graph ? (
              <div role="alert" className="flex h-[400px] flex-col items-center justify-center gap-3 text-center">
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
              <div role="status" className="flex h-[400px] flex-col items-center justify-center gap-3 px-6 text-center">
                <div>
                  <h2 className="font-display text-2xl font-medium">No mapped connections yet.</h2>
                  <p className="mt-1 max-w-md text-sm text-muted-foreground">
                    {graph.center.name} is in the catalog, but nothing links to it yet. Search another artist or try a random rabbit hole.
                  </p>
                </div>
                <Link
                  href={`/artists/${graph.center.slug}`}
                  className="inline-flex items-center gap-1 text-sm text-primary hover:underline underline-offset-4"
                >
                  Open {graph.center.name}’s page <ArrowRight className="size-3.5" aria-hidden="true" />
                </Link>
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
                      <div role="status" className="flex h-[400px] items-center justify-center text-sm text-muted-foreground">
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
        <Link href="/shows" className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground">
          Tonight’s shows <ArrowRight className="size-3.5" aria-hidden="true" />
        </Link>
        <Link href="/scenes" className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground">
          Your scene <ArrowRight className="size-3.5" aria-hidden="true" />
        </Link>
        <button
          type="button"
          onClick={handleShuffle}
          disabled={isShuffleFetching || isFindingRandom}
          className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground disabled:opacity-50"
        >
          {isShuffleFetching || isFindingRandom ? 'Finding a rabbit hole…' : 'A random rabbit hole'}
          <Shuffle className="size-3.5" aria-hidden="true" />
        </button>
        {shuffleError && <p role="status" className="basis-full text-xs text-destructive">{shuffleError}</p>}
      </div>
    </div>
  )
}
