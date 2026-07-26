'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
// maplibre-gl v6 has NO default export — `import maplibregl from 'maplibre-gl'`
// is `undefined` and fails confusingly (PSY-1537 spike). Namespace import only.
import * as maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { useGraphPalette } from '@/components/graph/graphPalette'
import type { GlobePov, PlaceableScene } from './globeTypes'
import { genreFamilyColor } from '../genreFamilies'
import {
  DOT_COLOR_SELECTED,
  DOT_HOVER_RADIUS_SCALE,
  altitudeForZoom,
  labelMinCountForAltitude,
  sceneDotColor,
  sceneDotRadiusPx,
  sceneLabelSizePx,
  visibleLabelScenes,
  zoomForAltitude,
} from './globeScale'

// PSY-1537 SPIKE FINDING (load-bearing): Turbopack rewrites maplibre's
// `import.meta.url` to a file://…node_modules… URL, so v6's runtime worker
// resolution returns "" and the map hangs FOREVER with no error — the raster
// earth renders but GeoJSON sources never parse and `idle` never fires. The
// fix is the vendored worker + shared modules in public/maplibre/ (pinned
// byte-identical by maplibreVendored.test.ts), pointed at BEFORE any Map is
// constructed: the worker pool is a module-global singleton, so a late
// setWorkerUrl is ignored by maps that already spun the pool.
if (typeof window !== 'undefined') {
  maplibregl.setWorkerUrl('/maplibre/maplibre-gl-worker.mjs')
}

interface GlobeCanvasProps {
  width: number
  height: number
  scenes: PlaceableScene[]
  /**
   * Camera focus. AtlasGlobe resolves this ONCE (the visitor-geo/default race
   * settles behind a guard) before mounting this canvas, and it's stable for
   * the component's lifetime — the map is aimed at it exactly once, at
   * construction (unless a saved camera from a previous show wins; see
   * savedCamera below).
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
   * Imperative fly-the-camera seam (PSY-1308 Drift; reused by scene search).
   * A plain ref rather than forwardRef because ref-forwarding through
   * next/dynamic is unreliable (PSY-1211) — GlobeCanvas fills it with a
   * function that reads the live map ref LAZILY, so it stays valid across a
   * hide/show remount (fresh map instance under the same ref).
   */
  flyToRef?: React.MutableRefObject<((scene: PlaceableScene) => void) | null>
  /** Slugs of scenes the viewer follows (PSY-1340) — tinted DOT_COLOR_FOLLOWED. */
  followedSlugs?: ReadonlySet<string> | null
}

// NASA GIBS Black Marble (VIIRS 2016 composite) — the night-earth raster the
// PSY-1537 spike verified. Note the {z}/{y}/{x} order (WMTS row-before-column)
// and .png. Public-domain NASA imagery; the host is allowlisted in the CSP
// connect-src (MapLibre fetches tiles via fetch(), not <img>).
const NIGHT_EARTH_TILES =
  'https://gibs.earthdata.nasa.gov/wmts/epsg3857/best/VIIRS_Black_Marble/default/2016-01-01/GoogleMapsCompatible_Level8/{z}/{y}/{x}.png'

// Camera altitude a fly-to lands at (legacy globe-altitude units — see
// zoomForAltitude) — closer than the initial continental POV (1.6–1.8) so
// arriving somewhere reads as a descent, but high enough that neighbouring
// scenes stay in frame.
const FLY_TO_ALTITUDE = 1.0
const FLY_TO_MS = 1200

// The GIBS tile set tops out at level 8; past ~9 the overzoomed raster is
// mush. PSY-1539 (street-level basemap) raises this ceiling deliberately.
const MAX_ZOOM = 9
const MIN_ZOOM = 1

// "Happening this week" pulse ring (PSY-1309 parity): a propagating stroked
// circle under each scene with a show in the next 7 days. Same period and
// fade curve as the shipped globe; radius converted from the old 1.6
// globe-degrees (~30 px at the default POV) to CSS px.
const RING_PERIOD_MS = 2600
const RING_MAX_RADIUS_PX = 30
const RING_MAX_OPACITY = 0.55
const RING_COLOR = '#ff7a3c'

// Atmosphere glow (PSY-1284-era visual: #4aa3ff at altitude 0.18). MapLibre's
// sky/atmosphere is thinner than the three.js halo, so a CSS box-shadow halo
// sized to the globe's screen radius supplements it (see updateHalo).
const HALO_COLOR = 'rgba(74, 163, 255, 0.32)'

