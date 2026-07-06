'use client'

import { useState, useCallback, useRef, useEffect, useMemo } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams, usePathname } from 'next/navigation'
import {
  Loader2,
  ThumbsUp,
  ThumbsDown,
  X,
  RotateCcw,
  ChevronRight,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { BracketLink, SectionHeader } from '@/components/shared'
import { useIsAuthenticated } from '@/features/auth'
import { useArtistGraph, useFetchArtistGraph, useArtistRelationshipVote, useCreateArtistRelationship } from '../hooks/useArtistGraph'
import { useArtistSearch } from '../hooks/useArtistSearch'
import { useArtist } from '../hooks/useArtists'
import { useReducedMotion } from '../hooks/useReducedMotion'
import { ArtistGraphVisualization, ConnectionPanelDismissContext, dismissConnectionPanelOnEscape, type ConnectionPanelDismissHandle } from './ArtistGraph'
import { mergeEgoGraphs } from './mergeEgoGraphs'
import { computeGraphDoi, selectSuggestedExpansions, doiWeightsForBias } from './graphDoi'
import { GraphAccessibleTree } from '@/components/graph/GraphAccessibleTree'
import { buildGraphTree, flattenVisibleTree } from '@/components/graph/graphTreeModel'
import {
  buildExpandAnnouncement,
  buildCollapseAnnouncement,
  buildDepthAnnouncement,
  buildCollapseAllAnnouncement,
  buildExpandErrorAnnouncement,
  buildFilterAnnouncement,
} from '@/components/graph/graphAnnouncements'
import {
  MAX_TRAIL_SLOTS,
  pushTrail,
  truncateTrail,
  resetTrail,
  buildRecenterAnnouncement,
  type TraversalEntry,
} from './graphTraversalHistory'
import type { ArtistGraph, ArtistGraphLink, ArtistGraphNode } from '../types'

// Per-relationship-type badge styling. PSY-1290: each type carries BOTH a light- and a dark-mode
// palette. The original classes were dark-only (`bg-{hue}-900/30 text-{hue}-300`) — light text on a
// dark wash, illegible in light mode (the wash composites to near-white and the light text vanishes).
// The `dark:` variants below are exactly those original (good) dark values, so dark mode is unchanged;
// the base classes are the new light palette (deeper `-800` text on a `-100` tint, `-300` border) so
// the badge is legible in light mode too.
//
// These badge FILLS are a SEPARATE, hand-synced palette from the graph EDGE strokes (the `--edge-*`
// CSS vars in globals.css + edgeGrammar.ts, WCAG-audited in PSY-1083) — they are NOT auto-coupled. A
// badge needs a bg+text+border triple; an edge is a single stroke color, so they can't share a value.
// The hues are CHOSEN to evoke the matching edge (teal=radio, blue=shared-bills, …) so a badge reads
// as its edge, but they're picked by eye, not derived — e.g. festival_cobill is Tailwind `orange` here
// while its edge is the Okabe-Ito vermillion `#d55e00`. If you re-tune an edge hue, update this map by
// hand to keep the badge and its edge consistent.
//
// Adding a type: give it the light recipe `bg-{hue}-100 text-{hue}-800 border-{hue}-300` + the dark
// recipe `dark:bg-{hue}-900/30 dark:text-{hue}-300 dark:border-{hue}-700/50`, and also add it to
// ALL_TYPES (below) or its graph toggle won't render. Used in two places: the sidebar list rows AND
// the graph dialog's type-filter toggles.
const RELATIONSHIP_BADGES: Record<string, { label: string; className: string }> = {
  similar: { label: 'Similar', className: 'bg-zinc-200 text-zinc-700 border-zinc-300 dark:bg-zinc-700/50 dark:text-zinc-300 dark:border-zinc-600' },
  shared_bills: { label: 'Shared Bills', className: 'bg-blue-100 text-blue-800 border-blue-300 dark:bg-blue-900/30 dark:text-blue-300 dark:border-blue-700/50' },
  shared_label: { label: 'Shared Label', className: 'bg-purple-100 text-purple-800 border-purple-300 dark:bg-purple-900/30 dark:text-purple-300 dark:border-purple-700/50' },
  side_project: { label: 'Side Project', className: 'bg-green-100 text-green-800 border-green-300 dark:bg-green-900/30 dark:text-green-300 dark:border-green-700/50' },
  member_of: { label: 'Member Of', className: 'bg-amber-100 text-amber-800 border-amber-300 dark:bg-amber-900/30 dark:text-amber-300 dark:border-amber-700/50' },
  radio_cooccurrence: { label: 'Radio Co-occurrence', className: 'bg-teal-100 text-teal-800 border-teal-300 dark:bg-teal-900/30 dark:text-teal-300 dark:border-teal-700/50' },
  // PSY-363: festival_cobill — vermillion-ish styling for the list badge.
  festival_cobill: { label: 'Festival co-lineup', className: 'bg-orange-100 text-orange-800 border-orange-300 dark:bg-orange-900/30 dark:text-orange-300 dark:border-orange-700/50' },
}

const ALL_TYPES = ['similar', 'shared_bills', 'shared_label', 'side_project', 'member_of', 'radio_cooccurrence', 'festival_cobill']

// PSY-954: festival_cobill is a query-time signal the backend now treats as
// strictly opt-in — an empty `types` filter returns STORED types only. Sharing
// one festival lineup says nothing about musical similarity, so it must never
// seed the default graph view (it floods the graph into an unreadable hairball).
// It stays toggleable in the graph dialog, but starts OFF.
const FESTIVAL_COBILL_TYPE = 'festival_cobill'

// The STORED relationship types — everything except the query-time
// festival_cobill signal. Two uses, hence two aliases below:
//   - as DEFAULT_ACTIVE_TYPES: the toggles active on open (festival starts off)
//   - as the fetch payload when opting into festival_cobill: we fetch these
//     PLUS festival_cobill so the stored relationships keep loading alongside
//     the festival edges (passing festival_cobill alone would make the backend
//     skip the stored-rels query entirely).
const STORED_TYPES = ALL_TYPES.filter(t => t !== FESTIVAL_COBILL_TYPE)

// Toggles active by default = the stored types only. festival_cobill is opt-in
// (toggle starts off; turning it on lazy-fetches the festival edges).
const DEFAULT_ACTIVE_TYPES = STORED_TYPES

// PSY-361: URL query param that encodes the currently re-centered artist's
// slug. Absent means the route's original artist is the center. Stored as a
// slug (not an ID) so links are human-readable and shareable.
const CENTER_QUERY_KEY = 'center'

// PSY-1273: how many unexpanded nodes get flagged as suggested expansion directions. Capped
// (van Ham & Perer "highlight <5") so the graph guides the eye to a few high-DOI next steps
// instead of lighting up every neighbor — and so a freshly-expanded hub can't flag all of
// the neighbors it just revealed (the cap is over the WHOLE graph, see selectSuggestedExpansions).
const MAX_SUGGESTED_DIRECTIONS = 5

// PSY-1303: at depth 2 we auto-expand only the top-K DOI-ranked 1-hop neighbours,
// NOT every neighbour — a bounded ~K fetches that keeps the graph legible instead
// of re-introducing the hairball the whole ego arc (PSY-1257..1275) fixed. The
// per-expansion node/edge caps still apply on top of this. Labelled honestly in
// the control ("top N connections") so "2 hops" doesn't overpromise a full 2-hop view.
const DEPTH_2_TOP_K = 8

// Two top-level exports: `ArtistSimilarSidebar` is the sidebar dense list;
// `ArtistGraphDialog` is the on-demand modal hosting `RecenteringGraph`.
// The parent (ArtistDetail) owns the Dialog open state so the header
// [Graph] link, the sidebar [Explore graph] link, and the `#graph` URL
// hash auto-open all drive the same Dialog.

interface ArtistSimilarSidebarProps {
  artistId: number
  artistSlug: string
  /** Fires when the user clicks the `[Explore graph]` affordance. Parent
   *  opens the graph Dialog. */
  onOpenGraph: () => void
}

// PSY-1280: the sidebar's accessible, motion-free counterpart to the canvas DOI ranking
// (PSY-1273) + discovery-bias slider (PSY-1260), which are canvas-only and hidden for
// prefers-reduced-motion users. The "Similar artists" list keeps its existing max-edge-score
// order by default; choosing a Discovery mode re-orders it by the SAME Degree-of-Interest score
// the canvas uses (computed here over the base ego graph), announced via aria-live. Discrete
// modes — not a range slider — are the most screen-reader-friendly and keep the opt-in explicit
// (default 'relevant' is unchanged until the user picks Discovery). Shown to ALL viewers, NOT
// gated on reduced-motion like the canvas slider: a screen-reader/keyboard user need not have
// prefers-reduced-motion set, so gating it there would exclude exactly the users it exists for.
type SidebarSortMode = 'relevant' | 'popular' | 'niche'

// Discovery modes → the PSY-1260 bias value (0 = Popular/high-degree-first, 1 = Niche/low-degree).
const SORT_MODE_BIAS: Record<Exclude<SidebarSortMode, 'relevant'>, number> = {
  popular: 0,
  niche: 1,
}

// The sidebar shows the base ego graph only (no expand-on-demand), so DOI is computed with an
// empty expansion set. Module const → stable identity for the DOI useMemo dep.
const NO_EXPANSIONS: ReadonlyMap<number, ArtistGraph> = new Map()

export function ArtistSimilarSidebar({
  artistId,
  artistSlug,
  onOpenGraph,
}: ArtistSimilarSidebarProps) {
  const { data: graph, isLoading } = useArtistGraph({
    artistId,
    enabled: artistId > 0,
  })
  const { isAuthenticated } = useIsAuthenticated()
  const [showSuggest, setShowSuggest] = useState(false)
  // PSY-1280: accessible DOI sort mode (default = the existing max-edge-score order) + an
  // aria-live announcement that's empty until the user changes it (so mount stays silent).
  const [sortMode, setSortMode] = useState<SidebarSortMode>('relevant')
  const [sortAnnouncement, setSortAnnouncement] = useState('')

  // DOI scores for the active Discovery mode, over the base ego graph — the SAME computeGraphDoi
  // the canvas uses (incl. the per-node edge cap), so the sidebar order matches the canvas's.
  // Null for 'relevant' (no DOI needed) or before the graph loads. Used ONLY to re-order the
  // already-built shown list below, never to change WHICH artists appear (keeps default opt-in).
  // Note: the shown list is the center-adjacent subset, while DOI's degree/relevance terms are
  // min-max-normalized over the WHOLE drawn graph (a superset). In the base ego graph these
  // coincide (every node is center-adjacent), so the ranking is faithful; a future payload that
  // included a node WITHOUT a center edge would have it influence the normalization without
  // appearing in the list — re-scope DOI to the shown set then if that ever happens.
  const sidebarDoi = useMemo(() => {
    if (!graph || sortMode === 'relevant') return null
    const merged = mergeEgoGraphs(graph, NO_EXPANSIONS)
    return computeGraphDoi(merged, undefined, doiWeightsForBias(SORT_MODE_BIAS[sortMode])).doiByNodeId
  }, [graph, sortMode])

  if (isLoading) return null

  const hasRelationships =
    !!graph && (graph.nodes.length > 0 || graph.links.length > 0)

  // Hide the section entirely when there's nothing to show AND the viewer
  // can't contribute. Authenticated users always get the suggest affordance.
  if (!hasRelationships && !isAuthenticated) return null

  // Group links by related artist for the list view; sort by max-score-per-edge.
  let sortedArtists: Array<{
    node: ArtistGraphNode
    links: ArtistGraphLink[]
  }> = []
  if (graph && hasRelationships) {
    const artistLinks = new Map<
      number,
      { links: ArtistGraphLink[]; node: typeof graph.nodes[0] }
    >()
    for (const node of graph.nodes) {
      artistLinks.set(node.id, { links: [], node })
    }
    for (const link of graph.links) {
      const otherId =
        link.source_id === graph.center.id
          ? link.target_id
          : link.target_id === graph.center.id
            ? link.source_id
            : null
      if (otherId && artistLinks.has(otherId)) {
        artistLinks.get(otherId)!.links.push(link)
      }
    }
    sortedArtists = Array.from(artistLinks.values())
      .filter(a => a.links.length > 0)
      .sort((a, b) => {
        // DOI order when a Discovery mode is active; otherwise the default max-edge-score order.
        // DOI is only a re-order key over this already-filtered shown set — the artists shown are
        // identical across modes, only the order changes (so the default ordering stays opt-in).
        if (sidebarDoi) {
          const ad = sidebarDoi.get(a.node.id) ?? -Infinity
          const bd = sidebarDoi.get(b.node.id) ?? -Infinity
          if (ad !== bd) return bd - ad
        }
        const aScore = Math.max(...a.links.map(l => l.score))
        const bScore = Math.max(...b.links.map(l => l.score))
        if (bScore !== aScore) return bScore - aScore
        return a.node.id - b.node.id // deterministic final tiebreak (stable order on score ties)
      })
  }

  return (
    <section>
      <SectionHeader
        title="Similar artists"
        action={
          hasRelationships ? (
            <BracketLink label="Explore graph" onClick={onOpenGraph} />
          ) : undefined
        }
      />
      {/* PSY-1280: accessible, motion-free DOI sort. Shown once there's >1 artist to order;
          re-orders the list below by the canvas's DOI ranking (popular- or niche-biased) and
          announces the change via the sr-only live region. Default 'Most relevant' is unchanged. */}
      {sortedArtists.length > 1 && (
        <div className="flex items-center gap-2 mb-2 text-xs text-muted-foreground">
          <label htmlFor={`similar-sort-${artistId}`} className="shrink-0 font-medium">
            Sort
          </label>
          <select
            id={`similar-sort-${artistId}`}
            value={sortMode}
            onChange={e => {
              const mode = e.target.value as SidebarSortMode
              setSortMode(mode)
              setSortAnnouncement(
                mode === 'relevant'
                  ? 'Similar artists sorted by most relevant.'
                  : `Similar artists sorted by discovery, ${mode}-first.`,
              )
            }}
            className="bg-transparent border border-border rounded px-1.5 py-0.5 text-foreground cursor-pointer"
          >
            <option value="relevant">Most relevant</option>
            <option value="popular">Discovery: popular-first</option>
            <option value="niche">Discovery: niche-first</option>
          </select>
        </div>
      )}
      {/* sr-only live region announcing a re-rank (mirrors the graph dialog's PSY-361 region):
          polite + atomic, empty on mount so nothing is announced until the user changes the sort. */}
      <div className="sr-only" aria-live="polite" aria-atomic="true">
        {sortAnnouncement}
      </div>
      {sortedArtists.length > 0 ? (
        <div className="space-y-1">
          {sortedArtists.map(({ node, links }) => (
            <RelatedArtistRow
              key={node.id}
              node={node}
              links={links}
              centerArtistId={artistId}
              centerArtistSlug={artistSlug}
              isAuthenticated={isAuthenticated}
              userVotes={graph?.user_votes}
            />
          ))}
        </div>
      ) : (
        // Empty + authenticated: render an explanatory line above the suggest
        // affordance, matching the pre-PSY-645 "Be the first to suggest one"
        // pattern. (We hit this branch only when authenticated; empty +
        // unauthenticated returns null above.)
        <p className="text-sm text-muted-foreground">
          No similar artists yet. Be the first to suggest one!
        </p>
      )}
      {isAuthenticated && (
        <div className="mt-2">
          {showSuggest ? (
            <SuggestSimilarArtist
              centerArtistId={artistId}
              centerArtistSlug={artistSlug}
              onClose={() => setShowSuggest(false)}
            />
          ) : (
            <BracketLink
              label="Suggest similar"
              onClick={() => setShowSuggest(true)}
            />
          )}
        </div>
      )}
    </section>
  )
}

interface ArtistGraphDialogProps {
  artistId: number
  artistSlug: string
  artistName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ArtistGraphDialog({
  artistId,
  artistSlug,
  artistName,
  open,
  onOpenChange,
}: ArtistGraphDialogProps) {
  // Filter + traversal state lives per-viewing-session in the Dialog.
  // Trail / slug→id cache are reset each time the parent unmounts the
  // Dialog (Radix lazy-mounts DialogContent under `open`).
  // PSY-954: default-active set excludes festival_cobill (opt-in). Turning the
  // festival toggle on lazy-fetches the festival edges (see RecenteringGraph).
  const [activeTypes, setActiveTypes] = useState<Set<string>>(new Set(DEFAULT_ACTIVE_TYPES))
  const [trail, setTrail] = useState<TraversalEntry[]>([])
  const [slugToIdCache, setSlugToIdCache] = useState<Record<string, number>>({})
  const [announcement, setAnnouncement] = useState('')
  const [containerWidth, setContainerWidth] = useState<number | null>(null)
  // PSY-1351: Escape-intercept handle for the ConnectionPanel that floats
  // inside this dialog. ArtistGraphVisualization keeps it current; the
  // onEscapeKeyDown below reads it (see the comment there).
  const connectionDismissRef = useRef<ConnectionPanelDismissHandle | null>(null)

  // Same callback-ref + ResizeObserver pattern as the pre-PSY-645 inline
  // graph (PSY-519). Measures the DialogContent inner container.
  const containerRefCallback = useCallback((node: HTMLDivElement | null) => {
    if (!node) return
    setContainerWidth(node.getBoundingClientRect().width)
    const observer = new ResizeObserver(entries => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width)
      }
    })
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  const toggleType = (type: string) => {
    setActiveTypes(prev => {
      const next = new Set(prev)
      if (next.has(type)) next.delete(type)
      else next.add(type)
      return next
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-5xl max-h-[90vh] overflow-auto"
        onEscapeKeyDown={e => {
          // PSY-1351: the ConnectionPanel floats inside this dialog and can't
          // win Escape against Radix's own capture-phase dismiss (Radix
          // registers first, at open), so one Escape would close the panel AND
          // the dialog. Close an open panel here and swallow the key; the next
          // Escape falls through and closes the dialog. Shared with the test.
          dismissConnectionPanelOnEscape(connectionDismissRef, e)
        }}
      >
        <DialogHeader>
          <DialogTitle>Similar artists · {artistName}</DialogTitle>
        </DialogHeader>
        {/*
          PSY-949: `min-w-0` is load-bearing — do NOT remove it.
          DialogContent is `display: grid`, so this div is a grid item whose
          default `min-width: auto` resolves to its min-content width. The
          ResizeObserver below feeds the measured width to ForceGraph2D as an
          explicit pixel `width`; without `min-w-0` the canvas's min-content
          width forces this item to grow past `max-w-5xl`, the observer fires
          larger, and the canvas balloons every tick (measured 3036px→11254px).
          `min-w-0` lets the item shrink to the grid track instead, capping the
          measured width at the real dialog inner width and breaking the loop.
        */}
        <div ref={containerRefCallback} className="min-w-0 w-full overflow-hidden">
          {containerWidth !== null && (
            <ConnectionPanelDismissContext.Provider value={connectionDismissRef}>
              <RecenteringGraph
                originalArtistId={artistId}
                originalArtistSlug={artistSlug}
                originalArtistName={artistName}
                containerWidth={containerWidth}
                activeTypes={activeTypes}
                onToggleType={toggleType}
                trail={trail}
                setTrail={setTrail}
                slugToIdCache={slugToIdCache}
                setSlugToIdCache={setSlugToIdCache}
                announcement={announcement}
                setAnnouncement={setAnnouncement}
              />
            </ConnectionPanelDismissContext.Provider>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------------------
// PSY-361: Re-centering graph subtree
// ---------------------------------------------------------------------------
//
// Owns:
//   - URL state (?center=<slug>) via Next.js router
//   - Graph data fetch for the *current* center (which may differ from the
//     route's artist if the user has clicked into a related node)
//   - Slug→ID resolution for ?center= reloads (via useArtist by slug)
//   - Type-filter toggles (preserved across re-centers)
//   - Breadcrumb chip rendering + reset button
//   - aria-live announcement on each re-center
//
// Why a sub-component: the URL-driven re-center pipeline (useSearchParams +
// useArtist + useArtistGraph + useEffects to glue them) only matters when
// the graph is actually visible. Wrapping it in its own component lets us
// gate all of that behind `showGraph && graphAvailable` so we don't ship
// useless network traffic on every artist page view.

interface RecenteringGraphProps {
  originalArtistId: number
  originalArtistSlug: string
  originalArtistName: string
  containerWidth: number
  activeTypes: Set<string>
  onToggleType: (type: string) => void
  trail: TraversalEntry[]
  setTrail: React.Dispatch<React.SetStateAction<TraversalEntry[]>>
  slugToIdCache: Record<string, number>
  setSlugToIdCache: React.Dispatch<React.SetStateAction<Record<string, number>>>
  announcement: string
  setAnnouncement: React.Dispatch<React.SetStateAction<string>>
}

function RecenteringGraph({
  originalArtistId,
  originalArtistSlug,
  originalArtistName,
  containerWidth,
  activeTypes,
  onToggleType,
  trail,
  setTrail,
  slugToIdCache,
  setSlugToIdCache,
  announcement,
  setAnnouncement,
}: RecenteringGraphProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  // The active center slug — either the URL's ?center= or the route's slug.
  const centerSlug = searchParams.get(CENTER_QUERY_KEY) ?? originalArtistSlug
  const isOriginalCenter = centerSlug === originalArtistSlug

  // Resolve slug → id. Three sources, in priority order:
  //   1. Original artist (always known) — no fetch
  //   2. slugToIdCache populated by clicks — no fetch
  //   3. useArtist({ artistId: slug }) — one fetch per uncached slug
  const cachedId = slugToIdCache[centerSlug]
  const needsResolve = !isOriginalCenter && cachedId === undefined
  const { data: resolvedArtist, isLoading: resolvingArtist } = useArtist({
    artistId: centerSlug,
    enabled: needsResolve,
  })

  // Cache the slug→id once the resolution lands so we don't refetch on
  // back-navigation through this same slug.
  useEffect(() => {
    if (resolvedArtist && resolvedArtist.id && !slugToIdCache[resolvedArtist.slug]) {
      setSlugToIdCache(prev => ({ ...prev, [resolvedArtist.slug]: resolvedArtist.id }))
    }
  }, [resolvedArtist, slugToIdCache, setSlugToIdCache])

  const centerId = isOriginalCenter
    ? originalArtistId
    : (cachedId ?? resolvedArtist?.id ?? 0)

  // PSY-954: festival_cobill is a LAZY opt-in fetch. The backend returns it
  // only when explicitly requested in `types`; an empty filter = stored types
  // only. So the fetch `types` depends solely on whether the festival toggle
  // is on:
  //   - off → fetch default (no `types`) = all stored types. The non-festival
  //     toggles stay purely client-side display filters over this payload
  //     (ArtistGraphVisualization filters links by activeTypes), so toggling
  //     them never refetches.
  //   - on  → fetch STORED_TYPES + festival_cobill, so the stored relationships
  //     keep loading alongside the festival edges (festival_cobill alone would
  //     make the backend skip the stored-rels query entirely).
  // useArtistGraph keys its query on `types`, so flipping the festival toggle
  // changes the queryKey → triggers the refetch with no setState-in-effect
  // (React 19.2 react-hooks/set-state-in-effect, PSY-919).
  const wantFestival = activeTypes.has(FESTIVAL_COBILL_TYPE)
  const fetchTypes = useMemo(
    () => (wantFestival ? [...STORED_TYPES, FESTIVAL_COBILL_TYPE] : undefined),
    [wantFestival]
  )

  // Fetch the graph for the current center. Same hook, just dynamic ID.
  // The 5-min staleTime in the hook + TanStack Query's keyed cache means
  // we don't refetch when the user clicks back to a previously-visited
  // center — the graph data is served from cache, the transition feels
  // instant.
  const { data: graph, isFetching: fetchingGraph } = useArtistGraph({
    artistId: centerId,
    types: fetchTypes,
    enabled: centerId > 0,
  })

  // True while the new center is loading — either the slug→id resolve
  // is in flight, or the graph fetch is, or both — and the previously
  // rendered graph (if any) is for a different artist than the URL
  // currently asks for. We OR these so the overlay stays up across the
  // sequential resolve→fetch pipeline rather than flickering between
  // phases.
  const graphMatchesUrl = graph && graph.center.id === centerId
  const isRecentering =
    !graphMatchesUrl && (resolvingArtist || (centerId > 0 && fetchingGraph))

  // ── PSY-1259: expand-on-demand ──────────────────────────────────────────────
  // Clicking a satellite fetches ITS ego graph and merges it into the current view
  // (concentric rings = hop depth), so the user walks the graph outward without losing the
  // context that re-center discards. Expansions are client-only state keyed by the expanded
  // node id; the URL still encodes only the center (PSY-361 unchanged). Re-centering is a
  // fresh start, so expansions reset whenever the center slug changes (render-phase, below).
  const fetchGraph = useFetchArtistGraph()
  const [expansions, setExpansions] = useState<Map<number, ArtistGraph>>(new Map())
  const [expandingIds, setExpandingIds] = useState<Set<number>>(() => new Set())

  // PSY-1303: exploration depth. 1 = the base 1-hop ego (+ any manual expand-on-
  // demand); 2 = additionally auto-expand the top-DOI 1-hop neighbours. depthAutoIds
  // records exactly which nodes depth 2 auto-expanded so returning to depth 1
  // collapses those (restoring the base view) WITHOUT discarding manual expansions.
  const [depth, setDepth] = useState<1 | 2>(1)
  const [depthAutoIds, setDepthAutoIds] = useState<Set<number>>(() => new Set())

  // PSY-1260: discovery-bias slider value, 0 = Popular (default — DOI exactly as PSY-1273
  // shipped) … 1 = Niche (boost low-degree/serendipitous artists). Per-viewing-session state;
  // persists across re-centers, resets when the dialog unmounts. Feeds the DOI weights below.
  const [diversityBias, setDiversityBias] = useState(0)

  // PSY-1260: the slider re-ranks the CANVAS, whose repaint is gated off for reduced-motion
  // users (PSY-1226 — see ArtistGraph's resumeAnimation effect). Rather than present a control
  // that does nothing for them, hide it; they keep the static default-bias graph. (A bias-aware,
  // motion-free accessible path — DOI ordering in the sidebar list + aria-live — is a follow-up.)
  const reducedMotion = useReducedMotion()

  // The exploration is scoped to the current (center, fetch-shape). Re-centering OR toggling
  // festival_cobill (which changes what the BASE fetch returns) invalidates the expansions:
  // their payloads were fetched for a different center, or without the now-wanted festival
  // edges. Reset on either via the render-phase "adjust state during render" pattern
  // (ArtistGraph's hover-reset idiom), avoiding a setState-in-effect. Other type toggles are
  // client-side filters over the already-fetched payload, so they do NOT reset.
  const explorationKey = `${centerSlug} ${wantFestival}`
  const [expansionKey, setExpansionKey] = useState(explorationKey)
  if (explorationKey !== expansionKey) {
    setExpansionKey(explorationKey)
    setExpansions(new Map())
    setExpandingIds(new Set())
    // PSY-1303: re-centering is a fresh exploration — the depth-2 auto-expansions
    // belonged to the prior center, so drop back to depth 1 rather than silently
    // showing "2 hops" over an empty expansion set.
    setDepth(1)
    setDepthAutoIds(new Set())
  }

  // Generation counter so an in-flight expand fetch is applied only if the exploration hasn't
  // been invalidated since it started — covering a re-center / festival toggle (bumped in the
  // effect below, reflecting the COMMITTED key, since a render-phase ref write isn't allowed)
  // AND a Collapse-all (bumped synchronously in collapseAll). Without it, a slow fetch resolving
  // after any of those would re-add a stale expansion ("it popped back").
  const generationRef = useRef(0)
  useEffect(() => {
    generationRef.current += 1
  }, [explorationKey])

  // PSY-1304: ids currently shown (center + base neighbours + every expansion's
  // nodes), kept current so an expand can announce how many artists it ADDED
  // (vs merely re-surfaced). Populated below once graph + expansions are built.
  const knownIdsRef = useRef<Set<number>>(new Set())

  // Click a satellite → expand it (fetch + merge) or, if already expanded, collapse it.
  // mergeEgoGraphs' reachability prune drops any node left dangling by the collapse, so we
  // only track the directly-expanded ids here. A click on a node whose fetch is still in
  // flight is ignored.
  const handleExpand = useCallback(
    (node: { id: number; slug: string; name: string }) => {
      if (expandingIds.has(node.id)) return
      if (expansions.has(node.id)) {
        setExpansions(prev => {
          const next = new Map(prev)
          next.delete(node.id)
          return next
        })
        // PSY-1304: announce the collapse to the shared aria-live region.
        setAnnouncement(buildCollapseAnnouncement(node.name))
        return
      }
      const genAtExpand = generationRef.current
      setExpandingIds(prev => new Set(prev).add(node.id))
      fetchGraph(node.id, fetchTypes)
        .then(ego => {
          // Drop the result if the exploration was invalidated while this was in flight
          // (re-center, festival toggle, or Collapse-all) — it belongs to a reset state.
          if (generationRef.current !== genAtExpand) return
          // PSY-1304: count only the genuinely NEW artists (knownIdsRef reflects
          // the pre-merge shown set) so the announcement is honest even when the
          // ego overlaps the current graph. Computed before the merge below.
          const added = ego.nodes.filter(n => !knownIdsRef.current.has(n.id)).length
          // Update knownIdsRef SYNCHRONOUSLY (it's otherwise repopulated in a
          // passive effect): two concurrent expands resolving in the same batch
          // would both read the stale set and double-count a shared artist.
          for (const n of ego.nodes) knownIdsRef.current.add(n.id)
          setExpansions(prev => new Map(prev).set(node.id, ego))
          setAnnouncement(buildExpandAnnouncement(node.name, added))
        })
        .catch(() => {
          // The graph simply doesn't grow — nothing to roll back — but the
          // accessible path must not go silent (every other outcome speaks).
          if (generationRef.current !== genAtExpand) return
          setAnnouncement(buildExpandErrorAnnouncement(node.name))
        })
        .finally(() => {
          // Only clear THIS fetch's loading state. If the exploration was reset
          // (collapse-all / re-center) and the same node was re-expanded, a newer
          // fetch owns expandingIds now — don't clear it out from under it.
          if (generationRef.current !== genAtExpand) return
          setExpandingIds(prev => {
            const next = new Set(prev)
            next.delete(node.id)
            return next
          })
        })
    },
    [expandingIds, expansions, fetchGraph, fetchTypes, setAnnouncement]
  )

  // The merged client graph + per-node hop distance (null until the base graph loads).
  const merged = useMemo(
    () => (graph ? mergeEgoGraphs(graph, expansions) : null),
    [graph, expansions]
  )
  // Memoized so the `data` handed to the canvas is referentially STABLE across re-renders. A
  // fresh object literal each render would make ArtistGraph's graphData (memoized on [data,
  // activeTypes]) recompute every render and fire its layout-change hover-dismiss on every
  // parent re-render, defeating the hover-grace tooltip (PSY-1218/1220). user_votes is the base
  // center's map only — the merge doesn't union expansion votes, which is fine: the canvas
  // renders no vote UI (the sidebar list owns voting, off its own base-graph fetch).
  const mergedData = useMemo<ArtistGraph | null>(
    () =>
      merged && graph
        ? { center: merged.center, nodes: merged.nodes, links: merged.links, user_votes: graph.user_votes }
        : null,
    [merged, graph]
  )
  const expandedIds = useMemo(() => new Set(expansions.keys()), [expansions])

  // PSY-1273: Degree-of-Interest ranking over the merged graph. Drives two things in the
  // canvas: label collision priority (most-interesting names survive the cull) and the
  // suggested expansion directions. Scoped to `activeTypes` so it tracks the DRAWN graph —
  // toggling a relationship type off re-ranks (and stops scoring nodes whose only ties were
  // hidden), keeping labels + suggestions consistent with what's on screen.
  // PSY-1260: the discovery-bias slider supplies the weights — dragging toward Niche flips the
  // importance term so low-degree artists surface. Memoized on [merged, activeTypes,
  // diversityBias] so it recomputes only when the graph, the toggles, or the bias change.
  const doi = useMemo(
    () => (merged ? computeGraphDoi(merged, activeTypes, doiWeightsForBias(diversityBias)) : null),
    [merged, activeTypes, diversityBias]
  )

  // PSY-1303: depth 2 — auto-expand the top-K DOI-ranked 1-hop neighbours in ONE
  // batch (fetch in parallel, merge together, a single aria-live announcement
  // instead of one per node). Generation-guarded exactly like handleExpand so a
  // re-center / festival toggle / collapse-all / return-to-depth-1 in flight
  // discards the stale result. Already-expanded (manual) neighbours are skipped,
  // so depth 2 is additive; the per-expansion node/edge caps still bound each ego.
  const expandTopKByDoi = useCallback(() => {
    if (!graph || !doi) return
    const targets = graph.nodes
      .filter(n => !expansions.has(n.id) && !expandingIds.has(n.id))
      .map(n => ({ node: n, score: doi.doiByNodeId.get(n.id) ?? -Infinity }))
      .sort((a, b) => b.score - a.score)
      .slice(0, DEPTH_2_TOP_K)
      .map(x => x.node)
    if (targets.length === 0) {
      setAnnouncement(buildDepthAnnouncement(2, 0))
      return
    }
    const ids = targets.map(n => n.id)
    const genAtExpand = generationRef.current
    setExpandingIds(prev => {
      const next = new Set(prev)
      for (const id of ids) next.add(id)
      return next
    })
    Promise.allSettled(
      targets.map(n => fetchGraph(n.id, fetchTypes).then(ego => ({ id: n.id, ego })))
    )
      .then(results => {
        if (generationRef.current !== genAtExpand) return
        const ok = results.flatMap(r => (r.status === 'fulfilled' ? [r.value] : []))
        // Count genuinely-new artists across the whole batch, mutating knownIdsRef
        // synchronously HERE (not in the setState updater, which can run twice) so
        // overlapping egos don't double-count — mirrors handleExpand.
        let added = 0
        for (const { ego } of ok) {
          for (const nd of ego.nodes) {
            if (!knownIdsRef.current.has(nd.id)) added++
            knownIdsRef.current.add(nd.id)
          }
        }
        setExpansions(prev => {
          const next = new Map(prev)
          for (const { id, ego } of ok) next.set(id, ego)
          return next
        })
        setDepthAutoIds(prev => {
          const next = new Set(prev)
          for (const { id } of ok) next.add(id)
          return next
        })
        setAnnouncement(buildDepthAnnouncement(2, added))
      })
      .finally(() => {
        if (generationRef.current !== genAtExpand) return
        setExpandingIds(prev => {
          const next = new Set(prev)
          for (const id of ids) next.delete(id)
          return next
        })
      })
  }, [graph, doi, expansions, expandingIds, fetchGraph, fetchTypes, setAnnouncement])

  // PSY-1303: toggle exploration depth. → 2 auto-expands the top-K; → 1 removes
  // exactly the auto-expanded set (mergeEgoGraphs prunes the freed second-hop
  // nodes) so any MANUAL expansions survive, and bumps the generation to cancel
  // an auto-expand still in flight (a fast 2→1 must not let it pop back).
  const handleDepthChange = useCallback(
    (next: 1 | 2) => {
      if (next === depth) return
      setDepth(next)
      if (next === 2) {
        expandTopKByDoi()
        return
      }
      generationRef.current += 1
      setExpandingIds(new Set())
      setExpansions(prev => {
        if (depthAutoIds.size === 0) return prev
        const nextMap = new Map(prev)
        for (const id of depthAutoIds) nextMap.delete(id)
        return nextMap
      })
      setDepthAutoIds(new Set())
      setAnnouncement(buildDepthAnnouncement(1, 0))
    },
    [depth, depthAutoIds, expandTopKByDoi, setAnnouncement]
  )

  // PSY-1304: keep the known-id set current (center + base neighbours + every
  // expansion's nodes) so handleExpand can announce how many artists it ADDED.
  useEffect(() => {
    const ids = new Set<number>()
    if (graph) {
      ids.add(graph.center.id)
      for (const n of graph.nodes) ids.add(n.id)
    }
    for (const ego of expansions.values()) {
      for (const n of ego.nodes) ids.add(n.id)
    }
    knownIdsRef.current = ids
  }, [graph, expansions])

  // PSY-1304: the node set the CANVAS actually draws for the current filter —
  // a node is visible iff it has ≥1 link of an active type (the canvas prunes the
  // same way). The tree is filtered to this so it can't list (or let a user
  // expand) artists a type toggle has hidden — the two representations must agree.
  const treeVisibleIds = useMemo(() => {
    if (!merged) return undefined
    const ids = new Set<number>([merged.center.id])
    for (const l of merged.links) {
      if (activeTypes.has(l.type)) {
        ids.add(l.source_id)
        ids.add(l.target_id)
      }
    }
    return ids
  }, [merged, activeTypes])

  // PSY-1304: rows for the accessible connections tree — built from the SAME base
  // graph + expansions the canvas draws, filtered by the active types and ranked
  // by the DOI so the list matches the canvas. Empty until the base graph loads.
  const connectionRows = useMemo(
    () =>
      graph
        ? flattenVisibleTree(
            buildGraphTree(graph, expansions, expandingIds, doi?.doiByNodeId, treeVisibleIds)
          )
        : [],
    [graph, expansions, expandingIds, doi, treeVisibleIds]
  )

  // The top ≤5 DOI-ranked nodes the user hasn't already expanded (or isn't mid-expanding) —
  // these get the "suggested direction" affordance in the canvas. Excluding expanding nodes
  // too keeps a node from being flagged-as-suggested and showing its loading ring at once.
  // Referentially stable (memoized) so ArtistGraph's paint callbacks don't churn per render.
  const suggestedIds = useMemo(() => {
    if (!doi) return new Set<number>()
    const excluded = new Set<number>(expandedIds)
    for (const id of expandingIds) excluded.add(id)
    return new Set(selectSuggestedExpansions(doi.ranked, excluded, MAX_SUGGESTED_DIRECTIONS))
  }, [doi, expandedIds, expandingIds])

  const collapseAll = useCallback(() => {
    generationRef.current += 1 // cancel any in-flight expand — the user asked for zero expansions
    setExpansions(new Map())
    setExpandingIds(new Set())
    // PSY-1304: bulk collapse is a graph state change like single-collapse —
    // announce it too (AC3), so keyboard/SR users aren't left without feedback.
    setAnnouncement(buildCollapseAllAnnouncement())
  }, [setAnnouncement])

  // PSY-361: announce each re-center to assistive tech, after the new payload
  // renders (keyed on the graph's actual center.id, never a stale center while
  // a fetch is in flight). PSY-1304: fire ONLY when the center actually changes
  // — a same-center refetch (e.g. toggling festival_cobill, which has its own
  // filter announcement) must not double-announce (AC3: exactly one per action).
  const announcedCenterIdRef = useRef<number | null>(null)
  useEffect(() => {
    if (!graph) return
    if (announcedCenterIdRef.current === graph.center.id) return
    announcedCenterIdRef.current = graph.center.id
    setAnnouncement(buildRecenterAnnouncement(graph.center.name, graph.nodes.length))
  }, [graph, setAnnouncement])

  // PSY-361: sync the trail with browser back/forward. When the URL's
  // center matches a slug already in the trail, the user has walked
  // back via the browser — truncate the trail at that point so the
  // breadcrumb stays consistent with the URL. When the URL goes back
  // to the original artist (no ?center=), the entire trail clears.
  useEffect(() => {
    if (isOriginalCenter) {
      if (trail.length > 0) setTrail(resetTrail())
      return
    }
    const idx = trail.findIndex(e => e.slug === centerSlug)
    if (idx >= 0) {
      setTrail(prev => truncateTrail(prev, idx))
    }
    // We deliberately key only on `centerSlug` so this fires on URL
    // changes (forward/back navigation), not on every trail mutation —
    // that would loop. Trail mutations from click/jump/reset already
    // sync forward via updateUrl().
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [centerSlug])

  const updateUrl = useCallback(
    (newCenterSlug: string | null, mode: 'push' | 'replace') => {
      const params = new URLSearchParams(searchParams.toString())
      if (!newCenterSlug || newCenterSlug === originalArtistSlug) {
        params.delete(CENTER_QUERY_KEY)
      } else {
        params.set(CENTER_QUERY_KEY, newCenterSlug)
      }
      const qs = params.toString()
      const url = qs ? `${pathname}?${qs}` : pathname
      // Push (not replace): re-centers should land in browser history so
      // back/forward walks the trail. The codebase precedent for in-page
      // query-only navigation is router.push(url, { scroll: false }) —
      // see features/{releases,labels,venues,artists,shows}/components/*List.tsx.
      // Browser back/forward triggers Next.js to re-render with the new
      // searchParams; useSearchParams above is reactive and the pipeline
      // re-runs to fetch the now-current center.
      if (mode === 'push') {
        router.push(url, { scroll: false })
      } else {
        router.replace(url, { scroll: false })
      }
    },
    [searchParams, pathname, router, originalArtistSlug]
  )

  // PSY-361: re-center handler. Called by ArtistGraph when the user clicks
  // a non-center node. The current graph's center moves into the trail
  // (it just became "prior"), and the URL updates to the new target.
  const handleRecenter = useCallback(
    (node: { id: number; slug: string; name: string }) => {
      if (!graph) return
      // Push the *outgoing* center onto the trail. If the trail is empty
      // and we're moving away from the original, the original is what
      // becomes the first chip. (When the user is already mid-trail, the
      // trail has prior entries; the current center moves on top.)
      const outgoing: TraversalEntry = {
        id: graph.center.id,
        slug: graph.center.slug,
        name: graph.center.name,
      }
      setTrail(prev => pushTrail(prev, outgoing))
      // Pre-populate the slug→id cache for the click target so the next
      // graph fetch doesn't need a slug→id round-trip.
      setSlugToIdCache(prev =>
        prev[node.slug] !== undefined ? prev : { ...prev, [node.slug]: node.id }
      )
      updateUrl(node.slug, 'push')
    },
    [graph, setTrail, setSlugToIdCache, updateUrl]
  )

  // PSY-361: breadcrumb chip click — jump back to a prior center.
  // chips carry their source trail index; -1 is the sentinel for the
  // synthetic original-artist anchor (full reset). Real trail entries
  // truncate the trail at their position, since the clicked entry
  // becomes the new center and is therefore no longer "prior".
  const handleTrailJump = useCallback(
    (chip: TraversalEntry & { trailIndex: number }) => {
      if (chip.trailIndex === -1) {
        // Original artist anchor — full reset.
        setTrail(resetTrail())
        updateUrl(null, 'push')
        return
      }
      setTrail(prev => truncateTrail(prev, chip.trailIndex))
      const targetSlug = chip.slug === originalArtistSlug ? null : chip.slug
      updateUrl(targetSlug, 'push')
    },
    [setTrail, updateUrl, originalArtistSlug]
  )

  // PSY-361: reset — clear trail, clear URL param, return to original.
  const handleReset = useCallback(() => {
    setTrail(resetTrail())
    updateUrl(null, 'push')
  }, [setTrail, updateUrl])

  // Derive the chip set we render. Per ticket: "The starting artist
  // always anchors position 1 in the breadcrumb (oldest); current
  // center is implicit." So we always prepend the original artist as
  // the first chip (when we aren't currently centered on it),
  // regardless of whether the trail still contains it — this handles
  // the shared/reloaded-URL case where the user lands mid-traversal
  // with no click history.
  //
  // Cap: MAX_TRAIL_SLOTS total visible chips (the user-decided "max
  // 3 prior centers"). When the trail overflows we drop the oldest
  // *non-anchor* trail entries to preserve the original anchor and
  // the most recent context.
  //
  // Each chip carries its source `trailIndex` so the click handler
  // can truncate the trail at the right spot — `trailIndex === -1`
  // means the chip is the synthetic original anchor (special-cased
  // to a full reset on click).
  const breadcrumbChips = useMemo<Array<TraversalEntry & { trailIndex: number }>>(() => {
    if (isOriginalCenter) return []
    const originalChip = {
      id: originalArtistId,
      slug: originalArtistSlug,
      name: originalArtistName,
      trailIndex: -1, // sentinel for "the synthetic original anchor"
    }
    // Drop any trail entry that already represents the original (so
    // it doesn't show up twice when the user clicks back to the
    // start and then off again).
    const trailWithIdx = trail
      .map((e, i) => ({ ...e, trailIndex: i }))
      .filter(e => e.slug !== originalArtistSlug)
    // Keep up to MAX_TRAIL_SLOTS-1 trail entries after the anchor;
    // prefer the most recent (drop oldest if overfull). Indices are
    // preserved from the original trail array, so clicking still
    // truncates at the correct position even after slicing.
    const tailRoom = MAX_TRAIL_SLOTS - 1
    const trimmedTrail =
      trailWithIdx.length > tailRoom
        ? trailWithIdx.slice(trailWithIdx.length - tailRoom)
        : trailWithIdx
    return [originalChip, ...trimmedTrail]
  }, [isOriginalCenter, trail, originalArtistId, originalArtistSlug, originalArtistName])

  // Filter type toggles — only render filters for types that actually appear in the
  // current MERGED graph (so a type introduced by an expanded node also gets a toggle).
  const linkTypesPresent = useMemo(() => {
    if (!merged) return new Set<string>()
    return new Set(merged.links.map(l => l.type))
  }, [merged])

  // Show the previous frame while the new center is loading — never show
  // an empty container or unmount the canvas. If we don't have a graph at
  // all yet (the first fetch is in flight), fall back to a simple loader.
  // Once we have *any* graph data, even stale, we keep rendering it under
  // the recentering overlay.
  if (!graph || !merged || !mergedData) {
    return (
      <div className="mb-6 flex items-center justify-center rounded-lg border border-border/50" style={{ height: 400 }}>
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="mb-6">
      {/* Breadcrumb + reset */}
      <BreadcrumbNav
        chips={breadcrumbChips}
        currentName={graph.center.name}
        onJump={handleTrailJump}
        onReset={handleReset}
      />

      {/* Type Filter Toggles */}
      <div className="flex flex-wrap gap-1.5 mb-3">
        {ALL_TYPES.map(type => {
          const badge = RELATIONSHIP_BADGES[type]
          const isActive = activeTypes.has(type)
          // PSY-954: festival_cobill always renders — it's the opt-in entry
          // point and won't be present in the default (stored-only) payload, so
          // present-gating it would hide the only way to turn it on. The other
          // (stored) toggles stay present-gated: only show a toggle for a type
          // that actually appears in the current graph.
          if (type !== FESTIVAL_COBILL_TYPE && !linkTypesPresent.has(type)) return null
          return (
            <button
              key={type}
              onClick={() => {
                onToggleType(type)
                // PSY-1304: announce the filter change (AC3). isActive is the
                // pre-toggle state, so the new state is its negation.
                setAnnouncement(buildFilterAnnouncement(badge.label, !isActive))
              }}
              className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full border transition-opacity ${
                badge.className
              } ${isActive ? 'opacity-100' : 'opacity-60'}`}
            >
              <span className={`w-1.5 h-1.5 rounded-full ${isActive ? 'bg-current' : 'bg-transparent border border-current'}`} />
              {badge.label}
            </button>
          )
        })}
      </div>

      {/* PSY-1303: exploration depth. 1 hop = the base ego; 2 hops auto-expands the
          top DOI-ranked neighbours (labelled "top N connections" — deliberately not a
          full 2-hop view, per PSY-1303). Always visible (unlike the canvas-only bias
          slider below): it changes the graph AND the accessible tree, so reduced-motion
          users need it too. aria-pressed makes the active hop count SR-legible. */}
      <div className="flex items-center gap-2 mb-3 text-xs text-muted-foreground">
        <span className="shrink-0 font-medium">Depth</span>
        <div role="group" aria-label="Graph depth" className="inline-flex rounded-md border border-border overflow-hidden">
          {([1, 2] as const).map(d => (
            <button
              key={d}
              type="button"
              onClick={() => handleDepthChange(d)}
              aria-pressed={depth === d}
              className={`px-2 py-0.5 transition-colors ${
                depth === d ? 'bg-accent text-foreground' : 'hover:bg-muted/50'
              }`}
            >
              {d === 1 ? '1 hop' : '2 hops'}
            </button>
          ))}
        </div>
        {depth === 2 && (
          <span className="shrink-0 text-[11px]">top {DEPTH_2_TOP_K} connections</span>
        )}
      </div>

      {/* PSY-1260: discovery-bias slider — interpolates DOI's importance weight from Popular
          (favor hubs, the default) to Niche (boost low-degree / serendipitous artists), re-ranking
          the labels + suggested directions live. The canvas repaint rides the doi re-rank via
          BOTH doiByNodeId (label priority) and suggestedIds in ArtistGraph's resumeAnimation deps
          (a one-notch nudge can change label order without changing the top-5 set). Hidden for
          reduced-motion users since that repaint is gated off for them (see reducedMotion above).
          Native range input = keyboard-accessible; the visible <label> is the accessible name
          (no aria-label, so it matches the visible text — WCAG 2.5.3), aria-valuetext speaks the
          position, and the Popular/Niche end labels are aria-hidden decoration. */}
      {!reducedMotion && (
        <div className="flex items-center gap-2 mb-3 text-xs text-muted-foreground">
          <label htmlFor="discovery-bias" className="shrink-0 font-medium">Discovery bias</label>
          <span className="shrink-0" aria-hidden="true">Popular</span>
          <input
            id="discovery-bias"
            type="range"
            min={0}
            max={1}
            step={0.1}
            value={diversityBias}
            onChange={e => setDiversityBias(Number(e.target.value))}
            aria-valuetext={
              diversityBias === 0
                ? 'Popular'
                : diversityBias === 1
                  ? 'Niche'
                  : diversityBias < 0.5
                    ? 'Leaning popular'
                    : diversityBias > 0.5
                      ? 'Leaning niche'
                      : 'Balanced'
            }
            className="flex-1 max-w-[12rem] accent-indigo-500 cursor-pointer"
          />
          <span className="shrink-0" aria-hidden="true">Niche</span>
        </div>
      )}

      {/* PSY-1259: expansion status + escape hatch. Discloses how far the user has walked
          (so the growing graph isn't a mystery) and offers a one-click return to the 1-hop
          ego. Only shown once something is expanded. */}
      {expansions.size > 0 && (
        <div className="flex items-center gap-2 mb-2 text-xs text-muted-foreground">
          <span>
            Expanded {expansions.size} {expansions.size === 1 ? 'artist' : 'artists'}
          </span>
          <button
            onClick={collapseAll}
            className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-muted-foreground hover:text-foreground transition-colors"
            title="Collapse all expansions back to the starting graph"
          >
            <RotateCcw className="h-3 w-3" />
            <span>Collapse all</span>
          </button>
        </div>
      )}

      <ArtistGraphVisualization
        data={mergedData}
        activeTypes={activeTypes}
        containerWidth={containerWidth}
        onRecenter={handleRecenter}
        onExpand={handleExpand}
        hopByNodeId={merged.hopByNodeId}
        expandedIds={expandedIds}
        expandingIds={expandingIds}
        doiByNodeId={doi?.doiByNodeId}
        suggestedIds={suggestedIds}
        canvasDescribedById="ego-graph-a11y-note"
        isRecentering={isRecentering}
      />

      {/* PSY-1304: the accessible connections list — the keyboard / screen-reader
          equivalent of the role=img canvas above. The sr-only note is what the
          canvas's aria-describedby points at (programmatic association, AC1); the
          role=tree drives the SAME expand-on-demand traversal via arrow keys +
          Enter (AC2). */}
      <p id="ego-graph-a11y-note" className="sr-only">
        The connections list below is the keyboard-accessible, screen-reader
        equivalent of this graph. Use the arrow keys to move between artists,
        Enter to expand a connection, and R to re-center the graph on an artist.
      </p>
      <div className="mt-3">
        <h3 className="text-xs font-semibold text-muted-foreground mb-1">Connections</h3>
        {/* Custom key hint (arrow/Enter are the standard tree keys; R is ours). */}
        <p className="text-[11px] text-muted-foreground mb-1">
          Arrow keys to navigate · Enter to expand · <kbd className="font-sans">R</kbd> to re-center
        </p>
        <GraphAccessibleTree
          id="ego-graph-connections"
          rows={connectionRows}
          label={`Connections for ${graph.center.name}`}
          onToggleExpand={handleExpand}
          onRecenter={handleRecenter}
          emptyLabel="No connections to navigate."
        />
      </div>

      {/* PSY-361/1304: single polite aria-live region — every graph state change
          (re-center, expand, collapse, filter) writes `announcement`, so each
          fires exactly one announcement. sr-only is the standard hidden utility;
          atomic so the whole string is read on each update. */}
      <div className="sr-only" aria-live="polite" aria-atomic="true">
        {announcement}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// PSY-361: Breadcrumb subcomponent
// ---------------------------------------------------------------------------

type BreadcrumbChip = TraversalEntry & { trailIndex: number }

interface BreadcrumbNavProps {
  chips: BreadcrumbChip[]
  currentName: string
  onJump: (chip: BreadcrumbChip) => void
  onReset: () => void
}

function BreadcrumbNav({ chips, currentName, onJump, onReset }: BreadcrumbNavProps) {
  // No chips means we're at the original artist — nothing to navigate
  // back to. Suppress the whole bar to avoid visual clutter on the
  // initial state.
  if (chips.length === 0) return null

  return (
    <nav
      aria-label="Graph traversal history"
      className="flex items-center flex-wrap gap-1.5 mb-3 text-xs"
    >
      {chips.map((chip, i) => (
        <span key={`${chip.trailIndex}-${chip.id}-${i}`} className="flex items-center gap-1">
          <button
            onClick={() => onJump(chip)}
            className="px-2 py-0.5 rounded-full border border-border/60 bg-muted/40 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
            title={`Re-center on ${chip.name}`}
          >
            {chip.name}
          </button>
          <ChevronRight className="h-3 w-3 text-muted-foreground/60" aria-hidden="true" />
        </span>
      ))}
      <span
        className="px-2 py-0.5 rounded-full bg-primary/10 text-primary font-medium"
        aria-current="page"
      >
        {currentName}
      </span>
      <button
        onClick={onReset}
        className="ml-1 inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-muted-foreground hover:text-foreground transition-colors"
        title="Reset to starting artist"
      >
        <RotateCcw className="h-3 w-3" />
        <span>Reset</span>
      </button>
    </nav>
  )
}

// --- Related Artist Row ---

interface RelatedArtistRowProps {
  node: { id: number; name: string; slug: string; city?: string; state?: string; upcoming_show_count: number }
  links: ArtistGraphLink[]
  centerArtistId: number
  centerArtistSlug: string
  isAuthenticated: boolean
  userVotes?: Record<string, string>
}

function RelatedArtistRow({
  node,
  links,
  centerArtistId,
  centerArtistSlug,
  isAuthenticated,
  userVotes,
}: RelatedArtistRowProps) {
  const voteMutation = useArtistRelationshipVote()

  const handleVote = (link: ArtistGraphLink, isUpvote: boolean) => {
    voteMutation.mutate({
      sourceId: link.source_id,
      targetId: link.target_id,
      type: link.type,
      isUpvote,
      centerArtistId,
    })
  }

  // Primary display info
  const similarLink = links.find(l => l.type === 'similar')
  const sharedBillsLink = links.find(l => l.type === 'shared_bills')
  const radioLink = links.find(l => l.type === 'radio_cooccurrence')
  // PSY-363: festival co-lineup link, used in the score-display sub-line.
  const festivalCobillLink = links.find(l => l.type === 'festival_cobill')

  // Format score display
  const getScoreDisplay = () => {
    const parts: string[] = []
    if (similarLink) {
      const pct = Math.round(similarLink.score * 100)
      parts.push(`${pct}% similar`)
    }
    if (sharedBillsLink && sharedBillsLink.detail) {
      const count = (sharedBillsLink.detail as Record<string, unknown>).shared_count
      if (count) {
        parts.push(`${count} shared ${Number(count) === 1 ? 'show' : 'shows'}`)
      }
    }
    if (radioLink && radioLink.detail) {
      const detail = radioLink.detail as Record<string, unknown>
      const coCount = detail.co_occurrence_count
      const stationCount = detail.station_count
      if (coCount) {
        const stationPart = stationCount && Number(stationCount) > 1
          ? ` across ${stationCount} stations`
          : ''
        parts.push(`${coCount}x on radio${stationPart}`)
      }
    }
    if (festivalCobillLink && festivalCobillLink.detail) {
      const detail = festivalCobillLink.detail as Record<string, unknown>
      const count = detail.count
      if (count) {
        parts.push(`${count} shared ${Number(count) === 1 ? 'festival' : 'festivals'}`)
      }
    }
    return parts.join(' · ')
  }

  return (
    <div className="flex items-center gap-3 py-2 px-3 rounded-md hover:bg-muted/50 transition-colors group">
      <Link
        href={`/artists/${node.slug}`}
        // PSY-1288: floor the name column at a readable min-width so the row's other items can't
        // squeeze it down to a single initial. 7rem ≈ a short two-word name (e.g. "Slow Pulp") at
        // text-sm; the sidebar is a fixed lg:w-80 (20rem) so this leaves ~10rem for the rest to use.
        // The name still truncates for genuinely long names, but only past this floor — and every
        // OTHER flexible item below now yields first (badges wrap, score truncates), so the name
        // wins the space contest instead of losing it. (Re-tune this floor if the row font/padding
        // or the sidebar width changes.)
        className="flex-1 min-w-[7rem] flex items-center gap-2"
      >
        <span className="text-sm font-medium truncate group-hover:text-foreground">
          {node.name}
        </span>
      </Link>

      {/* Relationship badges — wrap + shrink (PSY-1288) so a row with several long badges yields
          space to the floored name instead of collapsing it; no badge is hidden, the group just
          wraps to a second line when the row is tight. */}
      <div className="hidden sm:flex items-center justify-end gap-1 flex-wrap min-w-0 shrink">
        {links.map(link => {
          const badge = RELATIONSHIP_BADGES[link.type]
          if (!badge) return null
          return (
            <Badge
              key={link.type}
              variant="outline"
              className={`text-[10px] px-1.5 py-0 ${badge.className}`}
            >
              {badge.label}
            </Badge>
          )
        })}
      </div>

      {/* Score — also yields (min-w-0 truncate, no longer flex-shrink-0) so a long multi-type score
          like "72% similar · 19x on radio across 3 stations" truncates rather than clipping off the
          fixed 320px sidebar's right edge (PSY-1288). Only the small vote buttons below stay fixed,
          so the floored name + votes always fit and everything else degrades gracefully. */}
      <span className="text-xs text-muted-foreground min-w-0 shrink truncate hidden md:block">
        {getScoreDisplay()}
      </span>

      {/* Vote buttons (only for "similar" type) */}
      {isAuthenticated && similarLink && (
        <div className="flex items-center gap-0.5 flex-shrink-0">
          <VoteButton
            link={similarLink}
            direction="up"
            userVotes={userVotes}
            onVote={() => handleVote(similarLink, true)}
            isPending={voteMutation.isPending}
          />
          <VoteButton
            link={similarLink}
            direction="down"
            userVotes={userVotes}
            onVote={() => handleVote(similarLink, false)}
            isPending={voteMutation.isPending}
          />
        </div>
      )}
    </div>
  )
}

// --- Vote Button ---

interface VoteButtonProps {
  link: ArtistGraphLink
  direction: 'up' | 'down'
  userVotes?: Record<string, string>
  onVote: () => void
  isPending: boolean
}

function VoteButton({ link, direction, userVotes, onVote, isPending }: VoteButtonProps) {
  const key = `${link.source_id}-${link.target_id}-${link.type}`
  const userVote = userVotes?.[key]
  const isActive = userVote === direction
  const count = direction === 'up' ? link.votes_up : link.votes_down
  const Icon = direction === 'up' ? ThumbsUp : ThumbsDown

  return (
    <button
      onClick={onVote}
      disabled={isPending}
      className={`inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-xs transition-colors ${
        isActive
          ? direction === 'up'
            // PSY-1290: theme-aware active states. The dark: values are the original (good) dark
            // colors; the base values are the light-mode palette (deeper text on a light tint) so an
            // active up/down vote is legible in light mode, where the old light-on-dark washed out.
            ? 'text-green-800 bg-green-100 dark:text-green-400 dark:bg-green-900/20'
            : 'text-red-800 bg-red-100 dark:text-red-400 dark:bg-red-900/20'
          : 'text-muted-foreground hover:text-foreground'
      }`}
      title={direction === 'up' ? 'Upvote similarity' : 'Downvote similarity'}
    >
      <Icon className="h-3 w-3" />
      {count > 0 && <span>{count}</span>}
    </button>
  )
}

// --- Suggest Similar Artist ---

interface SuggestSimilarArtistProps {
  centerArtistId: number
  centerArtistSlug: string
  onClose: () => void
}

function SuggestSimilarArtist({ centerArtistId, centerArtistSlug, onClose }: SuggestSimilarArtistProps) {
  const [query, setQuery] = useState('')
  const [isOpen, setIsOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(-1)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const { data: searchResults } = useArtistSearch({ query })
  const createRelationship = useCreateArtistRelationship()

  const artists = (searchResults?.artists ?? []).filter(a => a.id !== centerArtistId)

  const handleSelect = useCallback(
    (selectedId: number) => {
      setError(null)
      createRelationship.mutate(
        {
          sourceArtistId: centerArtistId,
          targetArtistId: selectedId,
          type: 'similar',
          centerArtistId,
        },
        {
          onSuccess: () => {
            setSuccess(true)
            setQuery('')
            setIsOpen(false)
            setTimeout(() => {
              setSuccess(false)
              onClose()
            }, 2000)
          },
          onError: (err: Error) => {
            const message = err.message || 'Failed to create relationship'
            if (message.includes('already exists')) {
              setError('This artist pair already has a similarity relationship.')
            } else {
              setError(message)
            }
          },
        }
      )
    },
    [centerArtistId, centerArtistSlug, createRelationship, onClose]
  )

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setQuery(value)
    setIsOpen(value.length > 0)
    setActiveIndex(-1)
    setError(null)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!isOpen || artists.length === 0) {
      if (e.key === 'Escape') {
        onClose()
      }
      return
    }

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        setActiveIndex(prev => (prev < artists.length - 1 ? prev + 1 : 0))
        break
      case 'ArrowUp':
        e.preventDefault()
        setActiveIndex(prev => (prev > 0 ? prev - 1 : artists.length - 1))
        break
      case 'Enter':
        e.preventDefault()
        if (activeIndex >= 0 && activeIndex < artists.length) {
          handleSelect(artists[activeIndex].id)
        }
        break
      case 'Escape':
        onClose()
        break
    }
  }

  return (
    <div className="relative">
      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-sm">
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={handleChange}
            onKeyDown={handleKeyDown}
            onBlur={() => setTimeout(() => setIsOpen(false), 150)}
            placeholder="Search for a similar artist..."
            autoFocus
            autoComplete="off"
            className="w-full text-sm px-3 py-1.5 rounded-md border bg-background text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          />

          {isOpen && artists.length > 0 && (
            <div className="absolute top-full left-0 w-full z-50 mt-1 rounded-md border bg-popover text-popover-foreground shadow-md">
              <div className="max-h-[200px] overflow-y-auto p-1">
                {artists.slice(0, 8).map((artist, i) => (
                  <button
                    type="button"
                    key={artist.id}
                    className={`relative flex w-full cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none ${
                      i === activeIndex
                        ? 'bg-accent text-accent-foreground'
                        : 'hover:bg-accent hover:text-accent-foreground'
                    }`}
                    onMouseDown={e => {
                      e.preventDefault()
                      handleSelect(artist.id)
                    }}
                    onMouseEnter={() => setActiveIndex(i)}
                  >
                    <span className="truncate">{artist.name}</span>
                    {(artist.city || artist.state) && (
                      <span className="ml-auto text-xs text-muted-foreground">
                        {[artist.city, artist.state].filter(Boolean).join(', ')}
                      </span>
                    )}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
        <Button variant="ghost" size="sm" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      {createRelationship.isPending && (
        <div className="mt-2 flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          Creating relationship...
        </div>
      )}

      {error && (
        <p className="mt-2 text-xs text-destructive">{error}</p>
      )}

      {success && (
        <p className="mt-2 text-xs text-success-foreground">Relationship created with your upvote!</p>
      )}
    </div>
  )
}
