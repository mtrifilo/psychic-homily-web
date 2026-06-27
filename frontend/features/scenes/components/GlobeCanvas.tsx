'use client'

import { useCallback, useMemo, useRef, useState } from 'react'
import Globe, { type GlobeMethods } from 'react-globe.gl'
import type { GlobePov, PlaceableScene } from './globeTypes'
import {
  labelMinCountForAltitude,
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
}

const EARTH_TEXTURE =
  'https://unpkg.com/three-globe/example/img/earth-night.jpg'

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
}: GlobeCanvasProps) {
  const globeRef = useRef<GlobeMethods | undefined>(undefined)

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

  // `pov` is resolved once in AtlasGlobe BEFORE this canvas mounts, so the
  // camera is aimed exactly once via onGlobeReady — no post-mount re-aim that
  // could snap over a user's in-progress rotation.
  return (
    <Globe
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
      pointAltitude={0.008}
      pointColor={() => '#ff7a3c'}
      pointRadius={(d) => sceneDotRadius((d as PlaceableScene).upcoming_show_count)}
      pointResolution={18}
      pointLabel={(d) => {
        const s = d as PlaceableScene
        return `${escapeHtml(s.city)}, ${escapeHtml(s.state)} · ${s.upcoming_show_count} upcoming`
      }}
      onPointClick={(d) => onSelect(d as PlaceableScene)}
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
  )
}
