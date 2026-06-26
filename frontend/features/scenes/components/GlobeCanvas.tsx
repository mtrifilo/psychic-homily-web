'use client'

import { useRef } from 'react'
import Globe, { type GlobeMethods } from 'react-globe.gl'
import type { GlobePov, PlaceableScene } from './globeTypes'

interface GlobeCanvasProps {
  width: number
  height: number
  scenes: PlaceableScene[]
  /** Camera focus; re-applied whenever it changes (e.g. visitor geo arrives). */
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
 * sqrt scale so dense scenes don't dwarf small ones; clicking one selects it.
 */
export default function GlobeCanvas({
  width,
  height,
  scenes,
  pov,
  onSelect,
}: GlobeCanvasProps) {
  const globeRef = useRef<GlobeMethods | undefined>(undefined)

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
      pointRadius={(d) =>
        0.28 + Math.sqrt((d as PlaceableScene).upcoming_show_count) / 14
      }
      pointResolution={18}
      pointLabel={(d) => {
        const s = d as PlaceableScene
        return `${escapeHtml(s.city)}, ${escapeHtml(s.state)} · ${s.upcoming_show_count} upcoming`
      }}
      onPointClick={(d) => onSelect(d as PlaceableScene)}
      labelsData={scenes}
      labelLat="latitude"
      labelLng="longitude"
      labelText={(d) => (d as PlaceableScene).city}
      labelSize={(d) =>
        0.5 + Math.sqrt((d as PlaceableScene).upcoming_show_count) / 32
      }
      labelDotRadius={0.18}
      labelColor={() => '#ffe6c2'}
      labelResolution={2}
      onGlobeReady={() => globeRef.current?.pointOfView(pov, 0)}
    />
  )
}
