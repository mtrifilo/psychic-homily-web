'use client'

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import dynamic from 'next/dynamic'
import Link from 'next/link'
import * as Sentry from '@sentry/nextjs'
import type { GeoLocation } from '@/lib/geo-default'
import { useScenes, useSceneDetail } from '../hooks'
import { useVenues } from '@/features/venues/hooks'
import type { VenueWithShowCount } from '@/features/venues/types'
import type { SceneListItem } from '../types'
import {
  isPlaceableScene,
  type CameraSettle,
  type GlobePov,
  type PlaceableScene,
  type VenuePin,
} from './globeTypes'
import {
  CITY_RAIL_WIDTH_PX,
  CITY_VENUE_FETCH_LIMIT,
  CITY_VIEW_MIN_VIEWPORT_PX,
  NO_CITY_VENUE_FILTERS,
  filterCityVenues,
  formatNextShowDate,
  resolveCityScene,
  venuePinPosition,
  type CityVenueFilters,
} from '../cityView'
import { VenueRail } from './VenueRail'
import { pickDriftScene } from './drift'
import { AtlasSearch } from './AtlasSearch'
import { GenreLegend } from './GenreLegend'
import { MyScenesStrip, MY_SCENES_FETCH_LIMIT } from './MyScenesStrip'
import { useMyFollowing } from '@/lib/hooks/common/useFollow'
import { ScenePreviewPanel } from './ScenePreviewPanel'
import { MobileSceneList } from './MobileSceneList'

const GLOBE_BREAKPOINT_PX = 640
// North America centroid — the default focus before/without visitor geo, so the
// first paint shows the populated cluster rather than empty ocean (PSY-1211).
const DEFAULT_POV: GlobePov = { lat: 39.5, lng: -98.35, altitude: 1.8 }
// Cap how long the globe waits for IP-geo before opening on the default focus.
const GEO_TIMEOUT_MS = 700
// Stable empty reference so an undefined scenes response doesn't churn memo deps.
const EMPTY_SCENES: SceneListItem[] = []
// Same, for the city-view venue list — an undefined response must not churn the
// pin/filter memos on every render (the identity-churn class of bug PSY-1538
// shipped a HIGH for).
const EMPTY_VENUES: VenueWithShowCount[] = []

function GlobeSkeleton() {
  return <div className="h-full w-full animate-pulse bg-muted/10" aria-hidden="true" />
}

// next/dynamic re-invokes `loading` with `error`/`retry` set on a failed chunk
// fetch (it does NOT throw to an error boundary) — without this branch a rotated
// hashed chunk would strand the user on the aria-hidden skeleton forever. Same
// pattern + rationale as InlineGraph's GraphLoadError.
function GlobeLoadError({ onRetry }: { onRetry?: () => void }) {
  return (
    <div
      role="alert"
      className="flex h-full w-full flex-col items-center justify-center gap-3 p-6 text-center text-sm text-muted-foreground"
    >
      <p>The globe couldn’t load.</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="text-primary underline-offset-4 hover:underline"
        >
          Try again
        </button>
      )}
    </div>
  )
}

// maplibre-gl is heavy (~900 kB chunk) and window-bound — dynamic-import the
// canvas with ssr:false so the chunk loads only here, on mount (PSY-1211
// pattern, isolation re-verified for MapLibre in the PSY-1537 spike).
const GlobeCanvas = dynamic(() => import('./GlobeCanvas'), {
  ssr: false,
  loading: ({ error, retry }) =>
    error ? <GlobeLoadError onRetry={retry} /> : <GlobeSkeleton />,
})

/**
 * Explore: The Globe (PSY-1213). A spin-to-discover globe where each city scene
 * is a dot; clicking one opens a preview with a link into the scene page.
 * Centered on the visitor's IP-geo region, falling back to North America.
 * Gated to a list below 640px (canvas gestures aren't usable there).
 */