const EMPTY_FC: GeoJSON.FeatureCollection = {
  type: 'FeatureCollection',
  features: [],
}

// Camera saved across hide/show cycles (module scope survives Cache
// Components' hide, which tears the map down via the cleanup below). This is
// deliberately a DATA cache, not an init guard: the map is still created
// fresh on every show — the one pattern PSY-1284 proved fatal was a guard ref
// that survives hide and skips re-init. Without this, nav-away/back would
// reset the camera to the initial POV (the map instance is new each show).
let savedCamera: { center: [number, number]; zoom: number } | null = null

// Deterministic starfield background (data-URI SVG, module scope — client-only
// module, so no hydration concern). Mulberry32 keeps it stable across builds.
function buildStarfieldDataUri(): string {
  let seed = 0x51ce
  const rand = () => {
    seed = (seed + 0x6d2b79f5) | 0
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
  const stars: string[] = []
  for (let i = 0; i < 110; i++) {
    const x = (rand() * 1600).toFixed(0)
    const y = (rand() * 1000).toFixed(0)
    const r = (0.4 + rand() * 0.9).toFixed(2)
    const o = (0.15 + rand() * 0.55).toFixed(2)
    stars.push(`<circle cx="${x}" cy="${y}" r="${r}" fill="#cfe2ff" opacity="${o}"/>`)
  }
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="1000">${stars.join('')}</svg>`
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`
}
const STARFIELD_BG = buildStarfieldDataUri()

// The globe's screen radius in CSS px at a zoom: the equator maps to
// worldSize = 512·2^zoom px of circumference.
function globeScreenRadiusPx(zoom: number): number {
  return (512 * 2 ** zoom) / (2 * Math.PI)
}

/**
 * The MapLibre globe canvas (PSY-1538), isolated in its own client module so
 * AtlasGlobe can dynamic-import it with `ssr:false`: maplibre-gl is ~900 kB
 * and window-bound, so it must never load on the server or in any other
 * route's initial JS (bundle isolation verified in the PSY-1537 spike).
 *
 * Lifecycle is a PLAIN create-on-setup / remove-on-cleanup effect. Under
 * Cache Components, nav-away hides the page (cleanup runs, map removed) and
 * nav-back re-runs setup (fresh map). Do NOT add an init-guard ref that
 * survives hide — a surviving `useEffectOnce`-style guard is exactly what
 * froze react-globe.gl (PSY-1284); the spike verified 20 nav cycles clean
 * with this plain pattern, which is why the old key-bump heal is gone.
 *
 * Dots are city-aggregated (one per scene), sized by upcoming-show count with
 * a capped sqrt scale; labels are zoom-gated + proximity-decluttered DOM
 * markers (globeScale.ts owns all calibration, translated from the legacy
 * altitude bands via altitudeForZoom).
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
  const containerRef = useRef<HTMLDivElement | null>(null)
  const tooltipRef = useRef<HTMLDivElement | null>(null)
  const haloRef = useRef<HTMLDivElement | null>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  // The style-loaded map instance, in STATE so the data/label/ring effects
  // below re-run against each fresh map after a hide/show cycle.
  const [mapReady, setMapReady] = useState<maplibregl.Map | null>(null)

  // Hover affordance (PSY-1312): state (not a ref) so the dot-color memo below
  // re-evaluates. mousemove only calls setHoveredSlug on enter/leave, not
  // per-move, so this doesn't churn renders.
  const [hoveredSlug, setHoveredSlug] = useState<string | null>(null)
  const selectedSlug = selected?.slug ?? null

  // Resolved theme palette for the dominant-genre dot tint (PSY-1315).
  const palette = useGraphPalette()

  // Latest onSelect without threading it into the map-creation effect's deps
  // (handlers read it at call time).
  const onSelectRef = useRef(onSelect)
  useEffect(() => {
    onSelectRef.current = onSelect
  }, [onSelect])

  // PSY-1223 zoom-gated labels: the discrete threshold, derived from zoom via
  // the altitude translation so the calibrated bands + declutter map are
  // reused verbatim. Seeded from the camera the map will actually open on
  // (saved camera from a previous show, else the resolved POV).
  const [labelMinCount, setLabelMinCount] = useState(() =>
    labelMinCountForAltitude(
      altitudeForZoom(savedCamera?.zoom ?? zoomForAltitude(pov.altitude)),
    ),
  )

  const labelScenes = useMemo(
    () => visibleLabelScenes(scenes, labelMinCount),
    [scenes, labelMinCount],
  )

  // Scenes by slug for click-event lookup (feature properties only carry the slug).
  const scenesBySlug = useMemo(
    () => new Map(scenes.map((s) => [s.slug, s])),
    [scenes],
  )
  // Live lookup for the map handlers (they're bound once per map instance).
  const scenesBySlugRef = useRef(scenesBySlug)
  useEffect(() => {
    scenesBySlugRef.current = scenesBySlug
  }, [scenesBySlug])

  // Dot layer data. Color precedence (selected > hovered > followed > genre
  // tint > base) and the capped sqrt radius live in globeScale; this just
  // bakes them into feature properties. One dot per scene, so a setData on
  // hover/selection change is cheap.
  const sceneFeatures = useMemo<GeoJSON.FeatureCollection>(
    () => ({
      type: 'FeatureCollection',
      features: scenes.map((s) => {
        const genreBase = genreFamilyColor(palette, s.dominant_genre)
        const base = sceneDotRadiusPx(s.upcoming_show_count)
        return {
          type: 'Feature',
          properties: {
            slug: s.slug,
            color: sceneDotColor(
              s.slug,
              hoveredSlug,
              selectedSlug,
              followedSlugs,
              genreBase,
            ),
            radiusPx: s.slug === hoveredSlug ? base * DOT_HOVER_RADIUS_SCALE : base,
            // Smaller dots draw ABOVE larger ones so a dense metro can't
            // swallow its neighbour — the PSY-1324 altitude-stacking
            // semantics, ported to circle-sort-key (higher key = on top).
            sortKey: Number.isFinite(s.upcoming_show_count)
              ? -s.upcoming_show_count
              : 0,
            count: s.upcoming_show_count,
          },
          geometry: {
            type: 'Point',
            coordinates: [s.longitude, s.latitude],
          },
        }
      }),
    }),
    [scenes, hoveredSlug, selectedSlug, followedSlugs, palette],
  )

  useEffect(() => {
    const src = mapReady?.getSource('scenes') as
      | maplibregl.GeoJSONSource
      | undefined
    src?.setData(sceneFeatures)
  }, [mapReady, sceneFeatures])

  // PSY-1309 pulse rings. prefers-reduced-motion suppresses the animation
  // entirely (no ring features, no rAF loop) rather than freezing a ring frame.
  const pulseScenes = useMemo(() => {
    if (
      typeof window !== 'undefined' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
    ) {
      return []
    }
    return scenes.filter((s) => s.shows_this_week > 0)
  }, [scenes])

  useEffect(() => {
    if (!mapReady || pulseScenes.length === 0) return
    const src = mapReady.getSource('scene-rings') as
      | maplibregl.GeoJSONSource
      | undefined
    if (!src) return
    src.setData({
      type: 'FeatureCollection',
      features: pulseScenes.map((s) => ({
        type: 'Feature',
        properties: {},
        geometry: { type: 'Point', coordinates: [s.longitude, s.latitude] },
      })),
    })
    // Per-frame paint updates keep the map perpetually rendering — same cost
    // profile as the shipped globe's ring shader. NOTE for any harness: gate
    // readiness on the FIRST `idle`, because this loop means idle never
    // settles afterwards.
    let raf = requestAnimationFrame(function tick(now: number) {
      const t = (now % RING_PERIOD_MS) / RING_PERIOD_MS
      mapReady.setPaintProperty('scene-rings', 'circle-radius', RING_MAX_RADIUS_PX * t)
      mapReady.setPaintProperty(
        'scene-rings',
        'circle-stroke-opacity',
        RING_MAX_OPACITY * (1 - t),
      )
      raf = requestAnimationFrame(tick)
    })
    return () => {
      cancelAnimationFrame(raf)
      src.setData(EMPTY_FC)
    }
  }, [mapReady, pulseScenes])

  // Zoom-gated + decluttered labels as DOM markers: crisp text in the app
  // font with no glyph-server dependency, and MapLibre's globe pipeline
  // fades markers occluded behind the horizon. pointer-events: none so labels
  // never swallow globe drags or dot clicks.
  useEffect(() => {
    if (!mapReady) return
    const markers = labelScenes.map((s) => {
      const el = document.createElement('div')
      el.textContent = s.city
      el.style.cssText = [
        'pointer-events: none',
        'user-select: none',
        'white-space: nowrap',
        `color: ${DOT_COLOR_SELECTED}`,
        'font-weight: 500',
        'letter-spacing: 0.01em',
        'text-shadow: 0 1px 4px rgba(0,0,0,0.9)',
      ].join(';')
      el.style.fontSize = `${sceneLabelSizePx(s.upcoming_show_count).toFixed(1)}px`
      return new maplibregl.Marker({
        element: el,
        anchor: 'top',
        offset: [0, Math.ceil(sceneDotRadiusPx(s.upcoming_show_count)) + 3],
      })
        .setLngLat([s.longitude, s.latitude])
        .addTo(mapReady)
    })
    return () => {
      for (const m of markers) m.remove()
    }
  }, [mapReady, labelScenes])

  // Fill the parent's fly-to seam (PSY-1308). Reads mapRef at call time —
  // never captured — so it aims whichever map instance is live. MapLibre
  // honors prefers-reduced-motion natively (respectPrefersReducedMotion
  // defaults true), degrading the flight to a jump cut.
  useEffect(() => {
    if (!flyToRef) return
    flyToRef.current = (scene: PlaceableScene) => {
      mapRef.current?.flyTo({
        center: [scene.longitude, scene.latitude],
        zoom: zoomForAltitude(FLY_TO_ALTITUDE),
        duration: FLY_TO_MS,
      })
    }
    return () => {
      flyToRef.current = null
    }
  }, [flyToRef])

  // ── Map lifecycle ─────────────────────────────────────────────────────────
  // Declared LAST on purpose: React destroys effects in declaration order, so
  // on unmount/hide the marker + rAF cleanups above run BEFORE map.remove().
  //
  // Plain create/remove — no init guard, no key-bump heal (see component doc).
  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const center: [number, number] = savedCamera?.center ?? [pov.lng, pov.lat]
    const zoom = savedCamera?.zoom ?? zoomForAltitude(pov.altitude)

    const map = new maplibregl.Map({
      container,
      center,
      zoom,
      minZoom: MIN_ZOOM,
      maxZoom: MAX_ZOOM,
      attributionControl: false,
      style: {
        version: 8,
        projection: { type: 'globe' },
        // Atmosphere: MapLibre's own halo, faded out as the globe fills the
        // viewport. The CSS halo (updateHalo) thickens it toward the shipped
        // three.js glow — screenshot comparison in the PR for sign-off.
        sky: {
          'sky-color': 'rgba(0,0,0,0)',
          'horizon-color': 'rgba(74,163,255,0.45)',
          'fog-color': 'rgba(10,16,32,0.6)',
          'atmosphere-blend': [
            'interpolate',
            ['linear'],
            ['zoom'],
            0,
            1,
            5,
            0.6,
            7,
            0,
          ],
        },
        // No background layer: space stays transparent so the CSS starfield
        // and halo behind the canvas show through.
        sources: {
          nightEarth: {
            type: 'raster',
            tiles: [NIGHT_EARTH_TILES],
            tileSize: 256,
            maxzoom: 8,
            attribution: 'Imagery courtesy NASA GIBS (VIIRS Black Marble)',
          },
          scenes: { type: 'geojson', data: EMPTY_FC },
          'scene-rings': { type: 'geojson', data: EMPTY_FC },
        },
        layers: [
          { id: 'earth', type: 'raster', source: 'nightEarth' },
          {
            // Under the dots so a ring never covers its own scene's dot —
            // the RING_ALTITUDE invariant of the shipped globe, by layer order.
            id: 'scene-rings',
            type: 'circle',
            source: 'scene-rings',
            paint: {
              'circle-radius': 0,
              'circle-opacity': 0,
              'circle-stroke-color': RING_COLOR,
              'circle-stroke-width': 1.5,
              'circle-stroke-opacity': 0,
            },
          },
          {
            id: 'scene-dots',
            type: 'circle',
            source: 'scenes',
            layout: {
              'circle-sort-key': ['get', 'sortKey'],
            },
            paint: {
              'circle-radius': ['get', 'radiusPx'],
              'circle-color': ['get', 'color'],
              'circle-stroke-width': 1,
              'circle-stroke-color': 'rgba(255,230,194,0.35)',
            },
          },
        ],
      },
    })
    mapRef.current = map

    // Verification seam for the browser-automation aliveness harness
    // (getCenter drag assertions + first-idle readiness gate). Deliberately
    // tiny and stateless — remove-safe.
    const w = window as unknown as {
      __atlasMap?: maplibregl.Map | null
      __atlasMapIdle?: boolean
    }
    w.__atlasMap = map
    w.__atlasMapIdle = false
    map.once('idle', () => {
      w.__atlasMapIdle = true
    })

    map.on('load', () => {
      setMapReady(map)
    })

    // CSS halo sized to the globe's screen radius (it grows past the viewport
    // and out of sight as the earth fills the frame).
    const updateHalo = () => {
      const halo = haloRef.current
      if (!halo) return
      const d = 2 * globeScreenRadiusPx(map.getZoom())
      halo.style.width = `${d}px`
      halo.style.height = `${d}px`
    }
    updateHalo()

    // Zoom drives the discrete label threshold; only threshold CROSSINGS
    // change state (and thus label markers) — micro-zooms are free.
    const handleZoom = () => {
      const next = labelMinCountForAltitude(altitudeForZoom(map.getZoom()))
      setLabelMinCount((prev) => (prev === next ? prev : next))
      updateHalo()
    }
    map.on('zoom', handleZoom)

    // Hover: pointer cursor + tooltip + dot highlight. Tooltip position/text
    // are written imperatively so mousemove never re-renders React.
    const pickTopScene = (
      features: maplibregl.MapGeoJSONFeature[] | undefined,
    ): string | null => {
      if (!features || features.length === 0) return null
      // Smallest count wins a stacked hit — matches the sort-key draw order
      // (the smaller dot is the one visibly on top).
      let best: maplibregl.MapGeoJSONFeature = features[0]
      for (const f of features) {
        const c = Number(f.properties?.count)
        const bc = Number(best.properties?.count)
        if (Number.isFinite(c) && (!Number.isFinite(bc) || c < bc)) best = f
      }
      return typeof best.properties?.slug === 'string'
        ? best.properties.slug
        : null
    }

    const handleMove = (
      e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] },
    ) => {
      const slug = pickTopScene(e.features)
      setHoveredSlug((prev) => (prev === slug ? prev : slug))
      map.getCanvas().style.cursor = slug ? 'pointer' : ''
      const tooltip = tooltipRef.current
      if (!tooltip) return
      const scene = slug ? scenesBySlugRef.current.get(slug) : undefined
      if (!scene) {
        tooltip.style.display = 'none'
        return
      }
      const week =
        scene.shows_this_week > 0 ? ` · ${scene.shows_this_week} this week` : ''
      // textContent, not innerHTML — contributor-editable city/state can't
      // inject markup (the old canvas needed an escapeHtml for this).
      tooltip.textContent = `${scene.city}, ${scene.state} · ${scene.upcoming_show_count} upcoming${week}`
      tooltip.style.display = 'block'
      tooltip.style.left = `${e.point.x + 12}px`
      tooltip.style.top = `${e.point.y + 12}px`
    }
    const handleLeave = () => {
      setHoveredSlug(null)
      map.getCanvas().style.cursor = ''
      if (tooltipRef.current) tooltipRef.current.style.display = 'none'
    }
    const handleClick = (
      e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] },
    ) => {
      const slug = pickTopScene(e.features)
      const scene = slug ? scenesBySlugRef.current.get(slug) : undefined
      if (scene) onSelectRef.current(scene)
    }
    map.on('mousemove', 'scene-dots', handleMove)
    map.on('mouseleave', 'scene-dots', handleLeave)
    map.on('click', 'scene-dots', handleClick)

    return () => {
      // Save the camera so nav-back reopens where the user left off — the
      // map itself is NOT reused (fresh instance every show; see doc above).
      savedCamera = {
        center: [map.getCenter().lng, map.getCenter().lat],
        zoom: map.getZoom(),
      }
      if (w.__atlasMap === map) {
        w.__atlasMap = null
        w.__atlasMapIdle = false
      }
      mapRef.current = null
      setMapReady((prev) => (prev === map ? null : prev))
      map.remove()
    }
    // pov is resolved once before this canvas mounts and stable for its
    // lifetime (see the prop doc); everything else this effect reads is a ref
    // or setter, so the camera never re-aims on data re-renders.
  }, [pov])

  return (
    <div
      style={{ width, height }}
      className="relative overflow-hidden"
      data-testid="globe-cursor-wrap"
    >
      {/* Space backdrop: starfield + atmosphere halo behind the transparent
          map canvas (the earth sphere itself is opaque and covers them). */}
      <div
        aria-hidden="true"
        className="absolute inset-0"
        style={{ backgroundImage: STARFIELD_BG }}
      />
      <div
        ref={haloRef}
        aria-hidden="true"
        className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 rounded-full"
        style={{ boxShadow: `0 0 90px 28px ${HALO_COLOR}` }}
      />
      <div ref={containerRef} className="absolute inset-0" />
      <div
        ref={tooltipRef}
        className="pointer-events-none absolute z-10 rounded border border-border bg-background/90 px-2 py-1 text-xs text-foreground backdrop-blur"
        style={{ display: 'none' }}
      />
    </div>
  )
}
