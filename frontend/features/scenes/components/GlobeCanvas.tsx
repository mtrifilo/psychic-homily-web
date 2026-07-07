'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Globe, { type GlobeMethods } from 'react-globe.gl'
import { useGraphPalette } from '@/components/graph/graphPalette'
import type { GlobePov, PlaceableScene } from './globeTypes'
import { genreFamilyColor } from '../genreFamilies'
import {
  DOT_HOVER_RADIUS_SCALE,
  labelMinCountForAltitude,
  RING_ALTITUDE,
  sceneDotAltitude,
  sceneDotColor,
  sceneDotRadius,
  sceneLabelSize,
  visibleLabelScenes,
} from './globeScale'

interface GlobeCanvasProps {
  width: number
  height: number
  scenes: PlaceableScene[]
  /**
   * Camera focus. AtlasGlobe resolves this ONCE (the visitor-geo/default race
   * settles behind a guard) before mounting this canvas, and it's stable for the
   * component's lifetime: the camera is aimed once via onGlobeReady, and the
   * PSY-1223 label-visibility threshold is seeded once from `pov.altitude`. If pov
   * is ever made dynamic, that seed must be re-synced (see the useState below).
   */
  pov: GlobePov
  onSelect: (scene: PlaceableScene) => void
  /**
   * The scene whose preview panel is open (PSY-1312): its dot stays visually
   * distinct so you can see which dot you're reading about. null when no panel
   * is open.
   */
  selected?: PlaceableScene | null
  /**
   * Imperative fly-the-camera seam (PSY-1308 Drift; reusable by scene search).
   * A plain ref rather than forwardRef because ref-forwarding through
   * next/dynamic is unreliable (PSY-1211) — GlobeCanvas fills it with a
   * function that reads the live GlobeMethods ref LAZILY, so it stays valid
   * across the PSY-1284 rebuild (which swaps the keyed <Globe> instance under
   * the same globeRef).
   */
  flyToRef?: React.MutableRefObject<((scene: PlaceableScene) => void) | null>
  /** Slugs of scenes the viewer follows (PSY-1340) — tinted DOT_COLOR_FOLLOWED. */
  followedSlugs?: ReadonlySet<string> | null
}

const EARTH_TEXTURE =
  'https://unpkg.com/three-globe/example/img/earth-night.jpg'

// Camera altitude a fly-to lands at — closer than the initial continental POV
// (1.6–1.8) so arriving somewhere reads as a descent, but high enough that
// neighbouring scenes stay in frame.
const FLY_TO_ALTITUDE = 1.0
const FLY_TO_MS = 1200

// PSY-1324 occlusion fix: less-dense dots get a slightly TALLER cylinder so an
// overlapped neighbor's top face always renders above the denser dot's (see
// sceneDotAltitude). Pure and hover-independent, so it lives at module scope —
// a stable identity means hover-state re-renders never rebuild the altitude
// accessor.
const pointAltitude = (d: object) =>
  sceneDotAltitude((d as PlaceableScene).upcoming_show_count)

// react-globe.gl's hover tooltip (pointLabel) is written to the DOM via
// innerHTML, so any markup in the contributor-editable city/state must be
// escaped to avoid a stored XSS in the tooltip. (The 3D labelText path renders
// as geometry, not HTML, so it's already safe.)
function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

/**
 * The react-globe.gl canvas, isolated in its own client module so AtlasGlobe can
 * dynamic-import it with `ssr:false`: react-globe.gl pulls in three.js (~470 kB
 * gz — PSY-1211 spike) and touches `window`, so it must never load on the server
 * or in the shared bundle. Holding the GlobeMethods ref HERE (rather than
 * threading it through next/dynamic) keeps ref-forwarding intact.
 *
 * Dots are city-aggregated (one per scene), sized by upcoming-show count with a
 * capped sqrt scale so dense scenes (Chicago ~283) read big without ballooning
 * over neighbours; clicking one selects it. Labels are zoom-gated so adjacent
 * dense cities (Minneapolis / St. Paul) don't overlap at the continental zoom
 * (PSY-1223 — see globeScale.ts).
 */
