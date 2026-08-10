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
  GraphLoadingBox,
  GraphRetryBox,
  GRAPH_BOX_HEIGHT_CLASS,
  GRAPH_BOX_MIN_HEIGHT_CLASS,
} from '@/components/graph/GraphStateCard'
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
  collapseTrail,
  pushTrail,
  resetTrail,
  truncateTrail,
  type TraversalEntry,
} from '@/components/graph/graphTraversalHistory'
import {
  useRandomArtistTarget,
  type RandomArtistTargetResponse,
} from '@/features/discovery/useRandomArtistTarget'
import { useScenes } from '@/features/scenes/hooks/useScenes'
import { useGeoDefaultScene } from '@/lib/hooks/common/useGeoDefaultScene'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import { TOOL_LABEL_TIERS } from '@/components/graph/graphLabels'
import { pickSceneEscapeHatches } from './sceneEscapeHatches'
import { buildSceneMap } from '../sceneMap'
import { isGraphOverviewNotBuilt, useGraphOverview } from '../hooks/useGraphOverview'
import { useGraphStartingPoints } from '../hooks/useGraphStartingPoints'
import { pickRotationSuggestions } from '../startingSuggestions'
import { replayStatusText, useSceneReplay, type SceneReplayController } from '../useSceneReplay'
import { SceneMapZeroState } from './SceneMapZeroState'
import { pickVisitorScene } from './visitorScene'

interface GraphAnchor {
  id: number
  slug: string
  name: string
}

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

// The random-target endpoint's contract, narrowed to a usable anchor: all
// three fields must be present or the target is unusable. Shared by the
// shuffle path and the zero-state fallback so the shape check can't drift.
function anchorFromRandomTarget(
  target: RandomArtistTargetResponse | undefined,
): GraphAnchor | null {
  if (!target?.artist_id || !target.artist_slug || !target.artist_name) {
    return null
  }
  return { id: target.artist_id, slug: target.artist_slug, name: target.artist_name }
}

/**
 * The zero state's "Try searching for {artist}" line.
 *
 * WHERE THE NAMES COME FROM (PSY-1749): the nightly build's connectivity
 * ranking, served by /graph/starting-points and already resolved against the
 * live catalog, so every offered name is a click that lands. This replaced a
 * hardcoded editorial trio whose names were validated by a fuzzy artist search
 * per name — a search that could reject a real artist for being past the result
 * cutoff, which collapsed the rotation to whichever single name happened to
 * survive and made the same suggestion the answer on every visit.
 *
 * When the endpoint has nothing to offer (a catalog before its first nightly
 * build), the surface falls back to a random catalog artist exactly as before.
 */