export function AtlasGlobe() {
  const { data, isLoading, isError } = useScenes()
  const allScenes = data?.scenes ?? EMPTY_SCENES

  // Followed scenes (PSY-1340): tint their dots + star the mobile rows. The
  // hook is auth-gated, so logged-out visitors cost no request. Memoized to a
  // Set so the GlobeCanvas color accessor's identity only changes when the
  // follow list actually does.
  const { data: myScenes } = useMyFollowing({
    type: 'scene',
    limit: MY_SCENES_FETCH_LIMIT, // same key as MyScenesStrip → one request
  })
  const followedSlugs = useMemo(() => {
    const follows = myScenes?.following ?? []
    return follows.length > 0 ? new Set(follows.map((f) => f.slug)) : null
  }, [myScenes])
  // Memoize so the scenes array reference is stable until the data actually
  // changes — GlobeCanvas keys its GeoJSON rebuild and label-set memos on it,
  // so a churning reference would re-run them on every click/resize render.
  const placeable = useMemo<PlaceableScene[]>(
    () => allScenes.filter(isPlaceableScene),
    [allScenes],
  )
  const unplaceableCount = allScenes.length - placeable.length

  const [size, setSize] = useState<{ width: number; height: number } | null>(null)
  // null until the initial focus is resolved (visitor geo, or the default after
  // a short timeout). Resolved ONCE before the globe mounts, so the camera never
  // snaps post-mount over a user's in-progress rotation.
  const [pov, setPov] = useState<GlobePov | null>(null)
  const [selected, setSelected] = useState<PlaceableScene | null>(null)
  const closePreview = useCallback(() => setSelected(null), [])
  // Owned here (not in GenreLegend) so a user's collapse survives opening/closing a
  // scene preview, which unmounts the legend (PSY-1315 adversarial review).
  const [legendOpen, setLegendOpen] = useState(true)
  const toggleLegend = useCallback(() => setLegendOpen((o) => !o), [])

  // Imperative fly-the-camera seam GlobeCanvas fills on mount (PSY-1308) —
  // see the flyToRef prop doc for why this is a ref, not a forwarded ref.
  const flyToRef = useRef<((scene: PlaceableScene) => void) | null>(null)

  // ── City view (PSY-1539) ──────────────────────────────────────────────────
  // The camera decides which city (if any) owns the screen. GlobeCanvas
  // reports only on SETTLE, so this state changes a handful of times per
  // journey, not per frame.
  const [camera, setCamera] = useState<CameraSettle | null>(null)
  const handleCameraSettle = useCallback((next: CameraSettle) => {
    setCamera((prev) =>
      prev && prev.lng === next.lng && prev.lat === next.lat && prev.zoom === next.zoom
        ? prev
        : next,
    )
  }, [])

  const cityScene = useMemo(
    () => (camera ? resolveCityScene(placeable, camera) : null),
    [placeable, camera],
  )
  const citySlug = cityScene?.slug ?? null

  // Filters and the open-venue seam belong to ONE city. Reset them during
  // render when the camera hands off to another city — React's
  // adjust-state-during-render pattern, deliberately not a useEffect (a
  // prop-derived reset in an effect renders one frame of the previous city's
  // filters against the new city's venues).
  const [filters, setFilters] = useState<CityVenueFilters>(NO_CITY_VENUE_FILTERS)
  const [selectedVenueId, setSelectedVenueId] = useState<number | null>(null)
  const [filtersCitySlug, setFiltersCitySlug] = useState<string | null>(null)
  if (citySlug !== filtersCitySlug) {
    setFiltersCitySlug(citySlug)
    setFilters(NO_CITY_VENUE_FILTERS)
    setSelectedVenueId(null)
  }

  // City-scoped, never bounding-box scoped (explicitly deferred): the rail
  // lists the venues of the scene's principal city. A metro-member city's
  // venues (a Tempe room under the Phoenix scene) are therefore NOT listed —
  // the venues endpoint filters on the literal city, and widening that to a
  // metro's member cities is its own change.
  const { data: cityVenueData, isLoading: venuesLoading } = useVenues({
    city: cityScene?.city,
    state: cityScene?.state,
    limit: CITY_VENUE_FETCH_LIMIT,
  })
  const cityVenues = cityScene
    ? (cityVenueData?.venues ?? EMPTY_VENUES)
    : EMPTY_VENUES

  // "N local artists" is genuinely a scene-level stat (the metro roster), so
  // it comes from the scene detail endpoint rather than being derived from a
  // city-scoped venue list.
  const { data: sceneDetail } = useSceneDetail(citySlug ?? '')

  const filteredVenues = useMemo(
    () => filterCityVenues(cityVenues, filters),
    [cityVenues, filters],
  )

  // The pin view model. Built from the SAME filtered array the rail lists, so
  // map and rail cannot show different sets of venues.
  const venuePins = useMemo<VenuePin[]>(() => {
    const pins: VenuePin[] = []
    for (const venue of filteredVenues) {
      const position = venuePinPosition(venue)
      if (!position) continue // no coordinates at all — lists, doesn't pin
      const nextDate = formatNextShowDate(venue.next_show_date)
      pins.push({
        id: venue.id,
        name: venue.name,
        lng: position.lng,
        lat: position.lat,
        upcomingShowCount: venue.upcoming_show_count,
        nextShowLabel: nextDate ? `next ${nextDate}` : '',
        precision: position.precision,
      })
    }
    return pins
  }, [filteredVenues])

  // The venue panel seam. PSY-1540 owns what opens; here selecting a venue
  // marks it in both the rail and the map so the two stay legibly in sync.
  const handleVenueSelect = useCallback((venueId: number) => {
    setSelectedVenueId((prev) => (prev === venueId ? null : venueId))
  }, [])

  // "← globe": fly back out to the globe's landing altitude over the city you
  // were in, which drops the camera below the city-view threshold and hands
  // the screen back to the scene dots.
  const handleBackToGlobe = useCallback(() => {
    if (cityScene) flyToRef.current?.(cityScene)
  }, [cityScene])

  // PSY-1313: the search trigger doubles as the preview panel's focus-return
  // target — it's the page's keyboard entry point into scenes, so closing the
  // panel lands a keyboard user back where the journey starts.
  const searchTriggerRef = useRef<HTMLButtonElement | null>(null)

  // Drift (PSY-1308): fly to a weighted-random scene and open its preview —
  // the radio.garden "balloon ride". The panel opens immediately so the
  // Bandcamp embed loads during the flight; the pick excludes the scene
  // already open so repeat drifts always land somewhere new.
  const handleDrift = useCallback(() => {
    // Plain read + set (NOT a functional updater): the pick is random and the
    // fly is a side effect — inside an updater StrictMode's double-invoke
    // would pick twice and fly twice.
    const next = pickDriftScene(placeable, selected?.slug)
    if (!next) return
    flyToRef.current?.(next)
    setSelected(next)
  }, [placeable, selected])

  // Search pick (PSY-1310): same fly + open as Drift, but for a scene the user
  // asked for by name.
  const handleSearchPick = useCallback((scene: PlaceableScene) => {
    flyToRef.current?.(scene)
    setSelected(scene)
  }, [])

  const measureRef = useCallback((node: HTMLDivElement | null) => {
    if (!node) return
    const rect = node.getBoundingClientRect()
    setSize({ width: rect.width, height: rect.height })
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setSize({ width: entry.contentRect.width, height: entry.contentRect.height })
      }
    })
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  // Resolve the initial focus once: the visitor's IP-geo region (PSY-946
  // plumbing, shared GeoLocation contract) if it carries coords, else North
  // America — whichever lands first, capped by GEO_TIMEOUT_MS so a slow or
  // edge-headerless geo route never blocks the globe.
  useEffect(() => {
    let settled = false
    const resolve = (p: GlobePov) => {
      if (!settled) {
        settled = true
        // Identity-preserving on purpose: under Cache Components this effect
        // SETUP re-runs on every show (the `settled` guard is a closure
        // local), and GlobeCanvas keys its map-creation effect on pov — a
        // fresh object per show would tear down and rebuild the map ~0–700ms
        // after every nav-back. First resolution wins forever.
        setPov((prev) => prev ?? p)
      }
    }
    const timer = setTimeout(() => resolve(DEFAULT_POV), GEO_TIMEOUT_MS)
    fetch('/api/geo')
      .then((res) =>
        res.ok ? (res.json() as Promise<{ geo: GeoLocation | null }>) : null,
      )
      .then((body) => {
        const lat = body?.geo?.latitude
        const lng = body?.geo?.longitude
        resolve(
          typeof lat === 'number' && typeof lng === 'number'
            ? { lat, lng, altitude: 1.6 }
            : DEFAULT_POV,
        )
      })
      .catch((error) => {
        // Non-fatal: open on the default focus, but surface a broken edge route.
        Sentry.captureException(error, {
          level: 'warning',
          tags: { service: 'atlas-geo' },
        })
        resolve(DEFAULT_POV)
      })
    return () => {
      settled = true
      clearTimeout(timer)
    }
  }, [])

  // A scene preview must not survive an error→recovery cycle: the error branch
  // unmounts the globe, and a retained selection would pop the old panel back
  // open on its own when the query recovers. Defer the clear to a microtask so
  // it lands after the effect returns (react-hooks/set-state-in-effect), the
  // same pattern useGeoDefaultCity uses.
  useEffect(() => {
    if (!isError) return
    let cancelled = false
    Promise.resolve().then(() => {
      if (!cancelled) setSelected(null)
    })
    return () => {
      cancelled = true
    }
  }, [isError])

  const isMobile = size !== null && size.width < GLOBE_BREAKPOINT_PX

  let content: ReactNode
  if (isError) {
    content = (
      <CenterMessage>The atlas couldn’t load. Try again shortly.</CenterMessage>
    )
  } else if (isMobile) {
    content = (
      <MobileSceneList
        scenes={allScenes}
        loading={isLoading}
        followedSlugs={followedSlugs}
      />
    )
  } else if (size !== null && placeable.length > 0 && pov !== null) {
    // The rail sits BESIDE the map (never over it — the map's bottom-left
    // attribution is a licensing requirement), so opening it narrows the
    // canvas by exactly the rail's width. Below CITY_VIEW_MIN_VIEWPORT_PX
    // that would leave a uselessly thin map, so city view stays map-only:
    // pins and status chip, no rail.
    const railOpen =
      cityScene !== null && size.width >= CITY_VIEW_MIN_VIEWPORT_PX
    const canvasWidth = railOpen ? size.width - CITY_RAIL_WIDTH_PX : size.width
    // The globe's own chrome is a globe-scale toolkit — Drift lands you in
    // another metro, the genre key explains dot tints that aren't drawn at
    // street zoom. City view replaces it with the rail.
    const globeChromeVisible = cityScene === null

    content = (
      <div className="flex h-full w-full">
        {railOpen && cityScene && (
          <VenueRail
            cityLabel={`${cityScene.city}, ${cityScene.state}`}
            venues={filteredVenues}
            allVenues={cityVenues}
            localArtistCount={sceneDetail?.stats.artist_count}
            loading={venuesLoading}
            filters={filters}
            onFiltersChange={setFilters}
            selectedVenueId={selectedVenueId}
            onVenueSelect={handleVenueSelect}
            onBackToGlobe={handleBackToGlobe}
          />
        )}
        <div className="relative min-w-0 flex-1">
          <GlobeCanvas
            width={canvasWidth}
            height={size.height}
            scenes={placeable}
            pov={pov}
            onSelect={setSelected}
            selected={selected}
            flyToRef={flyToRef}
            followedSlugs={followedSlugs}
            venues={venuePins}
            selectedVenueId={selectedVenueId}
            onVenueSelect={handleVenueSelect}
            cityLabel={
              cityScene ? `${cityScene.city}, ${cityScene.state}` : null
            }
            onCameraSettle={handleCameraSettle}
          />
          {globeChromeVisible && (
            <>
              <button
                type="button"
                onClick={handleDrift}
                aria-label="Drift to a random scene"
                className="absolute bottom-4 left-1/2 z-10 -translate-x-1/2 rounded-full border border-border bg-background/90 px-4 py-2 text-sm font-medium text-foreground backdrop-blur transition-colors hover:border-primary hover:text-primary"
              >
                Drift
              </button>
              <AtlasSearch
                scenes={allScenes}
                onPick={handleSearchPick}
                triggerRef={searchTriggerRef}
              />
              <MyScenesStrip scenes={allScenes} onPick={handleSearchPick} />
              {/* Genre color key (PSY-1315). Hidden while a preview is open — that
                  docks the right edge, and you're reading one scene, not scanning. */}
              {!selected && (
                <GenreLegend open={legendOpen} onToggle={toggleLegend} />
              )}
              {unplaceableCount > 0 && (
                <Link
                  href="/scenes"
                  /* bottom-11, not bottom-4: the map's attribution control (PSY-1543,
                     a license requirement) is docked bottom-left, and this link must
                     clear its ~30px strip rather than sit on the OSM credit. */
                  className="absolute bottom-11 left-4 z-10 rounded border border-border bg-background/90 px-3 py-1.5 text-xs text-muted-foreground underline-offset-4 hover:underline"
                >
                  {unplaceableCount} more{' '}
                  {unplaceableCount === 1 ? 'scene' : 'scenes'} not on the map ·
                  View all →
                </Link>
              )}
              {selected && (
                <ScenePreviewPanel
                  scene={selected}
                  onClose={closePreview}
                  returnFocusTo={searchTriggerRef}
                />
              )}
            </>
          )}
        </div>
      </div>
    )
  } else if (size !== null && !isLoading && placeable.length === 0) {
    content = (
      <CenterMessage>
        No scenes to place on the map yet.{' '}
        <Link
          href="/scenes"
          className="text-primary underline-offset-4 hover:underline"
        >
          Browse scenes →
        </Link>
      </CenterMessage>
    )
  } else {
    content = <GlobeSkeleton />
  }

  return (
    <div className="relative h-[calc(100dvh-4rem)] min-h-[480px] w-full overflow-hidden bg-[#0a0a0a]">
      <div ref={measureRef} className="h-full w-full">
        {content}
      </div>
    </div>
  )
}

function CenterMessage({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-full w-full items-center justify-center p-6 text-center text-sm text-muted-foreground">
      {children}
    </div>
  )
}