export default function GlobeCanvas({
  width,
  height,
  scenes,
  pov,
  onSelect,
  selected = null,
  flyToRef,
  followedSlugs = null,
}: GlobeCanvasProps) {
  const globeRef = useRef<GlobeMethods | undefined>(undefined)

  // PSY-1312: hover affordance. Tracking the hovered slug in STATE (not a ref)
  // is deliberate — the color/radius accessors below close over it, and the
  // state change gives them new identities, which is exactly what makes
  // react-globe.gl re-evaluate the points layer. The dot set is small (one per
  // scene), so the re-evaluation is cheap; onPointHover only fires on
  // enter/leave, not per-frame.
  const [hoveredSlug, setHoveredSlug] = useState<string | null>(null)
  const selectedSlug = selected?.slug ?? null

  // Resolved theme palette for the dominant-genre dot tint (PSY-1315). Stable
  // between theme changes (useGraphPalette memoizes on the <html> class), so it
  // doesn't churn pointColor per render; on a theme toggle it re-resolves and the
  // dots re-tint — the same one-per-theme-change cost as the graph surfaces.
  const palette = useGraphPalette()

  const handlePointHover = useCallback((point: object | null) => {
    setHoveredSlug(point ? (point as PlaceableScene).slug : null)
  }, [])

  const pointColor = useCallback(
    (d: object) => {
      const s = d as PlaceableScene
      // A scene's dominant genre family (when confident) tints its resting dot;
      // hover/select/follow still override it inside sceneDotColor.
      const genreBase = genreFamilyColor(palette, s.dominant_genre)
      return sceneDotColor(s.slug, hoveredSlug, selectedSlug, followedSlugs, genreBase)
    },
    [hoveredSlug, selectedSlug, followedSlugs, palette],
  )
  const pointRadius = useCallback(
    (d: object) => {
      const s = d as PlaceableScene
      const base = sceneDotRadius(s.upcoming_show_count)
      return s.slug === hoveredSlug ? base * DOT_HOVER_RADIUS_SCALE : base
    },
    [hoveredSlug],
  )

  // Fill the parent's fly-to seam (PSY-1308). The closure reads globeRef at
  // call time — never captured — so it aims whichever keyed <Globe> instance
  // is live, including after a PSY-1284 rebuild. Honors prefers-reduced-motion
  // with a jump cut instead of the 1.2s flight.
  useEffect(() => {
    if (!flyToRef) return
    flyToRef.current = (scene: PlaceableScene) => {
      const reduced =
        typeof window !== 'undefined' &&
        window.matchMedia('(prefers-reduced-motion: reduce)').matches
      globeRef.current?.pointOfView(
        {
          lat: scene.latitude,
          lng: scene.longitude,
          altitude: FLY_TO_ALTITUDE,
        },
        reduced ? 0 : FLY_TO_MS,
      )
    }
    return () => {
      flyToRef.current = null
    }
  }, [flyToRef])

  // PSY-1284: heal a globe that comes back frozen after a client-side
  // navigation away from /atlas and back.
  //
  // Next 16 Cache Components keeps the /atlas page's React tree (state + DOM)
  // alive across client navigation rather than unmounting it — Activity-style:
  // effect cleanups run on hide, effect setups re-run on show, and state/refs
  // survive. While the page is hidden, react-globe.gl's react-kapsule wrapper
  // runs the globe's destructor chain — pausing the render loop, disposing
  // OrbitControls + the WebGLRenderer, and (via three-render-objects'
  // `_destructor` → `emptyObject(scene)`) emptying the three.js scene — but on
  // show it does NOT re-run init (react-kapsule's `useEffectOnce` guard ref
  // survived the hide). The result is an inert, frozen globe: it shows the last
  // frame but can't rotate, zoom, or be clicked.
  //
  // The torn-down instance can't be revived through react-globe.gl's public API
  // (`resumeAnimation()` does not restart the loop, and the scene-graph teardown
  // is too deep to rebuild by hand — both were tried), so detect it and force a
  // brand-new <Globe> via a key change; a fresh init rebuilds the renderer,
  // controls, loop, and scene. Because the effect setup re-runs on every show,
  // this re-heals on every away→back cycle, not just the first.
  //
  // Detection signal: the emptied scene (`scene().children.length === 0`). This
  // depends on three-render-objects' `_destructor` zeroing `scene.children`,
  // pinned transitively via react-globe.gl 2.38. Fragility: if a future bump
  // stops emptying the scene on teardown, `children` stays non-empty on a
  // torn-down instance and this heal silently no-ops (PSY-1284 returns with no
  // error) — re-verify the away→back interactivity on any react-globe.gl / three
  // upgrade.
  const [rebuildNonce, setRebuildNonce] = useState(0)
  useEffect(() => {
    const globe = globeRef.current
    if (!globe) return
    let sceneEmptied = false
    try {
      sceneEmptied = globe.scene().children.length === 0
    } catch {
      sceneEmptied = false
    }
    if (!sceneEmptied) return
    // Release the dead instance's WebGL context before building a fresh one so
    // live contexts don't accumulate toward the browser's ~16 limit. Each keyed
    // <Globe> owns a SEPARATE context, so losing the stale one is safe — unlike
    // the shared-context teardown that an unconditional dispose-on-unmount would
    // have broken (see PSY-1284 notes).
    try {
      globe.renderer().forceContextLoss()
    } catch {
      // best effort — the context may already be lost
    }
    // Defer the remount to a microtask so forceContextLoss() finishes and the
    // setState lands after this effect returns rather than synchronously
    // (react-hooks/set-state-in-effect); the cancelled guard mirrors AtlasGlobe.
    let cancelled = false
    Promise.resolve().then(() => {
      if (!cancelled) setRebuildNonce((nonce) => nonce + 1)
    })
    return () => {
      cancelled = true
    }
  }, [])

  // PSY-1223: zoom-gated labels. Track the label-visibility threshold (derived
  // from camera altitude) in state, seeded from the initial pov. onZoom fires
  // continuously as the camera moves, so update only when the discrete threshold
  // actually changes — otherwise labelsData churns and react-globe.gl rebuilds
  // the label geometry every frame.
  const [labelMinCount, setLabelMinCount] = useState(() =>
    labelMinCountForAltitude(pov.altitude),
  )
  const handleZoom = useCallback((nextPov: GlobePov) => {
    const next = labelMinCountForAltitude(nextPov.altitude)
    setLabelMinCount((prev) => (prev === next ? prev : next))
  }, [])

  // Only scenes above the current threshold carry an always-on label; memoized
  // on the discrete threshold so the array identity is stable between zoom
  // crossings (react-globe.gl diffs labelsData by reference).
  const labelScenes = useMemo(
    () => visibleLabelScenes(scenes, labelMinCount),
    [scenes, labelMinCount],
  )

  // PSY-1309: "happening this week" pulse — a slow propagating ring under each
  // scene with a show in the next 7 days, so a live scene reads differently
  // from a merely-catalogued one. prefers-reduced-motion suppresses the
  // animation entirely (empty ringsData) rather than freezing a ring frame.
  // Memoized by reference for the same geometry-churn reason as labelsData.
  const pulseScenes = useMemo(() => {
    if (
      typeof window !== 'undefined' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
    ) {
      return []
    }
    return scenes.filter((s) => s.shows_this_week > 0)
  }, [scenes])

  // `pov` is resolved once in AtlasGlobe BEFORE this canvas mounts, so the
  // camera is aimed exactly once via onGlobeReady — no post-mount re-aim that
  // could snap over a user's in-progress rotation.
  //
  // The wrapping div exists only for the hover cursor (PSY-1312): react-globe.gl
  // renders into a canvas with no per-object cursor, so the pointer cursor is
  // applied to the container while a dot is hovered. Sized to the canvas, so it
  // is layout-neutral.
  return (
    <div
      style={{ width, height, cursor: hoveredSlug ? 'pointer' : undefined }}
      data-testid="globe-cursor-wrap"
    >
      <Globe
        key={rebuildNonce}
        ref={globeRef}
        width={width}
        height={height}
        globeImageUrl={EARTH_TEXTURE}
        backgroundColor="rgba(0,0,0,0)"
        showAtmosphere
        atmosphereColor="#4aa3ff"
        atmosphereAltitude={0.18}
        pointsData={scenes}
        pointLat="latitude"
        pointLng="longitude"
        pointAltitude={pointAltitude}
        pointColor={pointColor}
        pointRadius={pointRadius}
        pointResolution={18}
        pointLabel={(d) => {
          const s = d as PlaceableScene
          const week = s.shows_this_week > 0 ? ` · ${s.shows_this_week} this week` : ''
          return `${escapeHtml(s.city)}, ${escapeHtml(s.state)} · ${s.upcoming_show_count} upcoming${week}`
        }}
        onPointClick={(d) => onSelect(d as PlaceableScene)}
        onPointHover={handlePointHover}
        ringsData={pulseScenes}
        ringLat="latitude"
        ringLng="longitude"
        ringAltitude={RING_ALTITUDE}
        ringMaxRadius={1.6}
        ringPropagationSpeed={0.9}
        ringRepeatPeriod={2600}
        ringColor={() => (t: number) => `rgba(255, 122, 60, ${Math.max(0, 0.55 * (1 - t))})`}
        ringResolution={48}
        labelsData={labelScenes}
        labelLat="latitude"
        labelLng="longitude"
        labelText={(d) => (d as PlaceableScene).city}
        labelSize={(d) => sceneLabelSize((d as PlaceableScene).upcoming_show_count)}
        labelDotRadius={0.18}
        labelColor={() => '#ffe6c2'}
        labelResolution={2}
        labelsTransitionDuration={300}
        onZoom={handleZoom}
        onGlobeReady={() => globeRef.current?.pointOfView(pov, 0)}
      />
    </div>
  )
}