function RotatingExample({
  onPick,
}: {
  onPick: (anchor: GraphAnchor) => void
}) {
  const reducedMotion = useReducedMotion()
  const [index, setIndex] = useState(0)
  // Pausing while hovered/focused removes the click-vs-rotation race: the
  // name the user is aiming at can't swap out from under the pointer (or a
  // paused screen reader) mid-crossfade.
  const [isPaused, setIsPaused] = useState(false)

  // Drawn ONCE PER MOUNT, which is what makes the rotation vary across visits:
  // the pool the endpoint returns is stable for an hour, so if the draw were
  // stable too every visit in that window would open on the same name.
  //
  // Safe in a `useState` initializer despite this surface being server-rendered.
  // The suggestion query cannot have resolved by the hydration render, so both
  // the server HTML and the first client render are the skeleton below — the
  // seed cannot change any rendered output until after hydration has committed.
  const [rotationSeed] = useState(() => Math.floor(Math.random() * 0x7fffffff))

  const startingPointsQuery = useGraphStartingPoints()
  const pool = startingPointsQuery.data?.artists
  const suggestions = useMemo(
    () => pickRotationSuggestions(pool ?? [], rotationSeed),
    [pool, rotationSeed],
  )
  const isPoolSettled = !startingPointsQuery.isPending
  const needsFallback = isPoolSettled && suggestions.length === 0

  const { refetch: refetchFallback } = useRandomArtistTarget()
  const [fallback, setFallback] = useState<GraphAnchor | null>(null)
  const [isFallbackSettled, setIsFallbackSettled] = useState(false)

  // Fetching IS the effect here — the fallback name comes from the network,
  // not from anything derivable during render. `cancelled` drops a late
  // result if the surface unmounts or the pool arrives after all.
  useEffect(() => {
    if (!needsFallback) return
    let cancelled = false
    void (async () => {
      try {
        const result = await refetchFallback()
        if (cancelled) return
        const anchor = anchorFromRandomTarget(result.isError ? undefined : result.data)
        if (anchor) {
          setFallback(anchor)
        }
      } catch {
        // Settle with no suggestion; the zero state still offers search and
        // the shuffle badge, which beats promising a name we can't honor.
      } finally {
        if (!cancelled) setIsFallbackSettled(true)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [needsFallback, refetchFallback])

  // One shape for both sources so the sentence, pause wrapper and crossfade
  // can't fork between the ranked and fallback paths. BOTH are catalog anchors
  // (id + slug + name), so activating either centers the graph directly — there
  // is no name lookup left that could fail on a name just promised.
  const anchors: GraphAnchor[] =
    suggestions.length > 0
      ? suggestions.map(suggestion => ({
          id: suggestion.artist_id,
          slug: suggestion.artist_slug,
          name: suggestion.artist_name,
        }))
      : fallback
        ? [fallback]
        : []
  const choices = anchors.map(anchor => ({
    key: `artist-${anchor.id}`,
    name: anchor.name,
    activate: () => onPick(anchor),
  }))

  useEffect(() => {
    if (reducedMotion || isPaused || choices.length < 2) return
    const timer = window.setInterval(() => {
      setIndex(current => (current + 1) % choices.length)
    }, 4000)
    return () => window.clearInterval(timer)
  }, [reducedMotion, isPaused, choices.length])

  // Modulo guard: the offered set can shrink between renders (a background
  // refetch returns a smaller pool, or the ranked path gives way to the single
  // fallback), and a stale index past the new length must never render
  // undefined.
  const activeIndex = choices.length > 0 ? index % choices.length : 0
  const active = choices[activeIndex]

  if (!active) {
    // Nothing offerable yet: hold the name slot open while the pool or the
    // fallback lookup is in flight (no flash of a name that then vanishes),
    // and drop the sentence entirely — never a fragment promising nothing —
    // if both come back empty.
    if (!isPoolSettled || (needsFallback && !isFallbackSettled)) {
      return (
        <p className="text-sm text-muted-foreground">
          Try searching for{' '}
          <span
            aria-hidden="true"
            className="inline-block h-4 w-28 animate-pulse rounded bg-muted align-[-2px] motion-reduce:animate-none"
          />
        </p>
      )
    }
    return null
  }

  return (
    <p className="text-sm text-muted-foreground">
      Try searching for{' '}
      {/* Pause handlers live on the WRAPPER, not on the button: React focus
          events bubble as focusin/focusout, so one pair of handlers here covers
          the button without depending on the button staying interactive. */}
      <span
        onMouseEnter={() => setIsPaused(true)}
        onMouseLeave={() => setIsPaused(false)}
        onFocus={() => setIsPaused(true)}
        onBlur={() => setIsPaused(false)}
      >
        <button
          type="button"
          onClick={active.activate}
          aria-label={`Search for ${active.name}`}
          className="inline-grid text-left align-baseline font-medium text-foreground underline-offset-4 transition-colors hover:text-primary hover:underline focus-visible:text-primary focus-visible:underline focus-visible:outline-none"
        >
          {/* All offered names share one grid cell so the line crossfades in
              place (and reserves the widest name's width — no layout jitter).
              Under reduced motion the rotation is frozen AND the fade is
              disabled. */}
          {choices.map((choice, choiceIndex) => (
            <span
              key={choice.key}
              aria-hidden="true"
              className={`col-start-1 row-start-1 ${
                reducedMotion ? '' : 'transition-opacity duration-500 motion-reduce:transition-none'
              } ${choiceIndex === activeIndex ? 'opacity-100' : 'pointer-events-none opacity-0'}`}
            >
              {choice.name}
            </span>
          ))}
        </button>
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
 * only while the empty state is visible, though that no longer saves the
 * request it once did — the footer's nightly link reads the same 10-minute
 * scenes query on every visit, so by the time this mounts the list is already
 * in cache.
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

/**
 * Which zero-state arm to render, as a pure function of the overview query.
 *
 * Split out so the rule is readable and testable on its own rather than as a
 * chain of ternaries inside JSX — and because "no snapshot yet" being a NORMAL
 * state rather than a failure is the non-obvious part:
 *
 *  - `loading`     — the first fetch is in flight.
 *  - `unavailable` — a settled failure that is NOT "not built yet". The error
 *                    card offers a retry; search is untouched either way.
 *  - `hero`        — nothing to draw: a catalog before its first nightly build
 *                    (the endpoint's 503), or a payload we could not decode.
 *                    Both leave a visitor in the same place, so they share an
 *                    arm — the shipped search-first hero.
 *  - `map`         — a snapshot we can draw.
 */
export function resolveZeroStateView({
  isPending,
  isError,
  error,
  hasMap,
}: {
  isPending: boolean
  isError: boolean
  error: unknown
  hasMap: boolean
}): 'loading' | 'unavailable' | 'hero' | 'map' {
  // A MAP WE ALREADY HAVE ALWAYS WINS. React Query keeps `data` when a
  // background refetch fails, so testing `isError` first would tear a
  // perfectly good on-screen map down and replace it with an error card —
  // or, on a 503 after a database restore, silently revert it to the old
  // hero. Refetches happen on window focus and on reconnect, so this is an
  // ordinary Tuesday, not an edge case. The sibling ego branch in this file
  // has always guarded its error arm with `&& !graph`; this is the same rule.
  if (hasMap) return 'map'
  if (isPending) return 'loading'
  if (isError) return isGraphOverviewNotBuilt(error) ? 'hero' : 'unavailable'
  return 'hero'
}

/**
 * The search-first hero the page opened on before the Map of the Scene
 * (PSY-1474). It is NOT retired: it is the branch for every state where there
 * is no map to draw — a catalog whose first nightly snapshot has not run, a dev
 * seed, a payload we cannot read — and it is what a phone gets above the map's
 * list, since a canvas at that width is a smear with failing tap targets.
 * Extracted from the old inline zero state unchanged.
 */
function ZeroStateHero({
  onShuffle,
  isShuffleBusy,
  onPickSuggestion,
  lookupError,
}: {
  onShuffle: () => void
  isShuffleBusy: boolean
  onPickSuggestion: (anchor: GraphAnchor) => void
  lookupError: string | null
}) {
  return (
    // Deliberately NOT on GRAPH_BOX_HEIGHT_CLASS: this is the zero-state
    // HERO (search-input sibling), not a graph state card — its heights
    // come from the approved /graph concept.
    <div
      className="flex min-h-[420px] flex-col items-center justify-center gap-4 px-6 text-center sm:min-h-[560px]"
      style={{
        backgroundImage: 'radial-gradient(circle, color-mix(in srgb, var(--muted-foreground) 18%, transparent) 1px, transparent 1px)',
        backgroundSize: '22px 22px',
      }}
    >
      <button
        type="button"
        onClick={onShuffle}
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
        <h2 className="font-display text-2xl font-medium">Explore the graph.</h2>
        <RotatingExample onPick={onPickSuggestion} />
        {lookupError && (
          <p role="status" className="text-xs text-destructive">{lookupError}</p>
        )}
      </div>
    </div>
  )
}

/**
 * `REPLAY · Mar 2019 · 412 ON THE MAP` — the header status line while a growth
 * replay runs (PSY-1737).
 *
 * The text is written straight into the DOM from the transport's frame
 * subscription rather than rendered. Both halves of it change on every frame,
 * and rendering them would re-render the whole Observatory sixty times a second
 * for two words — the same reason the playhead itself is not React state.
 *
 * A `useEffect` here is not a fallback: subscribing to something outside React
 * and unsubscribing on unmount is precisely what an effect is for, and there is
 * no render-time way to attach to a live clock. `useSyncExternalStore` is the
 * wrong tool for the same reason it would be for the playhead — it exists to
 * turn an external change into a RE-RENDER, which is the cost being avoided.
 */
function ReplayStatusLine({ replay }: { replay: SceneReplayController }) {
  const textRef = useRef<HTMLSpanElement>(null)
  const { subscribe, timeline } = replay

  useEffect(() => {
    if (!timeline) return
    let last = ''
    return subscribe(frame => {
      const text = replayStatusText(timeline, frame.progress)
      if (text === last) return
      last = text
      if (textRef.current) textRef.current.textContent = text
    })
  }, [subscribe, timeline])

  // `aria-live` is deliberately absent: the run repaints this every frame, and
  // announcing it would flood a screen reader. The scrubber's slider carries the
  // accessible position instead, where it can be read on demand.
  return <span ref={textRef} className="text-foreground" />
}

// Shared by the serendipity footer's two plain links, which must not drift
// apart: they read as one row of alternatives.
const SERENDIPITY_FOOTER_LINK_CLASS =
  'inline-flex items-center gap-1 text-muted-foreground hover:text-foreground'

/**
 * "Tonight's shows" — the visitor's own scene when we can name one, the global
 * listing otherwise.
 *
 * A nightly scene page is the better answer for a visitor we can place: one
 * city, tonight, at the rooms we track, rather than every city at once. What
 * counts as "can place" is deliberately strict — see `pickVisitorScene`, which
 * refuses the neighbouring-metro guess precisely because this link is silent
 * about where it is sending anyone.
 *
 * The LABEL is fixed; only the href moves. This link sits in a wrap row ahead
 * of the shuffle pill and the resolution lands after mount, so a label that
 * grew a city name would drag its siblings sideways under the reader's cursor.
 *
 * `useHydrated` is what makes it safe to derive an href from browser-only
 * state at all: this page is server-rendered, and the geo suggestion comes out
 * of a sessionStorage cache the server cannot see. Without the gate the server
 * HTML and the hydration render could disagree about where this link points.
 */
function TonightShowsLink() {
  const hydrated = useHydrated()
  const geo = useGeoDefaultScene()
  const scenesQuery = useScenes()
  const scene = useMemo(
    () => pickVisitorScene(scenesQuery.data?.scenes ?? [], geo),
    [scenesQuery.data, geo],
  )
  const href = hydrated && scene ? `/scenes/${scene.slug}/tonight` : '/shows'

  return (
    <Link href={href} className={SERENDIPITY_FOOTER_LINK_CLASS}>
      Tonight’s shows <ArrowRight className="size-3.5" aria-hidden="true" />
    </Link>
  )
}

export function GraphObservatory() {
  const { refCallback, containerWidth } = useContainerWidth()
  const [center, setCenter] = useState<GraphAnchor | null>(null)
  const [trail, setTrail] = useState<TraversalEntry[]>([])
  // Bumped on every trail mutation; keys <Trail> so its local disclosure
  // state resets (re-collapses) on any hop, jump, search, or reset. Mutate
  // trail ONLY through updateTrail below — it owns the epoch bump.
  const [trailEpoch, setTrailEpoch] = useState(0)
  const [selectedNode, setSelectedNode] = useState<ArtistGraphSelection | null>(null)
  const [selectionSource, setSelectionSource] = useState<'canvas' | 'list' | null>(null)
  const [lookupError, setLookupError] = useState<string | null>(null)
  // The random rabbit hole is the ONLY async lookup left on this surface (the
  // zero state's suggestions now arrive as catalog anchors and center
  // synchronously), so this is a plain flag rather than the tagged union it was
  // when two buttons competed for the busy treatment. The generation counter
  // below still guards it: a search, reset, or hop mid-lookup must not be
  // overwritten by the result that lands afterwards.
  const [isShuffleLookupPending, setIsShuffleLookupPending] = useState(false)
  const panelRef = useRef<HTMLElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const listTriggerRef = useRef<HTMLButtonElement | null>(null)
  const resetButtonRef = useRef<HTMLButtonElement>(null)
  const lookupGeneration = useRef(0)

  // The single way to mutate the trail: pairs the state update with the
  // epoch bump so no future mutation site can forget the re-collapse.
  const updateTrail = useCallback(
    (next: TraversalEntry[] | ((previous: TraversalEntry[]) => TraversalEntry[])) => {
      setTrail(next)
      setTrailEpoch(epoch => epoch + 1)
    },
    [],
  )

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

  // Cancels an in-flight random-rabbit-hole lookup — bumping the generation
  // makes the pending promise a no-op.
  const cancelPendingLookup = useCallback(() => {
    lookupGeneration.current += 1
    setIsShuffleLookupPending(false)
  }, [])

  const startAt = useCallback((next: GraphAnchor) => {
    setCenter(next)
    updateTrail(resetTrail())
    setSelectedNode(null)
    setSelectionSource(null)
    setLookupError(null)
    listTriggerRef.current = null
  }, [updateTrail])

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
    updateTrail(previous => pushTrail(previous, center))
    setCenter(anchorFromNode(selectedNode))
    setSelectedNode(null)
    setSelectionSource(null)
    listTriggerRef.current = null
    if (shouldRestoreFocus) {
      window.requestAnimationFrame(() => resetButtonRef.current?.focus())
    }
  }, [cancelPendingLookup, center, selectedNode, selectionSource, updateTrail])

  const handleTrailJump = useCallback((entry: TraversalEntry, index: number) => {
    cancelPendingLookup()
    updateTrail(previous => truncateTrail(previous, index))
    setCenter(entry)
    setSelectedNode(null)
    setSelectionSource(null)
    listTriggerRef.current = null
    window.requestAnimationFrame(() => resetButtonRef.current?.focus())
  }, [cancelPendingLookup, updateTrail])

  const handleReset = useCallback(() => {
    cancelPendingLookup()
    setCenter(null)
    updateTrail(resetTrail())
    setSelectedNode(null)
    setSelectionSource(null)
    setLookupError(null)
    listTriggerRef.current = null
    window.requestAnimationFrame(() => searchInputRef.current?.focus())
  }, [cancelPendingLookup, updateTrail])

  const handleCanvasSelect = useCallback((node: ArtistGraphSelection) => {
    cancelPendingLookup()
    listTriggerRef.current = null
    // Second click on the selected node deselects — "put it away", the same
    // toggle grammar useArtistPanelSelection locks on the Section-class
    // surfaces (PSY-1478 review finding: this surface was the one place the
    // gesture didn't release the pin).
    const toggleOff = selectedNode?.id === node.id
    setSelectedNode(toggleOff ? null : node)
    setSelectionSource(toggleOff ? null : 'canvas')
  }, [cancelPendingLookup, selectedNode])

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

  // Edge click opened the ConnectionPanel — deselect the node panel so the
  // two inspectors never stack (and the edge-endpoint focus pin isn't
  // suppressed by a lingering selection — PSY-1478). No focus move: the
  // user's attention just shifted to the connection inspector
  // (useArtistPanelSelection convention on the Section-class surfaces).
  const handleConnectionInspectOpen = useCallback(() => {
    setSelectedNode(null)
    setSelectionSource(null)
    listTriggerRef.current = null
  }, [])

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
    setIsShuffleLookupPending(true)
    try {
      for (let attempt = 0; attempt < RANDOM_GRAPH_ATTEMPTS; attempt += 1) {
        const result = await refetchShuffle()
        if (requestGeneration !== lookupGeneration.current) return
        const anchor = anchorFromRandomTarget(result.isError ? undefined : result.data)
        if (!anchor) break

        const candidateGraph = await fetchArtistGraph(anchor.id)
        if (requestGeneration !== lookupGeneration.current) return
        const hasCenterLink = candidateGraph.links.some(link =>
          link.source_id === anchor.id || link.target_id === anchor.id,
        )
        if (!hasCenterLink) continue

        startAt(anchor)
        return
      }
    } catch {
      // The shared random-target and graph requests both cross the network;
      // collapse either failure into the same recoverable inline state.
    } finally {
      if (requestGeneration === lookupGeneration.current) {
        setIsShuffleLookupPending(false)
      }
    }
    if (requestGeneration === lookupGeneration.current) {
      setLookupError('No rabbit hole is available right now — try again in a moment.')
    }
  }, [fetchArtistGraph, refetchShuffle, startAt])

  // Zero-state suggestion click (PSY-1474 F1, re-sourced in PSY-1749). Both the
  // ranked suggestions and the random fallback arrive as catalog anchors
  // (id + slug + name), so this centers directly — there is no lookup left that
  // could fail on a name the sentence just promised, and therefore no busy
  // state and no error copy for this affordance.
  const handlePickSuggestion = useCallback((anchor: GraphAnchor) => {
    cancelPendingLookup()
    startAt(anchor)
  }, [cancelPendingLookup, startAt])

  // The Map of the Scene (PSY-1725). Fetched unconditionally rather than only
  // while the zero state is showing: a visitor who resets back to it should get
  // the map they already had, not a second load of a payload that changes once
  // a night.
  const overviewQuery = useGraphOverview()
  const sceneMap = useMemo(
    () => (overviewQuery.data ? buildSceneMap(overviewQuery.data) : null),
    [overviewQuery.data],
  )
  const zeroStateView = resolveZeroStateView({
    isPending: overviewQuery.isPending,
    isError: overviewQuery.isError,
    error: overviewQuery.error,
    hasMap: sceneMap !== null,
  })
  const isCanvasUsable = containerWidth !== null && containerWidth >= GRAPH_BREAKPOINT_PX

  // The growth replay's transport (PSY-1737). Owned HERE rather than inside the
  // map card because the header's status line reads the same clock, and two
  // clocks would be two answers to "what year is on screen".
  //
  // GATED ON THE SURFACE, not just on the data. A run belongs to the drawn map:
  // hand the transport a map only while the map arm is what is actually on
  // screen at a width that draws a canvas. Everything downstream reads that one
  // decision instead of re-deriving it, which is what stops a run outliving the
  // surface hosting it — resize to a phone mid-run, or click a dot to re-root,
  // and the scrubber and its Escape handler unmount while a header that decided
  // for itself would keep ticking `REPLAY · …` with no way left to stop it.
  //
  // It also means no timeline is built (a sort plus two passes over every node)
  // for the ego-graph and hero renders that will never draw a map.
  const replayableMap =
    !center && zeroStateView === 'map' && isCanvasUsable ? sceneMap : null
  const replay = useSceneReplay(replayableMap)

  const isShuffleBusy = isShuffleFetching || isShuffleLookupPending
  const graph = graphQuery.data
  const hasCenterConnections = graph?.links.some(link =>
    link.source_id === graph.center.id || link.target_id === graph.center.id,
  ) ?? false
  // True when the no-connections card is the active surface — it then owns
  // the lookup-error slot (adjacent to its own shuffle pill), not the footer.
  const isEmptyGraph = graph !== undefined && !hasCenterConnections
  const activeTypes = useMemo(
    () => new Set(graph?.links.map(link => link.type) ?? []),
    [graph],
  )

  // Whether the hero — which owns the lookup-error slot when it is on screen —
  // is actually mounted. It is NOT simply "no center": the map arm replaces it
  // on a wide viewport, and the footer has to pick the message up there or a
  // failed shuffle is a dead button with no explanation.
  const isHeroMounted =
    !center && (zeroStateView === 'hero' || (zeroStateView === 'map' && !isCanvasUsable))

  const heroZeroState = (
    <ZeroStateHero
      onShuffle={handleShuffle}
      isShuffleBusy={isShuffleBusy}
      onPickSuggestion={handlePickSuggestion}
      lookupError={lookupError}
    />
  )

  return (
    <div className="mx-auto w-full max-w-[1600px] px-4 py-6 sm:px-6 lg:px-8">
      <header className="mb-5 flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="font-mono text-[10px] uppercase tracking-[0.2em] text-primary">
            Music Knowledge Graph
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
            placeholder="Search an artist to begin, or start anywhere on the map"
            className="max-w-2xl flex-1"
          />
          {(center || sceneMap) && (
            <p className="shrink-0 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
              {center ? (
                <>
                  Centered on <span className="text-foreground">{center.name}</span>
                </>
              ) : replay.isActive ? (
                <ReplayStatusLine replay={replay} />
              ) : (
                <>
                  The whole map ·{' '}
                  <span className="text-foreground">
                    {sceneMap?.artistCount.toLocaleString()} artists
                  </span>
                </>
              )}
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
          <div ref={refCallback}>
            {zeroStateView === 'loading' ? (
              <GraphLoadingBox>Mapping the scene…</GraphLoadingBox>
            ) : zeroStateView === 'unavailable' ? (
              // A settled failure that is NOT "no snapshot yet". The search row
              // above is untouched — the map failing must never cost a visitor
              // the one control that always works.
              <GraphRetryBox
                message="The map couldn’t load."
                onRetry={() => overviewQuery.refetch()}
              />
            ) : zeroStateView === 'hero' || !sceneMap ? (
              heroZeroState
            ) : containerWidth === null ? (
              // Width unknown on the FIRST render of a remount — the callback
              // ref has not run yet. Returning to /graph client-side hits this
              // with the snapshot already cached, so without this arm a desktop
              // would paint one frame of the sub-640px treatment (hero + mobile
              // pitch line) before the measurement landed. The centered branch
              // below has always held the box for the same reason.
              <GraphLoadingBox>Mapping the scene…</GraphLoadingBox>
            ) : (
              <>
                {/* Below the canvas breakpoint the hero stays the primary
                    affordance and the map contributes its counts + list. */}
                {!isCanvasUsable && heroZeroState}
                <SceneMapZeroState
                  map={sceneMap}
                  canvasWidth={isCanvasUsable ? containerWidth : null}
                  onSelectArtist={startAt}
                  replay={replay}
                />
              </>
            )}
          </div>
        ) : (
          <div ref={refCallback} className={`relative p-3 ${GRAPH_BOX_MIN_HEIGHT_CLASS}`}>
            {containerWidth === null || graphQuery.isPending ? (
              <GraphLoadingBox>Mapping {center.name}…</GraphLoadingBox>
            ) : graphQuery.isError && !graph ? (
              <GraphRetryBox
                message="This graph couldn’t load."
                onRetry={() => graphQuery.refetch()}
              />
            ) : isEmptyGraph ? (
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
                  {lookupError && (
                    <p role="status" className="text-xs text-destructive">{lookupError}</p>
                  )}
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
                      // Tool-class tier ladder: labels size by DOI/degree
                      // tier, center largest (locked spec).
                      labelTiers={TOOL_LABEL_TIERS}
                      containerWidth={containerWidth}
                      onSelect={handleCanvasSelect}
                      onBackgroundClick={handlePanelClose}
                      onConnectionInspectOpen={handleConnectionInspectOpen}
                      // Pin the focus-dim to the selection (PSY-1478) —
                      // grammar in graphFocus.resolveFocusForeground.
                      focusNodeId={selectedNode?.id ?? null}
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
        <Link href="/scenes" className={SERENDIPITY_FOOTER_LINK_CLASS}>
          Your scene <ArrowRight className="size-3.5" aria-hidden="true" />
        </Link>
        <TonightShowsLink />
        <ShufflePill onClick={handleShuffle} busy={isShuffleBusy} />
        {/* The lookup error renders beside the affordance the user most
            likely clicked: the hero when it is mounted (the fallback zero
            state, and the map's sub-640px arm), the no-connections card when
            that surface — which has its own shuffle pill — is active, and this
            footer slot otherwise. "Otherwise" now includes the MAP zero state,
            where the footer's shuffle pill is the only one on screen. */}
        {lookupError && !isHeroMounted && !isEmptyGraph && (
          <p role="status" className="basis-full text-xs text-destructive">{lookupError}</p>
        )}
      </div>
    </div>
  )
}
