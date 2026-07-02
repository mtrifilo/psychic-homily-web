'use client'

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import dynamic from 'next/dynamic'
import Link from 'next/link'
import * as Sentry from '@sentry/nextjs'
import type { GeoLocation } from '@/lib/geo-default'
import { useScenes } from '../hooks'
import type { SceneListItem } from '../types'
import {
  isPlaceableScene,
  type GlobePov,
  type PlaceableScene,
} from './globeTypes'
import { ScenePreviewPanel } from './ScenePreviewPanel'

const GLOBE_BREAKPOINT_PX = 640
// North America centroid — the default focus before/without visitor geo, so the
// first paint shows the populated cluster rather than empty ocean (PSY-1211).
const DEFAULT_POV: GlobePov = { lat: 39.5, lng: -98.35, altitude: 1.8 }
// Cap how long the globe waits for IP-geo before opening on the default focus.
const GEO_TIMEOUT_MS = 700
// Stable empty reference so an undefined scenes response doesn't churn memo deps.
const EMPTY_SCENES: SceneListItem[] = []

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

// react-globe.gl + three.js are heavy and window-bound — dynamic-import the
// canvas with ssr:false so the chunk loads only here, on mount (PSY-1211).
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
  // Memoize so the points/labels array reference is stable until the data
  // actually changes — react-globe.gl diffs pointsData by reference and would
  // otherwise rebuild the three.js geometry on every click/resize render.
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
        setPov(p)
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
    content = <MobileSceneList scenes={allScenes} loading={isLoading} />
  } else if (size !== null && placeable.length > 0 && pov !== null) {
    content = (
      <>
        <GlobeCanvas
          width={size.width}
          height={size.height}
          scenes={placeable}
          pov={pov}
          onSelect={setSelected}
          selected={selected}
        />
        {unplaceableCount > 0 && (
          <Link
            href="/scenes"
            className="absolute bottom-4 left-4 z-10 rounded border border-border bg-background/90 px-3 py-1.5 text-xs text-muted-foreground underline-offset-4 hover:underline"
          >
            {unplaceableCount} more {unplaceableCount === 1 ? 'scene' : 'scenes'}{' '}
            not on the map · View all →
          </Link>
        )}
        {selected && (
          <ScenePreviewPanel scene={selected} onClose={closePreview} />
        )}
      </>
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

// <640px: the WebGL globe + canvas gestures aren't usable (PSY-511/1086 gate),
// so serve the scenes as a simple list — still the geographic-discovery payoff,
// just not spatial. Lists ALL scenes (incl. ones the globe can't place).
function MobileSceneList({
  scenes,
  loading,
}: {
  scenes: SceneListItem[]
  loading: boolean
}) {
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
          {scenes.map((s) => (
            <li key={s.slug}>
              <Link
                href={`/scenes/${s.slug}`}
                className="flex items-center justify-between gap-3 py-3"
              >
                <span className="font-medium">
                  {s.city}, {s.state}
                </span>
                <span className="font-mono text-xs text-muted-foreground">
                  {s.upcoming_show_count} upcoming
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
