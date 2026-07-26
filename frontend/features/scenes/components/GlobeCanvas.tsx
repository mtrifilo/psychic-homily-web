'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
// maplibre-gl v6 has NO default export — `import maplibregl from 'maplibre-gl'`
// is `undefined` and fails confusingly (PSY-1537 spike). Namespace import only.
import * as maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { useGraphPalette } from '@/components/graph/graphPalette'
import { PH_BASEMAP_MIN_ZOOM, phBasemapFragment } from '../basemap/phBasemap'
import type { GlobePov, PlaceableScene } from './globeTypes'
import { genreFamilyColor } from '../genreFamilies'
import {
  DOT_COLOR_HOVERED,
  DOT_COLOR_SELECTED,
  DOT_HOVER_RADIUS_SCALE,
  globeScreenRadiusPx,
  labelMinCountForAltitude,
  labelMinCountForZoom,
  sceneDotColor,
  sceneDotRadiusPx,
  sceneDotSortKey,
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
   * next/dynamic is unreliable (PSY-1211). Filled inside the map-creation
   * effect (closes over that map instance) and NULLED in its cleanup, so it
   * is empty while the page is hidden or the map is torn down — callers must
   * stay null-safe (AtlasGlobe's `flyToRef.current?.(…)` calls are).
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

// Globe → street handoff (PSY-1543): the Black Marble raster fades OUT and
// the PH basemap's background fades IN across this zoom range. Bounds chosen
// so the crossfade (a) starts only after the sphere more than fills a
// desktop viewport (screen radius ≈ 512·2^z/2π px ⇒ ~1300 px at z4, so no
// street style ever peeks past the sphere edge), (b) finishes exactly where
// the sky's atmosphere-blend reaches 0 (z7 — one coordinated "descent"
// moment), and (c) is fully done well before the GIBS raster's native z8
// ceiling turns overzoomed tiles to mush. The basemap's road/boundary layers
// switch on at PH_BASEMAP_MIN_ZOOM — half a zoom BEFORE the fade starts — so
// city-lights pixels dissolve into streets already drawn beneath them: no
// black void, no pop. Retuning the handoff means editing these two constants
// and nothing else; the raster ramp, the raster cutoff and the atmosphere
// ramp are all derived from them.
const BLACK_MARBLE_FADE_START = 5.5
const BLACK_MARBLE_FADE_END = 7

// Street-zoom ceiling (PSY-1543). The OpenFreeMap vector source is maxzoom 14
// and overzooms cleanly; 17 gives block-level framing for the PSY-1539 venue
// pins without letting the camera dive into an empty ground plane. (The GIBS
// raster no longer binds this — its layer is capped at the fade end.)
const MAX_ZOOM = 17
const MIN_ZOOM = 1

// "Happening this week" pulse ring (PSY-1309 parity): a propagating stroked
// circle under each scene with a show in the next 7 days. Same period and
// fade curve as the shipped globe; radius converted from the old 1.6
// globe-degrees (~30 px at the default POV) to CSS px.
const RING_PERIOD_MS = 2600
const RING_MAX_RADIUS_PX = 30
const RING_MAX_OPACITY = 0.55
const RING_COLOR = '#ff7a3c'

// Atmosphere glow (matches the shipped globe's #4aa3ff three.js halo).
// MapLibre's sky/atmosphere renders thinner and sun-angled, so a two-layer
// CSS box-shadow halo sized to the globe's screen radius supplements it:
// a tight rim plus a wide falloff, approximating the old uniform glow.
const HALO_SHADOW =
  '0 0 90px 24px rgba(74, 163, 255, 0.42), 0 0 220px 80px rgba(74, 163, 255, 0.18)'

const EMPTY_FC: GeoJSON.FeatureCollection = {
  type: 'FeatureCollection',
  features: [],
}

// Camera saved across map teardowns. Module scope (not a ref) on purpose: it
// survives not only Cache Components' hide but also REAL unmounts of this
// component — e.g. the <640px mobile-gate flip, which unmounts the canvas
// entirely. Single-instance surface (one Atlas globe per app), so shared
// module state is safe. This is deliberately a DATA cache, not an init
// guard: the map is still created fresh on every show — the one pattern
// PSY-1284 proved fatal was a guard ref that survives hide and skips
// re-init. Without this, nav-away/back would reset the camera to the
// initial POV (the map instance is new each show).
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
 * altitude bands via labelMinCountForZoom).
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
  // The style-loaded map instance, in STATE so the data/label/ring effects
  // below re-run against each fresh map after a hide/show cycle.
  const [mapReady, setMapReady] = useState<maplibregl.Map | null>(null)

  const selectedSlug = selected?.slug ?? null

  // Resolved theme palette for the dominant-genre dot tint (PSY-1315).
  const palette = useGraphPalette()

  // Latest onSelect without threading it into the map-creation effect's deps
  // (handlers read it at call time).
  const onSelectRef = useRef(onSelect)
  useEffect(() => {
    onSelectRef.current = onSelect
  }, [onSelect])

  // PSY-1223 zoom-gated labels: the discrete threshold, from the calibrated
  // bands (globeScale owns the zoom translation). Seeded from the camera the
  // map will actually open on (saved camera from a previous show, else the
  // resolved POV — already in the altitude units the bands were tuned in).
  const [labelMinCount, setLabelMinCount] = useState(() =>
    savedCamera
      ? labelMinCountForZoom(savedCamera.zoom)
      : labelMinCountForAltitude(pov.altitude),
  )

  const labelScenes = useMemo(
    () => visibleLabelScenes(scenes, labelMinCount),
    [scenes, labelMinCount],
  )

  // Live slug → scene lookup for the map handlers (bound once per map
  // instance; feature properties only carry the slug).
  const scenesBySlugRef = useRef<ReadonlyMap<string, PlaceableScene>>(new Map())
  useEffect(() => {
    scenesBySlugRef.current = new Map(scenes.map((s) => [s.slug, s]))
  }, [scenes])

  // Dot layer data. Color precedence (selected > hovered > followed > genre
  // tint > base) and the capped sqrt radius live in globeScale; this bakes
  // the SLOW-CHANGING states into feature properties. Hover is deliberately
  // NOT baked here: it rides feature-state (see the paint expressions), so a
  // hover enter/leave never rebuilds the source or re-renders React —
  // setData round-trips the whole collection through the worker.
  const sceneFeatures = useMemo<GeoJSON.FeatureCollection>(
    () => ({
      type: 'FeatureCollection',
      features: scenes.map((s) => {
        const genreBase = genreFamilyColor(palette, s.dominant_genre)
        return {
          type: 'Feature',
          properties: {
            slug: s.slug,
            color: sceneDotColor(s.slug, null, selectedSlug, followedSlugs, genreBase),
            radiusPx: sceneDotRadiusPx(s.upcoming_show_count),
            // The hover color must not override the selected cream — the
            // paint expression checks this flag (selected > hovered).
            isSelected: s.slug === selectedSlug,
            // Smaller dots draw ABOVE larger ones so a dense metro can't
            // swallow its neighbour — PSY-1324 stacking as circle-sort-key
            // (globeScale owns the rule; hit-testing compares the same key).
            sortKey: sceneDotSortKey(s.upcoming_show_count),
          },
          geometry: {
            type: 'Point',
            coordinates: [s.longitude, s.latitude],
          },
        }
      }),
    }),
    [scenes, selectedSlug, followedSlugs, palette],
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
    // validate:false — these are trusted constants; skip per-frame style
    // validation on the animation hot path.
    let raf = requestAnimationFrame(function tick(now: number) {
      const t = (now % RING_PERIOD_MS) / RING_PERIOD_MS
      mapReady.setPaintProperty(
        'scene-rings',
        'circle-radius',
        RING_MAX_RADIUS_PX * t,
        { validate: false },
      )
      mapReady.setPaintProperty(
        'scene-rings',
        'circle-stroke-opacity',
        RING_MAX_OPACITY * (1 - t),
        { validate: false },
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
        // Fully hide labels occluded behind the globe — MapLibre's default
        // fades them to 0.2 opacity, which ghosts far-side city names over
        // the sphere; the old depth-tested 3D labels were fully hidden.
        opacityWhenCovered: '0',
      })
        .setLngLat([s.longitude, s.latitude])
        .addTo(mapReady)
    })
    return () => {
      for (const m of markers) m.remove()
    }
  }, [mapReady, labelScenes])

  // ── Map lifecycle ─────────────────────────────────────────────────────────
  // Declared LAST on purpose: React destroys effects in declaration order, so
  // on unmount/hide the marker + rAF cleanups above run BEFORE map.remove().
  //
  // INVARIANT: this effect's deps must be identity-stable for the component's
  // lifetime (pov — first-resolution-wins in AtlasGlobe; flyToRef — a stable
  // ref container). A deps-change re-run would remove the map while the
  // sibling effects above still hold it until the next commit (the ordering
  // guarantee only covers unmount/hide, where all cleanups run together).
  //
  // Plain create/remove — no init guard, no key-bump heal (see component doc).
  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const center: [number, number] = savedCamera?.center ?? [pov.lng, pov.lat]
    const zoom = savedCamera?.zoom ?? zoomForAltitude(pov.altitude)

    // PH street basemap (PSY-1543): OpenFreeMap vector tiles restyled to the
    // app's dark tokens, with its background ramped in across the Black
    // Marble fade range (see phBasemapFragment for why the background must
    // ramp rather than sit opaque).
    const basemap = phBasemapFragment(
      BLACK_MARBLE_FADE_START,
      BLACK_MARBLE_FADE_END,
    )

    const map = new maplibregl.Map({
      container,
      center,
      zoom,
      minZoom: MIN_ZOOM,
      maxZoom: MAX_ZOOM,
      attributionControl: false,
      // Parity with the shipped globe: spin + zoom only. MapLibre's default
      // pitch/rotate gestures would let a user tilt the horizon with NO
      // reset affordance (no compass control), detaching the CSS halo from
      // the sphere outline; double-click zoom would fire an unrequested
      // flight on a double-clicked dot. Rotation is further disabled on the
      // touch + keyboard handlers below.
      dragRotate: false,
      pitchWithRotate: false,
      touchPitch: false,
      doubleClickZoom: false,
      style: {
        version: 8,
        projection: { type: 'globe' },
        // Glyph server for the basemap's street/place labels (the scene
        // labels stay DOM markers in the app font — no glyph dependency).
        glyphs: basemap.glyphs,
        // Atmosphere: MapLibre's own halo, faded out as the globe fills the
        // viewport. The CSS halo (updateHalo) thickens it toward the shipped
        // three.js glow — screenshot comparison in the PR for sign-off.
        //
        // The atmosphere reaches 0 exactly at BLACK_MARBLE_FADE_END, on
        // purpose: sky, raster and street basemap all resolve on the same
        // zoom so the descent reads as ONE moment. Derived from the constant
        // rather than repeating the literal, so retuning the handoff can't
        // silently leave the atmosphere lingering over the street view.
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
            PH_BASEMAP_MIN_ZOOM,
            0.6,
            BLACK_MARBLE_FADE_END,
            0,
          ],
        },
        // The basemap's background layer is opacity-ramped (0 until the
        // street fade), so space stays transparent at globe zooms and the
        // CSS starfield and halo behind the canvas show through.
        sources: {
          ...basemap.sources,
          nightEarth: {
            type: 'raster',
            tiles: [NIGHT_EARTH_TILES],
            tileSize: 256,
            maxzoom: 8,
            // Rendered by the AttributionControl below (PSY-1543), alongside
            // the OpenFreeMap/OSM credit the openmaptiles source carries.
            // NASA imagery is public domain and GIBS attribution is
            // requested rather than required, but showing it costs nothing
            // once the control exists for the OSM requirement.
            attribution: 'Imagery courtesy NASA GIBS (VIIRS Black Marble)',
          },
          // promoteId: features are keyed by slug so the hover feature-state
          // (set in handleMove below) sticks across setData refreshes.
          scenes: { type: 'geojson', data: EMPTY_FC, promoteId: 'slug' },
          'scene-rings': { type: 'geojson', data: EMPTY_FC },
        },
        layers: [
          // Street basemap under the raster: at globe zooms the opaque Black
          // Marble covers it (and its layers are minzoom-gated anyway); as
          // the raster fades out across the handoff range the streets are
          // already drawn beneath — no black frame between the two worlds.
          ...basemap.layers,
          {
            id: 'earth',
            type: 'raster',
            source: 'nightEarth',
            // Both halves of the crossfade come from phBasemapFragment, so
            // this ramp is the background ramp's mirror BY CONSTRUCTION —
            // retuning the handoff means editing the two constants above and
            // nothing else. The maxzoom stops GIBS fetching/compositing once
            // the raster is provably invisible.
            maxzoom: basemap.rasterMaxZoom,
            paint: { 'raster-opacity': basemap.rasterFadeOut },
          },
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
              // Zero out MapLibre's default 300ms paint transitions: the rAF
              // loop drives these two properties as a sawtooth, and a
              // retargeted ease would smear it — the ring would collapse
              // inward at each cycle wrap instead of restarting at center.
              'circle-radius-transition': { duration: 0, delay: 0 },
              'circle-stroke-opacity-transition': { duration: 0, delay: 0 },
            },
          },
          {
            id: 'scene-dots',
            type: 'circle',
            source: 'scenes',
            layout: {
              'circle-sort-key': ['get', 'sortKey'],
            },
            // Hover rides feature-state so enter/leave never rebuilds the
            // source (see the sceneFeatures doc). Precedence: the selected
            // cream must not be overridden by hover (selected > hovered —
            // sceneDotColor's contract), hence the isSelected guard; the
            // radius bump applies regardless, matching the old canvas.
            paint: {
              'circle-radius': [
                '*',
                ['get', 'radiusPx'],
                [
                  'case',
                  ['boolean', ['feature-state', 'hover'], false],
                  DOT_HOVER_RADIUS_SCALE,
                  1,
                ],
              ],
              'circle-color': [
                'case',
                [
                  'all',
                  ['boolean', ['feature-state', 'hover'], false],
                  ['!', ['get', 'isSelected']],
                ],
                DOT_COLOR_HOVERED,
                ['get', 'color'],
              ],
              'circle-stroke-width': 1,
              'circle-stroke-color': 'rgba(255,230,194,0.35)',
            },
          },
        ],
      },
    })
    // See the constructor options: bearing/pitch must stay locked at 0 on
    // every input path (savedCamera deliberately persists only center/zoom).
    map.touchZoomRotate.disableRotation()
    map.keyboard.disableRotation()

    // Attribution (PSY-1543): the OpenStreetMap credit is a license
    // requirement (ODbL) now that street tiles ship, so the old chrome-free
    // look gains an always-visible control (non-compact — OSM's guidance
    // frowns on hidden-behind-an-icon attribution on desktop). Credit
    // strings come from the sources above (OpenFreeMap/OSM + NASA GIBS);
    // the dark restyle of MapLibre's default white pill lives in
    // globals.css (.maplibregl-ctrl-attrib).
    //
    // BOTTOM-LEFT, not MapLibre's bottom-right default: the Atlas chrome
    // owns bottom-right twice over — GenreLegend sits there at z-10 (the
    // control's own stacking context tops out at z-index 2, so the legend
    // wins), and ScenePreviewPanel docks the entire right edge full-height
    // whenever a scene is selected, which would hide the required credit
    // outright. Bottom-left is the one corner nothing else docks to; the
    // "N more scenes" link that shares it is offset above the strip in
    // AtlasGlobe.
    map.addControl(
      new maplibregl.AttributionControl({ compact: false }),
      'bottom-left',
    )

    // Fill the parent's fly-to seam (PSY-1308, reused by search/Drift).
    // Closes over THIS map; nulled in cleanup, so after a hide/show cycle
    // the seam always points at the live instance. MapLibre honors
    // prefers-reduced-motion natively — flyTo degrades to a jump cut unless
    // the animation is marked `essential` (verified in the v6 bundle).
    if (flyToRef) {
      flyToRef.current = (scene: PlaceableScene) => {
        map.flyTo({
          center: [scene.longitude, scene.latitude],
          zoom: zoomForAltitude(FLY_TO_ALTITUDE),
          duration: FLY_TO_MS,
        })
      }
    }

    // Verification seam for the browser-automation aliveness harness
    // (getCenter drag assertions + readiness gate). Gated on 'load', NOT
    // 'idle': the pulse-ring rAF loop changes paint properties every frame,
    // so the map never idles while rings are animating. Deliberately tiny
    // and stateless — remove-safe.
    const w = window as unknown as {
      __atlasMap?: maplibregl.Map | null
      __atlasMapLoaded?: boolean
    }
    w.__atlasMap = map
    w.__atlasMapLoaded = false

    // Dots/labels/rings mount on 'style.load' (style parsed, sources
    // registered), NOT 'load': 'load' additionally waits for every in-view
    // GIBS raster tile, and a hanging third-party tile response would hold
    // the page's actual content — locally-available scene data — hostage to
    // the basemap. The harness flag stays on 'load' (full first render).
    map.on('style.load', () => {
      setMapReady(map)
    })
    map.on('load', () => {
      w.__atlasMapLoaded = true
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
      const next = labelMinCountForZoom(map.getZoom())
      setLabelMinCount((prev) => (prev === next ? prev : next))
      updateHalo()
    }
    map.on('zoom', handleZoom)

    // Hover: pointer cursor + tooltip + dot highlight, all imperative —
    // mousemove never re-renders React. The highlight itself is a
    // feature-state flag consumed by the paint expressions above.
    const pickTopScene = (
      features: maplibregl.MapGeoJSONFeature[] | undefined,
    ): string | null => {
      if (!features || features.length === 0) return null
      // Highest sort key wins a stacked hit — the SAME key that decides draw
      // order (globeScale.sceneDotSortKey), so the dot you see on top is the
      // dot the pointer selects.
      let best: maplibregl.MapGeoJSONFeature = features[0]
      for (const f of features) {
        if (Number(f.properties?.sortKey) > Number(best.properties?.sortKey)) {
          best = f
        }
      }
      return typeof best.properties?.slug === 'string'
        ? best.properties.slug
        : null
    }

    // Tracked per map instance (fresh map each show → no stale hover).
    let hoveredSlug: string | null = null
    const setHoverState = (slug: string | null) => {
      if (slug === hoveredSlug) return
      if (hoveredSlug !== null) {
        map.removeFeatureState({ source: 'scenes', id: hoveredSlug }, 'hover')
      }
      if (slug !== null) {
        map.setFeatureState({ source: 'scenes', id: slug }, { hover: true })
      }
      hoveredSlug = slug
    }

    const handleMove = (
      e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] },
    ) => {
      const slug = pickTopScene(e.features)
      setHoverState(slug)
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
      // Flip the offset near the right/bottom edges so edge-of-viewport
      // scenes don't get their tooltip clipped by the overflow-hidden wrap.
      const bounds = map.getContainer().getBoundingClientRect()
      const flipX = e.point.x + 12 + tooltip.offsetWidth > bounds.width
      const flipY = e.point.y + 12 + tooltip.offsetHeight > bounds.height
      tooltip.style.left = `${flipX ? e.point.x - 12 - tooltip.offsetWidth : e.point.x + 12}px`
      tooltip.style.top = `${flipY ? e.point.y - 12 - tooltip.offsetHeight : e.point.y + 12}px`
    }
    const handleLeave = () => {
      setHoverState(null)
      map.getCanvas().style.cursor = ''
      if (tooltipRef.current) tooltipRef.current.style.display = 'none'
    }
    // Camera motion under a STATIONARY pointer (wheel zoom, keyboard zoom,
    // fly-to) slides dots out from under the cursor without any mouse event,
    // which would strand a stale highlight + floating tooltip. Any camera
    // movement clears the hover; the next real mousemove re-establishes it.
    map.on('movestart', handleLeave)
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
        w.__atlasMapLoaded = false
      }
      if (flyToRef) flyToRef.current = null
      setMapReady((prev) => (prev === map ? null : prev))
      map.remove()
    }
    // pov is resolved once before this canvas mounts, and flyToRef is a
    // stable ref container from AtlasGlobe — both are identity-stable for
    // the canvas's lifetime; everything else this effect reads is a ref or
    // setter, so the camera never re-aims on data re-renders.
  }, [pov, flyToRef])

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
        style={{ boxShadow: HALO_SHADOW }}
      />
      {/* Inline position/inset, NOT Tailwind classes: maplibre-gl.css sets
          `.maplibregl-map { position: relative }` on this node at map init,
          which ties with the `absolute` utility class and (depending on
          stylesheet order) collapses the container to 0 height — the canvas
          then falls back to its 300px default. Inline style always wins. */}
      <div ref={containerRef} style={{ position: 'absolute', inset: 0 }} />
      <div
        ref={tooltipRef}
        className="pointer-events-none absolute z-10 rounded border border-border bg-background/90 px-2 py-1 text-xs text-foreground backdrop-blur"
        style={{ display: 'none' }}
      />
    </div>
  )
}
