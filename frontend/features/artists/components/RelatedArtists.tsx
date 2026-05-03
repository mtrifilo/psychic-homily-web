'use client'

import { useState, useCallback, useRef, useEffect, useMemo } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams, usePathname } from 'next/navigation'
import {
  Loader2,
  ThumbsUp,
  ThumbsDown,
  Network,
  X,
  Plus,
  RotateCcw,
  ChevronRight,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { useIsAuthenticated } from '@/features/auth'
import { GRAPH_HASH, useUrlHash } from '@/lib/hooks/common/useUrlHash'
import { useArtistGraph, useArtistRelationshipVote, useCreateArtistRelationship } from '../hooks/useArtistGraph'
import { useArtistSearch } from '../hooks/useArtistSearch'
import { useArtist } from '../hooks/useArtists'
import { ArtistGraphVisualization } from './ArtistGraph'
import {
  MAX_TRAIL_SLOTS,
  pushTrail,
  truncateTrail,
  resetTrail,
  buildRecenterAnnouncement,
  type TraversalEntry,
} from './graphTraversalHistory'
import type { ArtistGraphLink } from '../types'

const RELATIONSHIP_BADGES: Record<string, { label: string; className: string }> = {
  similar: { label: 'Similar', className: 'bg-zinc-700/50 text-zinc-300 border-zinc-600' },
  shared_bills: { label: 'Shared Bills', className: 'bg-blue-900/30 text-blue-300 border-blue-700/50' },
  shared_label: { label: 'Shared Label', className: 'bg-purple-900/30 text-purple-300 border-purple-700/50' },
  side_project: { label: 'Side Project', className: 'bg-green-900/30 text-green-300 border-green-700/50' },
  member_of: { label: 'Member Of', className: 'bg-amber-900/30 text-amber-300 border-amber-700/50' },
  radio_cooccurrence: { label: 'Radio Co-occurrence', className: 'bg-teal-900/30 text-teal-300 border-teal-700/50' },
  // PSY-363: festival_cobill — vermillion-ish styling for the list badge.
  festival_cobill: { label: 'Festival co-lineup', className: 'bg-orange-900/30 text-orange-300 border-orange-700/50' },
}

const ALL_TYPES = ['similar', 'shared_bills', 'shared_label', 'side_project', 'member_of', 'radio_cooccurrence', 'festival_cobill']

// PSY-361: URL query param that encodes the currently re-centered artist's
// slug. Absent means the route's original artist is the center. Stored as a
// slug (not an ID) so links are human-readable and shareable.
const CENTER_QUERY_KEY = 'center'

interface RelatedArtistsProps {
  artistId: number
  artistSlug: string
}

export function RelatedArtists({ artistId, artistSlug }: RelatedArtistsProps) {
  const { data: originalGraph, isLoading } = useArtistGraph({ artistId, enabled: artistId > 0 })
  const { isAuthenticated } = useIsAuthenticated()
  // null = not interacted; URL hash drives the default. User toggle sticks once set.
  const [showGraphOverride, setShowGraphOverride] = useState<boolean | null>(null)
  const hash = useUrlHash()
  const [activeTypes, setActiveTypes] = useState<Set<string>>(new Set(ALL_TYPES))
  const [showSuggest, setShowSuggest] = useState(false)
  // Defer the graph render until ResizeObserver reports a real width.
  // Initialising to a hard-coded value caused the canvas to render at
  // the wrong size on first paint; null + a measured update is the fix.
  const [containerWidth, setContainerWidth] = useState<number | null>(null)

  // Callback ref instead of useRef + useEffect (PSY-519). Same fix that
  // landed on SceneGraph.tsx (PSY-516/PSY-517): on first render this
  // component returns `null` while TanStack Query is loading, so a ref-
  // based useEffect with `[]` deps fires once with a null ref, bails, and
  // never re-runs after the JSX mounts. A callback ref fires whenever the
  // underlying DOM node mounts/unmounts, so we always measure the right
  // node. Cleanup return is honored automatically (React 19).
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

  // PSY-361: re-center state. The trail is the breadcrumb of *prior*
  // centers (not including the current). Capped at MAX_TRAIL_SLOTS=3
  // by pushTrail. The slug→id map is populated from each click event so
  // we can re-center without an extra resolution round-trip; on reload
  // (URL has ?center=<slug> but we have no click history), we fall back
  // to the useArtist hook below.
  const [trail, setTrail] = useState<TraversalEntry[]>([])
  const [slugToIdCache, setSlugToIdCache] = useState<Record<string, number>>({})
  const [announcement, setAnnouncement] = useState('')

  if (isLoading) return null

  const hasRelationships = originalGraph && (originalGraph.nodes.length > 0 || originalGraph.links.length > 0)

  const autoOpenFromHash = hash === GRAPH_HASH && Boolean(hasRelationships)
  const showGraph = showGraphOverride ?? autoOpenFromHash

  // Empty state: show header + message + suggest button for authenticated users
  if (!hasRelationships) {
    return (
      <div ref={containerRefCallback} className="mt-8 px-4 md:px-0">
        <h2 className="text-lg font-semibold mb-4">Related Artists</h2>
        <p className="text-sm text-muted-foreground">
          No similar artists yet. Be the first to suggest one!
        </p>
        {isAuthenticated && (
          <div className="mt-4">
            {showSuggest ? (
              <SuggestSimilarArtist
                centerArtistId={artistId}
                centerArtistSlug={artistSlug}
                onClose={() => setShowSuggest(false)}
              />
            ) : (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setShowSuggest(true)}
                className="text-muted-foreground"
              >
                <Plus className="h-4 w-4 mr-1.5" />
                Suggest similar artist
              </Button>
            )}
          </div>
        )}
      </div>
    )
  }

  const toggleType = (type: string) => {
    setActiveTypes(prev => {
      const next = new Set(prev)
      if (next.has(type)) {
        next.delete(type)
      } else {
        next.add(type)
      }
      return next
    })
  }

  // Group links by related artist for the list view
  const artistLinks = new Map<number, { links: ArtistGraphLink[]; node: typeof originalGraph.nodes[0] }>()
  for (const node of originalGraph.nodes) {
    artistLinks.set(node.id, { links: [], node })
  }
  for (const link of originalGraph.links) {
    const otherId =
      link.source_id === originalGraph.center.id ? link.target_id :
      link.target_id === originalGraph.center.id ? link.source_id : null
    if (otherId && artistLinks.has(otherId)) {
      artistLinks.get(otherId)!.links.push(link)
    }
  }

  // Sort by combined score
  const sortedArtists = Array.from(artistLinks.values())
    .filter(a => a.links.length > 0)
    .sort((a, b) => {
      const aScore = Math.max(...a.links.map(l => l.score))
      const bScore = Math.max(...b.links.map(l => l.score))
      return bScore - aScore
    })

  // PSY-366: dropped the `nodes.length >= 3` gate to fix entry-point
  // invisibility per `docs/research/knowledge-graph-viz-prior-art.md` §5.4
  // — the button is the affordance, and a sparse graph (1–2 related artists)
  // is still informative. The `!hasRelationships` early return above handles
  // the truly-zero case.
  //
  // Mobile gating retained: below the Tailwind `sm` breakpoint (640px) the
  // graph is unusable on a phone (PSY-369), so the button is hidden and the
  // list view is the only surface. `containerWidth === null` (pre-measurement)
  // also gates off so we never flash the button before measuring the viewport.
  const graphAvailable = containerWidth !== null && containerWidth >= 640

  return (
    <div ref={containerRefCallback} id="graph" className="mt-8 px-4 md:px-0 scroll-mt-20">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold">Related Artists</h2>
        <div className="flex items-center gap-2">
          {graphAvailable && (
            <Button
              variant={showGraph ? 'default' : 'outline'}
              size="sm"
              onClick={() => setShowGraphOverride(!showGraph)}
            >
              <Network className="h-4 w-4 mr-1.5" />
              {showGraph ? 'Hide graph' : 'Explore graph'}
            </Button>
          )}
        </div>
      </div>

      {/* Graph View */}
      {showGraph && graphAvailable && (
        <RecenteringGraph
          originalArtistId={artistId}
          originalArtistSlug={artistSlug}
          originalArtistName={originalGraph.center.name}
          containerWidth={containerWidth!}
          activeTypes={activeTypes}
          onToggleType={toggleType}
          trail={trail}
          setTrail={setTrail}
          slugToIdCache={slugToIdCache}
          setSlugToIdCache={setSlugToIdCache}
          announcement={announcement}
          setAnnouncement={setAnnouncement}
        />
      )}

      {/* List View */}
      <div className="space-y-2">
        {sortedArtists.map(({ node, links }) => (
          <RelatedArtistRow
            key={node.id}
            node={node}
            links={links}
            centerArtistId={artistId}
            centerArtistSlug={artistSlug}
            isAuthenticated={isAuthenticated}
            userVotes={originalGraph.user_votes}
          />
        ))}
      </div>

      {/* Suggest Similar Artist */}
      {isAuthenticated && (
        <div className="mt-4">
          {showSuggest ? (
            <SuggestSimilarArtist
              centerArtistId={artistId}
              centerArtistSlug={artistSlug}
              onClose={() => setShowSuggest(false)}
            />
          ) : (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowSuggest(true)}
              className="text-muted-foreground"
            >
              <Plus className="h-4 w-4 mr-1.5" />
              Suggest similar artist
            </Button>
          )}
        </div>
      )}
    </div>
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

  // Fetch the graph for the current center. Same hook, just dynamic ID.
  // The 5-min staleTime in the hook + TanStack Query's keyed cache means
  // we don't refetch when the user clicks back to a previously-visited
  // center — the graph data is served from cache, the transition feels
  // instant.
  const { data: graph, isFetching: fetchingGraph } = useArtistGraph({
    artistId: centerId,
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

  // PSY-361: announce each re-center to assistive tech. Keyed on the
  // graph's actual center.id (not the URL slug) so the announcement
  // fires only after the new payload has actually rendered, and we
  // never announce a stale center while a fetch is in flight.
  useEffect(() => {
    if (!graph) return
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

  // Filter type toggles — only render filters for types that actually
  // appear in the *current* graph's payload (not the original's).
  const linkTypesPresent = useMemo(() => {
    if (!graph) return new Set<string>()
    return new Set(graph.links.map(l => l.type))
  }, [graph])

  // Show the previous frame while the new center is loading — never show
  // an empty container or unmount the canvas. If we don't have a graph at
  // all yet (the first fetch is in flight), fall back to a simple loader.
  // Once we have *any* graph data, even stale, we keep rendering it under
  // the recentering overlay.
  if (!graph) {
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
          // Only show toggle if this type exists in the current graph.
          if (!linkTypesPresent.has(type)) return null
          return (
            <button
              key={type}
              onClick={() => onToggleType(type)}
              className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full border transition-opacity ${
                badge.className
              } ${isActive ? 'opacity-100' : 'opacity-40'}`}
            >
              <span className={`w-1.5 h-1.5 rounded-full ${isActive ? 'bg-current' : 'bg-transparent border border-current'}`} />
              {badge.label}
            </button>
          )
        })}
      </div>

      <ArtistGraphVisualization
        data={graph}
        activeTypes={activeTypes}
        containerWidth={containerWidth}
        onRecenter={handleRecenter}
        isRecentering={isRecentering}
      />

      {/* PSY-361: aria-live region for re-center announcements. The
          `sr-only` class is the codebase's standard visually-hidden
          utility (Tailwind built-in). Polite politeness so the
          announcement doesn't interrupt other assistive-tech speech;
          atomic so the entire string is read on each update. */}
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
        className="flex-1 min-w-0 flex items-center gap-2"
      >
        <span className="text-sm font-medium truncate group-hover:text-foreground">
          {node.name}
        </span>
      </Link>

      {/* Relationship badges */}
      <div className="hidden sm:flex items-center gap-1 flex-shrink-0">
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

      {/* Score */}
      <span className="text-xs text-muted-foreground flex-shrink-0 hidden md:block">
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
            ? 'text-green-400 bg-green-900/20'
            : 'text-red-400 bg-red-900/20'
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
        <p className="mt-2 text-xs text-green-400">Relationship created with your upvote!</p>
      )}
    </div>
  )
}
