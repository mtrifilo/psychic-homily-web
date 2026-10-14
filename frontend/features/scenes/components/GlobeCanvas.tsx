'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
// maplibre-gl v6 has NO default export — `import maplibregl from 'maplibre-gl'`
// is `undefined` and fails confusingly (PSY-1537 spike). Namespace import only.
import * as maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
// Aims the worker pool at the vendored copy before any Map is constructed.
import './maplibreWorker'
import { markAtlasMapReady } from '@/lib/atlasMapReady'
import { useCoarsePointer } from '@/lib/hooks/common/useCoarsePointer'
import { useReducedMotion } from '@/lib/hooks/common/useReducedMotion'
import {
  handleBasemapError,
  reportGlobePlacesFailure,
} from '../basemap/basemapTelemetry'
import {
  globeSurfaceLayers,
  globeSurfaceSources,
  loadGlobeBoundaries,
  loadGlobePlaces,
  showGlobeSurface,
} from '../basemap/globeSurface'
import {
  type Box,
  type GlobePlace,
  dotBox,
  facingPoint,
  isFacing,
  parseGlobePlaces,
} from '../basemap/globePlaces'
import { type MapChromeWatch, mountPlaceLabels, watchMapChrome } from './globePlaceLabels'
import {
  type AtlasTheme,
  PH_BASEMAP_MIN_ZOOM,
  lightGroundPaperZoom,
  phBasemapFragment,
} from '../basemap/phBasemap'
import type {
  CameraSettle,
  FlyToScene,
  GlobePov,
  PlaceableScene,
  VenuePin,
  VenueStackMarker,
} from './globeTypes'
import type { BottomSheetDetent } from '@/components/ui/bottom-sheet'
import {
  dotClearOfSheet,
  scenePreviewCameraPadding,
  scenePreviewSheetPx,
} from '../scenePreviewCamera'
import { genreFamilyColor } from '../genreFamilies'
import {
  VENUE_PIN_INK,
  VENUE_PIN_STROKE,
  venuePinFeatures,
  venuePinPaint,
  venuePinRadiusPx,
} from './venuePinLayer'
import { readAtlasCamera, saveAtlasCamera } from './atlasCamera'
import { circleMarkAt } from './mapTapTarget'
import { AtlasMapUnrecoverableError } from '../atlasViewport'
import { atlasMapContextRefused, watchAtlasMapHealth } from './atlasMapHealth'
import {
  ATLAS_TOP_CREDIT_OFFSET_PX,
  CITY_VIEW_MIN_ZOOM,
  VENUE_LABEL_DECLUTTER_KM,
  labelledVenuePinIds,
  venuePinTooltipLines,
} from '../cityView'
import {
  DOT_COLOR_BASE,
  DOT_HOVER_RADIUS_SCALE,
  MARKER_RAMPS,
  globeScreenRadiusPx,
  labelMinCountForAltitude,
  labelMinCountForZoom,
  sceneDotColor,
  sceneDotRadiusPx,
  sceneDotSortKey,
  sceneLabelSizePx,
  sceneTooltipLabel,
  selectedRingStroke,
  visibleLabelScenes,
  zoomForAltitude,
} from './globeScale'
import { type GraphPalette, useGraphPalette } from '@/components/graph/graphPalette'

interface GlobeCanvasProps {
  width: number
  height: number
  scenes: PlaceableScene[]
  /**
   * Whether the globe draws its light look (flat ocean, vector land, boundary
   * lines and place labels; globeSurface.ts) instead of the night-earth
   * raster. The map is built with the value this has at construction; a later
   * change switches the live map's look and never rebuilds it.
   */
  lightGlobe: boolean
  /**
   * Camera focus. AtlasGlobe resolves this ONCE (the visitor-geo/default race
   * settles behind a guard) before mounting this canvas, and it's stable for
   * the component's lifetime — the map is aimed at it exactly once, at
   * construction (unless a saved camera from a previous show wins; see
   * the saved camera in atlasCamera.ts).
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
  flyToRef?: React.MutableRefObject<FlyToScene | null>
  /** Slugs of scenes the viewer follows (PSY-1340), tinted the ramp's followed colour. */
  followedSlugs?: ReadonlySet<string> | null
  /**
   * City view (PSY-1539): the venues to pin, ALREADY positioned and ALREADY
   * filtered by the rail. The canvas draws exactly this array — it is the same
   * array the rail lists, which is what keeps map and rail in sync by
   * construction rather than by two parallel filter passes.
   */
  venues?: readonly VenuePin[]
  /** The venue whose panel seam is open — its pin stays visually distinct. */
  selectedVenueId?: number | null
  /** Pin click. AtlasGlobe owns what opens; the canvas only reports the hit. */
  onVenueSelect?: (venueId: number) => void
  /**
   * "AUSTIN, TX" for the status chip, and the flag that city view is engaged:
   * non-null suppresses the globe-scale scene labels, which at street zoom
   * would stamp a city name over the venue layer.
   */
  cityLabel?: string | null
  /**
   * Camera position after a movement settles (moveend/zoomend), plus once on
   * style load so a restored street-zoom camera re-engages city view on
   * nav-back. Settle-only, never per-frame: AtlasGlobe turns this into React
   * state.
   */
  onCameraSettle?: (camera: CameraSettle) => void
  /**
   * Which corner holds the attribution control (the OpenStreetMap credit an
   * ODbL license requires stay visible). `top-left` sits under the status row,
   * for layouts whose bottom edge is owned by sheets.
   */
  attributionPosition?: 'bottom-left' | 'top-left'
  /**
   * When given, city view shows a back-to-globe control at the start of the
   * status row.
   */
  onBackToGlobe?: () => void
  /**
   * Points where several venues pin. Each draws one counted marker over its
   * pins (which stay on the pin layer, so a tap on one still routes to the
   * stack), and those pins lose their name labels. Requires
   * onVenueStackSelect.
   */
  venueStacks?: readonly VenueStackMarker[]
  /**
   * A tap on a stack's marker, or on any pin inside a stack, reports the
   * stack's key here instead of a single venue to onVenueSelect.
   */
  onVenueStackSelect?: (stackKey: string) => void
  /**
   * The detent of the bottom sheet the scene preview stands in: its current
   * detent while a scene is selected, and the detent it opens at otherwise.
   * Undefined when the preview is no bottom sheet. The preview camera effect
   * keeps the selected dot above this sheet.
   */
  scenePreviewSheetDetent?: BottomSheetDetent
}

// Camera altitude a fly-to lands at (legacy globe-altitude units — see
// zoomForAltitude) — closer than the initial continental POV (1.6–1.8) so
// arriving somewhere reads as a descent, but high enough that neighbouring
// scenes stay in frame.
const FLY_TO_ALTITUDE = 1.0
const FLY_TO_MS = 1200

// A camera's bottom padding within this of a target counts as at it: an
// ease's last frame can land a float rounding off its target, and an exact
// test would ease again after every such move.
const PADDING_TOLERANCE_PX = 0.5

/** Whether the camera's bottom padding is off `paddingPx`. */
function cameraPaddingDiffers(map: maplibregl.Map, paddingPx: number): boolean {
  return Math.abs((map.getPadding().bottom ?? 0) - paddingPx) > PADDING_TOLERANCE_PX
}

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

// The zoom from which the light theme's ground counts as paper for the map's
// labels (see lightGroundPaperZoom): below it the ground is the darker one.
export const LABEL_PAPER_GROUND_ZOOM = lightGroundPaperZoom(
  BLACK_MARBLE_FADE_START,
  BLACK_MARBLE_FADE_END,
)

// Street-zoom ceiling (PSY-1543). The OpenFreeMap vector source is maxzoom 14
// and overzooms cleanly; 17 gives block-level framing for the PSY-1539 venue
// pins without letting the camera dive into an empty ground plane. (The GIBS
// raster no longer binds this — its layer is capped at the fade end.)
const MAX_ZOOM = 17
const MIN_ZOOM = 1

// "Happening in the next 7 days" pulse ring (PSY-1309 parity): a propagating
// stroked circle under each scene with a show in the next 7 days. Same period
// and fade curve as the shipped globe; radius converted from the old 1.6
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

// The scene dot's outline width, drawn outside its radius, and the rim of
// every dot without a selected ring of its own.
const SCENE_DOT_STROKE_PX = 1
const SCENE_DOT_RIM_COLOR = 'rgba(255,230,194,0.35)'

// Every DOM label on the map (scene, venue and stack names) is drawn in the
// ink and halo of the ground under it: cream on a dark halo over the night
// globe and the dark streets, and, while the map root carries
// data-atlas-ground="paper", the ink and paper halo globals.css sets there.
const MAP_LABEL_INK = `color: var(--atlas-label-ink, ${MARKER_RAMPS.dark.selected})`
const MAP_LABEL_HALO = 'text-shadow: var(--atlas-label-halo, 0 1px 4px rgba(0,0,0,0.9))'

const EMPTY_FC: GeoJSON.FeatureCollection = {
  type: 'FeatureCollection',
  features: [],
}

// Stable empty default for the `venues` prop, so a caller that omits it can't
// churn the venue-layer memo on every render.
const EMPTY_VENUES: readonly VenuePin[] = []
const EMPTY_STACKS: readonly VenueStackMarker[] = []

// The stack marker's count badge diameter: at least the 24px minimum target.
const STACK_BADGE_PX = 24

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

/** What the camera left when a map was removed (see the preview camera rule). */
interface LeftPreviewCamera {
  paddingPx: number
  settled: boolean
  slug: string | null
  sheetPx: number
}

/**
 * The globe canvas for the app theme in force. The theme keys the map: a
 * theme change unmounts the map built for the old theme and mounts one for
 * the new, the same remove-and-create a hide and show runs, so a map's street
 * style, marker ramp and every paint built from them are fixed for its
 * lifetime and no `setStyle` ever drops its sources or hover state. The
 * camera carries over as it does across a hide: the saved camera
 * (atlasCamera.ts) and the preview padding the old map left, held here so
 * the remount keeps it.
 */
export default function GlobeCanvas(props: GlobeCanvasProps) {
  // Resolved theme palette: the theme, and the dominant-genre dot tint (PSY-1315).
  const palette = useGraphPalette()
  const theme: AtlasTheme = palette.isDark ? 'dark' : 'light'
  const leftPreviewCameraRef = useRef<LeftPreviewCamera | null>(null)
  return (
    <GlobeMap
      key={theme}
      {...props}
      theme={theme}
      palette={palette}
      leftPreviewCameraRef={leftPreviewCameraRef}
    />
  )
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
 * altitude bands via labelMinCountForZoom).
 */
function GlobeMap({
  theme,
  palette,
  leftPreviewCameraRef,
  width,
  height,
  scenes,
  lightGlobe,
  pov,
  onSelect,
  selected = null,
  flyToRef,
  followedSlugs = null,
  venues = EMPTY_VENUES,
  selectedVenueId = null,
  onVenueSelect,
  cityLabel = null,
  onCameraSettle,
  attributionPosition = 'bottom-left',
  onBackToGlobe,
  venueStacks = EMPTY_STACKS,
  onVenueStackSelect,
  scenePreviewSheetDetent,
}: GlobeCanvasProps & {
  /** The app theme this map is drawn for, fixed for its lifetime. */
  theme: AtlasTheme
  palette: GraphPalette
  /** Held by GlobeCanvas, so it outlives a remount for a new theme. */
  leftPreviewCameraRef: React.MutableRefObject<LeftPreviewCamera | null>
}) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  // The root element: the page draws its chrome beside it.
  const globeRootRef = useRef<HTMLDivElement | null>(null)
  const tooltipRef = useRef<HTMLDivElement | null>(null)
  const haloRef = useRef<HTMLDivElement | null>(null)
  const statusChipRef = useRef<HTMLDivElement | null>(null)
  const venueTooltipRef = useRef<HTMLDivElement | null>(null)
  const venueTooltipNameRef = useRef<HTMLDivElement | null>(null)
  const venueTooltipMetaRef = useRef<HTMLDivElement | null>(null)
  const venueTooltipCtaRef = useRef<HTMLDivElement | null>(null)
  // Filled by the map effect (closes over that map), nulled in its cleanup —
  // callers must stay null-safe, same contract as flyToRef.
  const redrawStatusChipRef = useRef<(() => void) | null>(null)
  // The scene label markers' elements and where they stand, for the
  // place-label collision pass.
  const sceneLabelsRef = useRef<{ el: HTMLElement; lng: number; lat: number }[]>([])
  const clearVenueHoverRef = useRef<(() => void) | null>(null)
  // Clears every hover the map handlers hold: the scene dot and the venue pin,
  // their tooltips and the pointer cursor. clearVenueHoverRef clears only the
  // pin's, for a change to the pin set that leaves the dots as they are. Both
  // are set by the map effect and nulled in its cleanup.
  const clearPointerHoverRef = useRef<(() => void) | null>(null)
  // The style-loaded map instance, in STATE so the data/label/ring effects
  // below re-run against each fresh map after a hide/show cycle, and against
  // the re-created style after a restored WebGL context. Null while the
  // context is lost.
  const [mapReady, setMapReady] = useState<maplibregl.Map | null>(null)
  // The same map once its first full render is in: style and every visible
  // source loaded. The light globe's overlays wait for it, so their downloads
  // never hold up that first frame. It stays set through a lost context, so an
  // effect that writes to the style also requires mapReady.
  const [mapLoaded, setMapLoaded] = useState<maplibregl.Map | null>(null)
  // A failure MapLibre reports from outside React (see atlasMapHealth.ts).
  // Rethrown from render, so it reaches the same error boundary as a failure
  // inside React.
  const [unrecoverable, setUnrecoverable] =
    useState<AtlasMapUnrecoverableError | null>(null)

  const selectedSlug = selected?.slug ?? null

  // The look the map-creation effect builds the map with. A later change
  // never rebuilds the map: the effect below switches the live map's look.
  // The light globe's place labels are the effect after the scene labels.
  const lightGlobeRef = useRef(lightGlobe)
  useEffect(() => {
    lightGlobeRef.current = lightGlobe
  }, [lightGlobe])
  useEffect(() => {
    if (mapReady) showGlobeSurface(mapReady, lightGlobe)
  }, [mapReady, lightGlobe])
  // Keyed on mapReady as well, so a restored style (whose sources are new)
  // gets its boundary data again.
  useEffect(() => {
    if (mapReady && mapLoaded && lightGlobe) loadGlobeBoundaries(mapReady)
  }, [mapReady, mapLoaded, lightGlobe])

  // The light globe's place-label data, loaded the first time a loaded map
  // shows the light look and kept for this canvas's lifetime.
  const [places, setPlaces] = useState<readonly GlobePlace[] | null>(null)
  const wantPlaces = mapLoaded !== null && lightGlobe && places === null
  useEffect(() => {
    if (!wantPlaces) return
    let cancelled = false
    void loadGlobePlaces().then((result) => {
      if (cancelled) return
      if (result.data) setPlaces(parseGlobePlaces(result.data))
      else reportGlobePlacesFailure(result.status)
    })
    return () => {
      cancelled = true
    }
  }, [wantPlaces])

  const ramp = MARKER_RAMPS[theme]
  // The widest outline a dot on this map draws: a selected ring, if the ramp
  // has one, else the rim.
  const dotMaxStrokePx = Math.max(SCENE_DOT_STROKE_PX, ramp.selectedRing?.widthPx ?? 0)

  // Latest onSelect without threading it into the map-creation effect's deps
  // (handlers read it at call time).
  const onSelectRef = useRef(onSelect)
  useEffect(() => {
    onSelectRef.current = onSelect
  }, [onSelect])

  // Same treatment for the city-view callbacks. These MUST stay out of the
  // map-creation effect's deps: PSY-1538 shipped a HIGH where an object whose
  // identity churned per nav-back rebuilt the entire map, invisible locally
  // because the churning value happened to be constant in dev.
  const onVenueSelectRef = useRef(onVenueSelect)
  useEffect(() => {
    onVenueSelectRef.current = onVenueSelect
  }, [onVenueSelect])
  const onCameraSettleRef = useRef(onCameraSettle)
  useEffect(() => {
    onCameraSettleRef.current = onCameraSettle
  }, [onCameraSettle])
  const onVenueStackSelectRef = useRef(onVenueStackSelect)
  useEffect(() => {
    onVenueStackSelectRef.current = onVenueStackSelect
  }, [onVenueStackSelect])
  // The preview sheet's height over this canvas; 0 with no sheet. Read by
  // the fly-to seam, which the map-creation effect builds once, through the
  // ref.
  const previewSheetPx =
    scenePreviewSheetDetent === undefined ? 0 : scenePreviewSheetPx(scenePreviewSheetDetent, height)
  const previewSheetPxRef = useRef(previewSheetPx)
  useEffect(() => {
    previewSheetPxRef.current = previewSheetPx
  }, [previewSheetPx])
  // While the device's primary pointer is coarse (`pointer: coarse`), the
  // scene dots and venue pins show no tooltip, set no hover highlight and leave
  // the cursor alone, whatever input sends the mousemove. The browser emulates
  // a mousemove before a tap's click, and Safari on iOS drops the click when
  // that mousemove changes the page. A coarse pointer's click hit-tests a tap
  // box rather than the drawn mark (circleMarkAt). The hover and click
  // handlers of both layers read this ref; a switch to a coarse primary
  // pointer clears any hover already held.
  const coarsePointer = useCoarsePointer()
  const coarsePointerRef = useRef(coarsePointer)
  useEffect(() => {
    coarsePointerRef.current = coarsePointer
    if (coarsePointer) clearPointerHoverRef.current?.()
  }, [coarsePointer])
  // The stacks by member venue id and by key, for the pin hover and click
  // handlers.
  const stacksRef = useRef<{
    byVenueId: ReadonlyMap<number, VenueStackMarker>
    byKey: ReadonlyMap<string, VenueStackMarker>
  }>({ byVenueId: new Map(), byKey: new Map() })
  useEffect(() => {
    const byVenueId = new Map<number, VenueStackMarker>()
    for (const stack of venueStacks) {
      for (const id of stack.venueIds) byVenueId.set(id, stack)
    }
    stacksRef.current = {
      byVenueId,
      byKey: new Map(venueStacks.map((s) => [s.key, s])),
    }
  }, [venueStacks])
  // A pin's tooltip depends on the stacks and on the stack handler, so a
  // change to either clears a held pin hover; the next mousemove rebuilds it.
  useEffect(() => {
    clearVenueHoverRef.current?.()
  }, [venueStacks, onVenueStackSelect])
  // The status chip is painted imperatively (see the map effect), so the label
  // rides a ref rather than re-entering the effect's deps.
  const cityLabelRef = useRef(cityLabel)
  useEffect(() => {
    cityLabelRef.current = cityLabel
  }, [cityLabel])

  // PSY-1223 zoom-gated labels: the discrete threshold, from the calibrated
  // bands (globeScale owns the zoom translation). Seeded from the camera the
  // map will actually open on (saved camera from a previous show, else the
  // resolved POV — already in the altitude units the bands were tuned in).
  const [labelMinCount, setLabelMinCount] = useState(() => {
    const restored = readAtlasCamera()
    return restored
      ? labelMinCountForZoom(restored.zoom)
      : labelMinCountForAltitude(pov.altitude)
  })

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
            color: sceneDotColor(ramp, s.slug, null, selectedSlug, followedSlugs, genreBase),
            radiusPx: sceneDotRadiusPx(s.upcoming_show_count),
            // The hover color must not override the selected color: the
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
    [scenes, selectedSlug, followedSlugs, palette, ramp],
  )

  useEffect(() => {
    const src = mapReady?.getSource('scenes') as
      | maplibregl.GeoJSONSource
      | undefined
    src?.setData(sceneFeatures)
  }, [mapReady, sceneFeatures])

  // ── City-view venue pins (PSY-1539) ───────────────────────────────────────
  // Same division of labour as the scene dots: slow-changing state is baked
  // into feature properties, hover rides feature-state so a mousemove never
  // rebuilds the source.
  const venueFeatures = useMemo<GeoJSON.FeatureCollection>(
    () => venuePinFeatures(venues, ramp, selectedVenueId ?? null),
    [venues, ramp, selectedVenueId],
  )

  useEffect(() => {
    const src = mapReady?.getSource('venues') as
      | maplibregl.GeoJSONSource
      | undefined
    src?.setData(venueFeatures)
    // A rail filter chip is a DOM click, not a map pointer event, so it can
    // delete the pin under the cursor without any mousemove/mouseleave firing.
    // Without this the tooltip would keep describing a venue that is no longer
    // on the map, over a pointer cursor pointing at nothing, until the user
    // happened to move the mouse. Clear the hover whenever the pin set changes.
    clearVenueHoverRef.current?.()
  }, [mapReady, venueFeatures])

  // Live id → pin lookup for the map handlers (feature properties carry only
  // the id, same contract the scene dots use with their slug).
  const venuesByIdRef = useRef<ReadonlyMap<number, VenuePin>>(new Map())
  useEffect(() => {
    venuesByIdRef.current = new Map(venues.map((v) => [v.id, v]))
  }, [venues])

  // Venue name labels, as DOM markers for the same reasons the scene labels
  // are: app font, no glyph-server dependency. Anchored below the pin so the
  // name never covers the mark it belongs to.
  // A stacked pin is named by its stack marker, not by one member's label, and
  // the marker's label claims its point in the declutter.
  const labelledVenueIds = useMemo(() => {
    if (venueStacks.length === 0) return labelledVenuePinIds(venues)
    const stacked = new Set(venueStacks.flatMap((s) => s.venueIds))
    return labelledVenuePinIds(
      venues.filter((v) => !stacked.has(v.id)),
      VENUE_LABEL_DECLUTTER_KM,
      venueStacks,
    )
  }, [venues, venueStacks])
  useEffect(() => {
    if (!mapReady || venues.length === 0) return
    const markers = venues.filter((v) => labelledVenueIds.has(v.id)).map((v) => {
      const el = document.createElement('div')
      el.textContent = v.name
      el.style.cssText = [
        'pointer-events: none',
        'user-select: none',
        'white-space: nowrap',
        MAP_LABEL_INK,
        'font-size: 12px',
        'font-weight: 500',
        'letter-spacing: 0.01em',
        MAP_LABEL_HALO,
      ].join(';')
      return new maplibregl.Marker({
        element: el,
        anchor: 'top',
        offset: [0, Math.ceil(venuePinRadiusPx(v.upcomingShowCount)) + 3],
      })
        .setLngLat([v.lng, v.lat])
        .addTo(mapReady)
    })
    return () => {
      for (const m of markers) m.remove()
    }
  }, [mapReady, venues, labelledVenueIds])

  // Stack markers: a counted button per point where several venues pin, as a
  // DOM marker so it is a real, focusable target in the app font.
  useEffect(() => {
    if (!mapReady || venueStacks.length === 0) return
    const markers = venueStacks.map((stack) => {
      const button = document.createElement('button')
      button.type = 'button'
      button.dataset.testid = 'atlas-venue-stack'
      button.dataset.stackKey = stack.key
      button.className =
        'flex flex-col items-center gap-0.5 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
      const badge = document.createElement('span')
      badge.setAttribute('aria-hidden', 'true')
      badge.textContent = String(stack.venueIds.length)
      badge.style.cssText = [
        `width: ${STACK_BADGE_PX}px`,
        `height: ${STACK_BADGE_PX}px`,
        'display: flex',
        'align-items: center',
        'justify-content: center',
        'border-radius: 9999px',
        `background: ${DOT_COLOR_BASE}`,
        `border: 1.5px solid ${VENUE_PIN_STROKE}`,
        `color: ${VENUE_PIN_INK}`,
        'font-size: 11px',
        'font-weight: 700',
      ].join(';')
      const label = document.createElement('span')
      // textContent, not innerHTML: the label carries a count only, but the
      // rule here is that nothing in the map's DOM is parsed as markup.
      label.textContent = stack.label
      label.style.cssText = [
        'white-space: nowrap',
        MAP_LABEL_INK,
        'font-size: 12px',
        'font-weight: 500',
        MAP_LABEL_HALO,
      ].join(';')
      button.append(badge, label)
      // Named explicitly: MapLibre labels a marker element "Map marker" unless
      // it already carries an aria-label, which would hide the visible text.
      button.setAttribute('aria-label', stack.label)
      button.addEventListener('click', (e) => {
        e.stopPropagation()
        onVenueStackSelectRef.current?.(stack.key)
      })
      return new maplibregl.Marker({
        element: button,
        // Anchored by the top edge and lifted half a badge, so the badge's
        // centre sits on the shared point and the label hangs below it.
        anchor: 'top',
        offset: [0, -STACK_BADGE_PX / 2],
      })
        .setLngLat([stack.lng, stack.lat])
        .addTo(mapReady)
    })
    return () => {
      for (const m of markers) m.remove()
    }
  }, [mapReady, venueStacks])

  // Attribution: the OpenStreetMap credit is a license requirement (ODbL), so
  // the control is always mounted and never compact (OSM's guidance frowns on
  // credit hidden behind an icon). Credit strings come from the sources
  // (OpenFreeMap/OSM, plus NASA GIBS while the raster layer is visible); the
  // dark restyle and the top-left placement offsets live in globals.css.
  //
  // `bottom-left` by default: GenreLegend owns bottom-right at z-10 (the
  // control's own stacking context tops out at z-index 2, so the legend would
  // win) and the desktop panels dock to the right edge. Bottom-left is the one
  // corner nothing else docks to there; the "N more scenes" link that shares it
  // is offset above the strip in AtlasGlobe.
  useEffect(() => {
    if (!mapReady) return
    const control = new maplibregl.AttributionControl({ compact: false })
    mapReady.addControl(control, attributionPosition)
    return () => {
      mapReady.removeControl(control)
    }
  }, [mapReady, attributionPosition])

  // The ground the labels sit on. In the light theme it is paper from
  // LABEL_PAPER_GROUND_ZOOM down to the streets; above it, and at every zoom
  // in the dark theme, it is dark. The attribute is written only when that
  // answer changes, so a zoom gesture costs one comparison a frame.
  useEffect(() => {
    const root = globeRootRef.current
    if (!mapReady || !root || theme === 'dark') return
    const update = () => {
      const paper = mapReady.getZoom() >= LABEL_PAPER_GROUND_ZOOM
      if ((root.dataset.atlasGround === 'paper') !== paper) {
        if (paper) root.dataset.atlasGround = 'paper'
        else delete root.dataset.atlasGround
      }
    }
    update()
    mapReady.on('zoom', update)
    return () => {
      mapReady.off('zoom', update)
    }
  }, [mapReady, theme])

  // Zoom controls, mounted only while city view is engaged — the mock's
  // street view has them, the globe deliberately stays chrome-free (and in
  // the panel layout GenreLegend docks bottom-right there). Plain add/remove,
  // no guard ref.
  const cityViewActive = cityLabel !== null
  useEffect(() => {
    if (!mapReady || !cityViewActive) return
    // showCompass:false — rotation is disabled on every input path, so a
    // compass would be a control that can never do anything.
    const control = new maplibregl.NavigationControl({ showCompass: false })
    mapReady.addControl(control, 'bottom-right')
    return () => {
      mapReady.removeControl(control)
    }
  }, [mapReady, cityViewActive])

  // Repaint the status chip when its inputs change with the camera at rest.
  useEffect(() => {
    redrawStatusChipRef.current?.()
  }, [mapReady, cityLabel, venues])

  // ── Scene-mark handoff ────────────────────────────────────────────────────
  // Scene dots are a GLOBE-scale abstraction: at street zoom the city is a
  // rail of venues, not one aggregate dot sitting on top of the
  // centroid-pinned venues underneath it.
  //
  // The handoff is driven by cityViewActive — React state that lands on camera
  // SETTLE — rather than by a zoom-keyed maxzoom on the layers. A zoom key is
  // evaluated by the GL renderer every frame, so a continuous pinch or wheel
  // zoom through the threshold would blank the scene dots mid-gesture while
  // the venue pins were still waiting on React, leaving one empty frame-run
  // with neither layer drawn. Tying both sides to the same state means the
  // dots hand over exactly when the rail appears.
  useEffect(() => {
    if (!mapReady) return
    const visibility = cityViewActive ? 'none' : 'visible'
    for (const layer of ['scene-dots', 'scene-rings']) {
      if (mapReady.getLayer(layer)) {
        mapReady.setLayoutProperty(layer, 'visibility', visibility)
      }
    }
  }, [mapReady, cityViewActive])

  // PSY-1309 pulse rings. prefers-reduced-motion suppresses the animation
  // entirely (no ring features, no rAF loop) rather than freezing a ring frame,
  // and follows the preference live: turning it on stops the rings.
  const prefersReducedMotion = useReducedMotion()
  const pulseScenes = useMemo(
    () =>
      prefersReducedMotion ? [] : scenes.filter((s) => s.shows_this_week > 0),
    [scenes, prefersReducedMotion],
  )

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
      // Read again: after a lost context the captured source belongs to a
      // destroyed style, and the live map has none until the restore.
      ;(mapReady.getSource('scene-rings') as maplibregl.GeoJSONSource | undefined)?.setData(
        EMPTY_FC,
      )
    }
  }, [mapReady, pulseScenes])

  // Zoom-gated + decluttered labels as DOM markers: crisp text in the app
  // font with no glyph-server dependency, and MapLibre's globe pipeline
  // fades markers occluded behind the horizon. pointer-events: none so labels
  // never swallow globe drags or dot clicks.
  //
  // Suppressed entirely once city view engages (PSY-1539): at street zoom the
  // label gate is "label everything", which would stamp every scene name over
  // the venue layer — and the basemap already labels the city itself.
  useEffect(() => {
    if (!mapReady || cityViewActive) return
    const labels: { el: HTMLElement; lng: number; lat: number }[] = []
    const markers = labelScenes.map((s) => {
      const el = document.createElement('div')
      labels.push({ el, lng: s.longitude, lat: s.latitude })
      el.textContent = s.city
      el.dataset.testid = 'atlas-scene-label'
      el.style.cssText = [
        'pointer-events: none',
        'user-select: none',
        'white-space: nowrap',
        MAP_LABEL_INK,
        'font-weight: 500',
        'letter-spacing: 0.01em',
        MAP_LABEL_HALO,
        // Above the place labels, which carry no z-index.
        'z-index: 1',
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
    sceneLabelsRef.current = labels
    return () => {
      sceneLabelsRef.current = []
      for (const m of markers) m.remove()
    }
  }, [mapReady, labelScenes, cityViewActive])

  // Place labels draw on the light globe at globe altitude, once their data
  // is in.
  const placeLabelsShown = lightGlobe && !cityViewActive && places !== null && places.length > 0

  // The controls drawn over the map, watched for the place labels
  // (watchMapChrome). Keyed on the map and on whether place labels show,
  // never on a label set, so a camera flight that changes the scene labels
  // keeps this watch and the boxes it has read. Declared before the
  // place-label effect, which reads chromeRef: this effect's deps are among
  // that effect's, so whenever the watch is replaced the labels mount again
  // against the new one.
  const chromeRef = useRef<MapChromeWatch | null>(null)
  useEffect(() => {
    const globeRoot = globeRootRef.current
    if (!mapReady || !placeLabelsShown || !globeRoot) return
    const watch = watchMapChrome(mapReady, globeRoot)
    chromeRef.current = watch
    return () => {
      chromeRef.current = null
      watch.stop()
    }
  }, [mapReady, placeLabelsShown])

  // Place labels on the light globe (globePlaceLabels.ts), clear of every
  // scene label and dot on the near side of the globe, so a scene always wins
  // its spot, and of the controls drawn over the map. Declared after the scene
  // label effect and keyed on the same label set, so whenever that effect
  // rebuilds its markers this one lays the place labels out again against
  // them.
  useEffect(() => {
    const chrome = chromeRef.current
    if (!mapReady || !placeLabelsShown || !places || !chrome) return
    const map = mapReady
    const obstacles = (): Box[] => {
      const container = map.getContainer()
      const origin = container.getBoundingClientRect()
      const boxes: Box[] = []
      for (const { el, lng, lat } of sceneLabelsRef.current) {
        // A label behind the globe is hidden and holds no spot on screen. Its
        // element keeps a box at the far-side projection, so the box alone
        // cannot tell; the same far-side test the place labels use decides.
        if (!isFacing(map, lng, lat)) continue
        const r = el.getBoundingClientRect()
        boxes.push({
          left: r.left - origin.left,
          top: r.top - origin.top,
          right: r.right - origin.left,
          bottom: r.bottom - origin.top,
        })
      }
      // A dot centred just outside the pane can still draw into it, so dots
      // are kept by side, not by pane.
      for (const s of scenes) {
        if (!isFacing(map, s.longitude, s.latitude)) continue
        const point = map.project([s.longitude, s.latitude])
        // The most a dot draws: its hovered radius plus its widest stroke.
        const radius =
          sceneDotRadiusPx(s.upcoming_show_count) * DOT_HOVER_RADIUS_SCALE + dotMaxStrokePx
        boxes.push(dotBox(point.x, point.y, radius))
      }
      return boxes
    }
    return mountPlaceLabels(map, places, {
      maxZoom: BLACK_MARBLE_FADE_START,
      obstacles,
      chrome,
    })
  }, [mapReady, placeLabelsShown, places, labelScenes, scenes, dotMaxStrokePx])

  // ── Preview camera: the selected dot above the preview sheet ─────────────
  // The rule, for the sheet layout:
  // - Selecting a scene, and each change of the preview sheet's height while
  //   one is selected, eases the selected dot to the centre of the band the
  //   sheet leaves visible: the camera's bottom padding becomes the sheet's
  //   height, at the same globe size (no zoom is passed).
  // - The exception is the sheet at Peek with the dot already wholly above
  //   it: the camera, padding included, stays as it is.
  // - With no selection, or no sheet, the camera keeps its centre and any
  //   padding eases back to 0.
  // The camera moves once per change, and never while another movement is
  // in progress: it waits for that movement's moveend, then for the next
  // frame, since moveend can fire from inside another camera call that would
  // overwrite a move started there. A move stopped part-way has its padding
  // finished once the camera is still (finishPreviewPadding, beside the map).
  // appliedPreviewCameraRef holds, for one map instance, the last selection
  // and sheet height this rule was applied for and the padding it sent the
  // camera to (null: none yet on that map). The fly-to seam writes it for a flight
  // that opens the preview, and a map rebuilt on a settled camera starts
  // with it, so neither needs a second move; a restored context keeps it.
  const previewCameraSheetPx = selectedSlug === null ? 0 : previewSheetPx
  const appliedPreviewCameraRef = useRef<{
    map: maplibregl.Map
    slug: string | null
    sheetPx: number
    /** The bottom padding the camera was sent to (kept, at the Peek exception). */
    paddingPx: number
  } | null>(null)
  // The current selection and sheet height, for a map built later.
  const previewCameraTargetRef = useRef({ slug: selectedSlug, sheetPx: previewCameraSheetPx })
  useEffect(() => {
    previewCameraTargetRef.current = { slug: selectedSlug, sheetPx: previewCameraSheetPx }
  }, [selectedSlug, previewCameraSheetPx])
  useEffect(() => {
    const map = mapReady
    if (!map) return
    const applied = appliedPreviewCameraRef.current
    if (
      applied?.map === map &&
      applied.slug === selectedSlug &&
      applied.sheetPx === previewCameraSheetPx
    ) {
      return
    }
    const apply = () => {
      const record = (paddingPx: number) => {
        appliedPreviewCameraRef.current = {
          map,
          slug: selectedSlug,
          sheetPx: previewCameraSheetPx,
          paddingPx,
        }
      }
      const padding = scenePreviewCameraPadding(previewCameraSheetPx)
      const scene =
        selectedSlug === null ? undefined : scenesBySlugRef.current.get(selectedSlug)
      if (!scene || previewCameraSheetPx === 0) {
        record(0)
        if (cameraPaddingDiffers(map, 0)) map.easeTo({ padding })
        return
      }
      if (
        scenePreviewSheetDetent === 'peek' &&
        dotClearOfSheet({
          point: facingPoint(map, scene.longitude, scene.latitude, width, height),
          radiusPx: sceneDotRadiusPx(scene.upcoming_show_count),
          width,
          height,
          sheetPx: previewCameraSheetPx,
        })
      ) {
        record(map.getPadding().bottom ?? 0)
        return
      }
      record(previewCameraSheetPx)
      map.easeTo({ center: [scene.longitude, scene.latitude], padding })
    }
    let frame = 0
    const applyWhenStill = () => {
      if (map.isMoving()) {
        map.once('moveend', applyNextFrame)
        return
      }
      apply()
    }
    const applyNextFrame = () => {
      frame = requestAnimationFrame(applyWhenStill)
    }
    applyWhenStill()
    return () => {
      map.off('moveend', applyNextFrame)
      cancelAnimationFrame(frame)
    }
  }, [mapReady, selectedSlug, previewCameraSheetPx, scenePreviewSheetDetent, width, height])

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

    const restored = readAtlasCamera()
    const center: [number, number] = restored?.center ?? [pov.lng, pov.lat]
    const zoom = restored?.zoom ?? zoomForAltitude(pov.altitude)

    // PH street basemap (PSY-1543): OpenFreeMap vector tiles restyled to the
    // theme's tokens, with its background ramped in across the same range
    // the globe surface (raster, or ocean and land) fades out; see
    // phBasemapFragment for the handoff.
    const basemap = phBasemapFragment(
      BLACK_MARBLE_FADE_START,
      BLACK_MARBLE_FADE_END,
      theme,
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
        // Background and fill layers draw on the sphere only on the globe
        // projection, so space stays transparent and the CSS starfield and
        // halo behind the canvas show through.
        sources: {
          ...basemap.sources,
          ...globeSurfaceSources(),
          // promoteId: features are keyed by slug so the hover feature-state
          // (set in handleMove below) sticks across setData refreshes.
          scenes: { type: 'geojson', data: EMPTY_FC, promoteId: 'slug' },
          'scene-rings': { type: 'geojson', data: EMPTY_FC },
          // City-view venue pins (PSY-1539). promoteId for the same reason
          // the scenes source has one: hover feature-state must survive the
          // setData refresh that a filter change causes.
          venues: { type: 'geojson', data: EMPTY_FC, promoteId: 'id' },
        },
        layers: [
          // Street basemap under the globe surface, which covers it at globe
          // zooms and fades out across the handoff (see globeSurface.ts), so
          // the streets are already drawn when it dissolves.
          ...basemap.layers,
          // Built with the current look's visibility, so the first frame of
          // a light globe never requests a raster tile.
          ...globeSurfaceLayers({
            lightGlobe: lightGlobeRef.current,
            fadeOut: basemap.rasterFadeOut,
            maxZoom: basemap.rasterMaxZoom,
          }),
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
            // colour must not be overridden by hover (selected > hovered,
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
                ramp.hovered,
                ['get', 'color'],
              ],
              ...selectedRingStroke(ramp, SCENE_DOT_STROKE_PX, SCENE_DOT_RIM_COLOR),
            },
          },
          {
            // Venue pins, ABOVE the scene marks so the two never fight during
            // the handoff. The scene marks hand off on React state (see the
            // handoff effect); this minzoom is the pins' own floor, and it
            // earns its keep on the way OUT: zooming back to the globe, it
            // stops a not-yet-cleared pin set from painting city venues over
            // the earth in the frames before React settles.
            id: 'venue-pins',
            type: 'circle',
            source: 'venues',
            minzoom: CITY_VIEW_MIN_ZOOM,
            // The affordance ramp (selected > hovered > base) is shared with
            // the /venues mini Atlas so the same colors mean the same things
            // on every surface that draws a room.
            paint: venuePinPaint(ramp),
          },
        ],
      },
    })

    // Throwing reaches the error boundary AtlasGlobe wraps around this canvas,
    // and AtlasGlobe swaps in the scene list. The half-built map is left alone
    // (see atlasMapContextRefused).
    if (atlasMapContextRefused(map)) {
      throw new AtlasMapUnrecoverableError('context-refused')
    }

    // Basemap failure signal (PSY-1568, PSY-1936), registered FIRST so the
    // style's own TileJSON fetch — the earliest thing that can fail — is
    // already covered. The handler restores MapLibre's default console.error
    // (attaching any listener suppresses it) and reports a failure of a
    // ground source (the OpenFreeMap vector tiles, the GIBS raster, or one of
    // the light globe's same-origin files) to Sentry once per session per
    // source;
    // basemapTelemetry.ts owns the filtering and the throttle. Removed with
    // the map in cleanup, like every listener here.
    map.on('error', handleBasemapError)

    // Context loss and style failures that leave the map unable to draw, and
    // whether the style can be written. A lost context destroys the style.
    // While it is gone some style writes throw (feature state, layout), and
    // until the style MapLibre re-creates on restore has loaded every checked
    // write throws; reads such as getSource answer undefined. Clearing
    // mapReady pauses every effect that writes to the style; the restored
    // style sets it again, and they write the current data to it. The null is
    // load-bearing: mapReady holds the same map object before and after, so
    // only the committed null in between (the loss and the restored style.load
    // are separate events, a frame or more apart) makes those effects run
    // again.
    // The scenes source is the first-render watchdog's: a map that renders no
    // frame once that source has loaded is given up.
    const health = watchAtlasMapHealth(
      map,
      setUnrecoverable,
      (live) => {
        if (live) setMapReady(map)
        else setMapReady((prev) => (prev === map ? null : prev))
      },
      'scenes',
    )

    // See the constructor options: bearing/pitch must stay locked at 0 on
    // every input path (the saved camera persists only center/zoom).
    map.touchZoomRotate.disableRotation()
    map.keyboard.disableRotation()

    // A map rebuilt on the saved camera (the page shown again) takes the
    // padding the previous map's camera had, since the saved centre is that
    // camera's padded centre: the view comes back as it was, unmoved. When
    // that camera had settled where the preview camera effect last sent it,
    // and the selection and sheet are unchanged, nothing is left to apply;
    // otherwise the effect makes its one move on this map.
    const leftCamera = leftPreviewCameraRef.current
    const previewTarget = previewCameraTargetRef.current
    const restoredLeftCamera = restored !== null && leftCamera !== null
    const startPaddingPx = restoredLeftCamera ? leftCamera.paddingPx : 0
    if (startPaddingPx > 0) map.setPadding(scenePreviewCameraPadding(startPaddingPx))
    appliedPreviewCameraRef.current =
      restoredLeftCamera &&
      leftCamera.settled &&
      leftCamera.slug === previewTarget.slug &&
      leftCamera.sheetPx === previewTarget.sheetPx
        ? { map, ...previewTarget, paddingPx: startPaddingPx }
        : null

    // Fill the parent's fly-to seam (PSY-1308, reused by search/Drift).
    // Closes over THIS map; nulled in cleanup, so after a hide/show cycle
    // the seam always points at the live instance. MapLibre honors
    // prefers-reduced-motion natively — flyTo degrades to a jump cut unless
    // the animation is marked `essential` (verified in the v6 bundle).
    // A flight that opens the preview lands with the padding the preview's
    // sheet keeps, and records it for the preview camera effect.
    if (flyToRef) {
      flyToRef.current = (scene, options) => {
        const sheetPx = options?.opensPreview ? previewSheetPxRef.current : 0
        map.flyTo({
          center: [scene.longitude, scene.latitude],
          zoom: zoomForAltitude(FLY_TO_ALTITUDE),
          duration: FLY_TO_MS,
          padding: scenePreviewCameraPadding(sheetPx),
        })
        appliedPreviewCameraRef.current = {
          map,
          slug: options?.opensPreview ? scene.slug : null,
          sheetPx,
          paddingPx: sheetPx,
        }
      }
    }

    // A move stopped part-way (a gesture, another camera call) leaves its
    // padding part-way too. Once the camera is still, the padding is finished
    // to what the preview camera last sent it to, keeping the centre. Checked
    // on the frame after moveend, which can fire inside another camera call;
    // a movement under way by then ends in a moveend of its own.
    let paddingFrame = 0
    const finishPreviewPadding = () => {
      cancelAnimationFrame(paddingFrame)
      paddingFrame = requestAnimationFrame(() => {
        const applied = appliedPreviewCameraRef.current
        if (map.isMoving() || applied?.map !== map) return
        if (cameraPaddingDiffers(map, applied.paddingPx)) {
          map.easeTo({ padding: scenePreviewCameraPadding(applied.paddingPx) })
        }
      })
    }
    map.on('moveend', finishPreviewPadding)

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

    // Dots/labels/rings mount once the style is live (mapReady, set from
    // the health watch on 'style.load': style parsed, sources registered),
    // NOT on 'load': 'load' additionally waits for every in-view GIBS raster
    // tile, and a hanging third-party tile response would hold the page's
    // actual content (locally available scene data) hostage to the basemap.
    // The harness flag stays on 'load' (full first render).
    map.on('load', () => {
      w.__atlasMapLoaded = true
    })
    // The first full render, read on 'render' rather than 'load': 'load' also
    // needs a frame with no style change pending, which the pulse-ring
    // animation (a paint change every frame) can withhold indefinitely.
    const handleFirstFullRender = () => {
      if (!map.isStyleLoaded() || !map.areTilesLoaded()) return
      map.off('render', handleFirstFullRender)
      setMapLoaded(map)
      markAtlasMapReady()
    }
    map.on('render', handleFirstFullRender)

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
    // The globe is drawn about the centre of the padded viewport, so the halo
    // sits there too: half the difference of the vertical padding off the
    // container's centre. Written only when that offset changes, starting
    // from the padding the map was built with (only ever bottom padding).
    let haloOffsetPx = Number.NaN
    const writeHaloOffset = (offset: number) => {
      const halo = haloRef.current
      if (!halo || offset === haloOffsetPx) return
      haloOffsetPx = offset
      if (offset === 0) halo.style.removeProperty('top')
      else halo.style.top = `calc(50% ${offset < 0 ? '-' : '+'} ${Math.abs(offset)}px)`
    }
    writeHaloOffset(-startPaddingPx / 2)
    const updateHaloOffset = () => {
      const { top = 0, bottom = 0 } = map.getPadding()
      writeHaloOffset((top - bottom) / 2)
    }
    map.on('move', updateHaloOffset)

    // Zoom drives the discrete label threshold; only threshold CROSSINGS
    // change state (and thus label markers) — micro-zooms are free.
    const handleZoom = () => {
      const next = labelMinCountForZoom(map.getZoom())
      setLabelMinCount((prev) => (prev === next ? prev : next))
      updateHalo()
    }
    map.on('zoom', handleZoom)

    // Highest sort key first, the SAME key that decides draw order
    // (globeScale.sceneDotSortKey), so the dot you see on top is the dot the
    // pointer selects. Stable: equal keys keep MapLibre's order.
    const sceneDotsTopFirst = (features: maplibregl.MapGeoJSONFeature[]) =>
      [...features].sort(
        (a, b) =>
          (Number(b.properties?.sortKey) || 0) -
          (Number(a.properties?.sortKey) || 0),
      )
    const sceneSlugOf = (feature: maplibregl.MapGeoJSONFeature | null | undefined) =>
      typeof feature?.properties?.slug === 'string' ? feature.properties.slug : null
    // Hover: pointer cursor + tooltip + dot highlight, all imperative —
    // mousemove never re-renders React. The highlight itself is a
    // feature-state flag consumed by the paint expressions above.
    const pickTopScene = (
      features: maplibregl.MapGeoJSONFeature[] | undefined,
    ): string | null => sceneSlugOf(sceneDotsTopFirst(features ?? [])[0])

    // Tracked per map instance (fresh map each show → no stale hover).
    // Feature state lives in the style, so it is written only while the
    // style is live; a lost context discards it with the style.
    let hoveredSlug: string | null = null
    const setHoverState = (slug: string | null) => {
      if (slug === hoveredSlug) return
      if (health.styleLive()) {
        if (hoveredSlug !== null) {
          map.removeFeatureState({ source: 'scenes', id: hoveredSlug }, 'hover')
        }
        if (slug !== null) {
          map.setFeatureState({ source: 'scenes', id: slug }, { hover: true })
        }
      }
      hoveredSlug = slug
    }

    const handleMove = (
      e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] },
    ) => {
      if (coarsePointerRef.current) return
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
      // textContent, not innerHTML — contributor-editable city/state can't
      // inject markup (the old canvas needed an escapeHtml for this).
      tooltip.textContent = sceneTooltipLabel(scene)
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
    // A map-wide click resolved through circleMarkAt rather than a layer
    // click, so a coarse pointer's tap reaches a dot within 12px of its edge.
    const handleClick = (e: maplibregl.MapMouseEvent) => {
      const slug = sceneSlugOf(
        circleMarkAt(
          map,
          e.point,
          'scene-dots',
          coarsePointerRef.current,
          sceneDotsTopFirst,
        ),
      )
      const scene = slug ? scenesBySlugRef.current.get(slug) : undefined
      if (scene) onSelectRef.current(scene)
    }
    map.on('mousemove', 'scene-dots', handleMove)
    map.on('mouseleave', 'scene-dots', handleLeave)
    map.on('click', handleClick)

    // ── City view: pins, tooltip, status chip, camera reporting ────────────
    // All imperative, for the same reason the scene handlers are: hovering a
    // pin or nudging the camera must not re-render React.
    let hoveredVenueId: number | null = null
    const setVenueHoverState = (id: number | null) => {
      if (id === hoveredVenueId) return
      if (health.styleLive()) {
        if (hoveredVenueId !== null) {
          map.removeFeatureState({ source: 'venues', id: hoveredVenueId }, 'hover')
        }
        if (id !== null) {
          map.setFeatureState({ source: 'venues', id }, { hover: true })
        }
      }
      hoveredVenueId = id
    }

    const venueIdOf = (
      feature: maplibregl.MapGeoJSONFeature | null | undefined,
    ): number | null => {
      const id = feature?.properties?.id
      return typeof id === 'number' ? id : null
    }

    // The stack a click at this event opens: the stack marker's own stack when
    // the event comes from inside a marker (its button opens that stack), else
    // the stack pin `id` pins in. None without a stack handler, where a pin
    // opens as its venue. The pin tooltip and the pin click both read this.
    const stackClickedAt = (target: EventTarget | null, id: number) => {
      if (!onVenueStackSelectRef.current) return undefined
      const markerKey =
        target instanceof Element
          ? target.closest<HTMLElement>('[data-stack-key]')?.dataset.stackKey
          : undefined
      const { byKey, byVenueId } = stacksRef.current
      return (markerKey !== undefined ? byKey.get(markerKey) : undefined) ?? byVenueId.get(id)
    }

    const hideVenueTooltip = () => {
      if (venueTooltipRef.current) {
        venueTooltipRef.current.style.display = 'none'
      }
    }

    const handleVenueMove = (
      e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] },
    ) => {
      if (coarsePointerRef.current) return
      const id = venueIdOf(e.features?.[0])
      setVenueHoverState(id)
      map.getCanvas().style.cursor = id !== null ? 'pointer' : ''
      const tooltip = venueTooltipRef.current
      const venue = id !== null ? venuesByIdRef.current.get(id) : undefined
      if (!tooltip || !venue) {
        hideVenueTooltip()
        return
      }
      const lines = venuePinTooltipLines(
        venue,
        stackClickedAt(e.originalEvent.target, venue.id)?.scopeLine ?? null,
      )
      // textContent, not innerHTML — venue names are contributor-editable.
      if (venueTooltipNameRef.current) {
        venueTooltipNameRef.current.textContent = lines.title
      }
      if (venueTooltipMetaRef.current) {
        venueTooltipMetaRef.current.textContent = lines.meta ?? ''
        venueTooltipMetaRef.current.hidden = lines.meta === null
      }
      if (venueTooltipCtaRef.current) {
        venueTooltipCtaRef.current.hidden = !lines.showsCta
      }
      tooltip.style.display = 'block'
      // Same edge-flip as the scene tooltip so a pin near the frame edge
      // doesn't get its tooltip clipped by the overflow-hidden wrap.
      const bounds = map.getContainer().getBoundingClientRect()
      const flipX = e.point.x + 12 + tooltip.offsetWidth > bounds.width
      const flipY = e.point.y + 12 + tooltip.offsetHeight > bounds.height
      tooltip.style.left = `${flipX ? e.point.x - 12 - tooltip.offsetWidth : e.point.x + 12}px`
      tooltip.style.top = `${flipY ? e.point.y - 12 - tooltip.offsetHeight : e.point.y + 12}px`
    }
    const handleVenueLeave = () => {
      setVenueHoverState(null)
      map.getCanvas().style.cursor = ''
      hideVenueTooltip()
    }
    // Map-wide, through circleMarkAt, for the same reason as the dot click.
    const handleVenueClick = (e: maplibregl.MapMouseEvent) => {
      const id = venueIdOf(
        circleMarkAt(map, e.point, 'venue-pins', coarsePointerRef.current),
      )
      if (id === null) return
      const stack = stackClickedAt(e.originalEvent.target, id)
      if (stack) {
        onVenueStackSelectRef.current?.(stack.key)
      } else {
        onVenueSelectRef.current?.(id)
      }
    }
    map.on('mousemove', 'venue-pins', handleVenueMove)
    map.on('mouseleave', 'venue-pins', handleVenueLeave)
    map.on('click', handleVenueClick)
    // Seam for the non-pointer paths that can invalidate a hover (a rail
    // filter deleting the hovered pin). One function owns the teardown.
    clearVenueHoverRef.current = handleVenueLeave
    const clearPointerHover = () => {
      handleLeave()
      handleVenueLeave()
    }
    clearPointerHoverRef.current = clearPointerHover
    // The hovered dot and pin go with the lost style: their tooltips and the
    // pointer cursor clear at the loss, and the hover bookkeeping resets so
    // the restored style starts with nothing hovered. No feature state is
    // written (the health watch, registered earlier, reports the style down).
    map.on('webglcontextlost', clearPointerHover)
    // A stationary pointer with a moving camera would strand a hovered pin +
    // floating tooltip — same guard the scene layer needs.
    map.on('movestart', handleVenueLeave)

    // Status chip: city, live zoom, and how many pins are actually in frame.
    // Painted from the map's own state on every move, never from React —
    // a per-frame setState here would re-render the whole Atlas while panning.
    const updateStatusChip = () => {
      const chip = statusChipRef.current
      if (!chip) return
      const label = cityLabelRef.current
      if (!label) {
        chip.style.display = 'none'
        return
      }
      const bounds = map.getBounds()
      let inView = 0
      for (const v of venuesByIdRef.current.values()) {
        if (bounds.contains([v.lng, v.lat])) inView++
      }
      chip.textContent = `${label.toUpperCase()} · z${map.getZoom().toFixed(1)} · ${inView} ${
        inView === 1 ? 'venue' : 'venues'
      } in view`
      chip.style.display = 'block'
    }

    // Camera reporting for the city-view resolver. Settle events only
    // (moveend/zoomend), plus one shot on style load so a saved street-zoom
    // camera re-engages city view after a nav-away/back.
    const reportCamera = () => {
      const center = map.getCenter()
      const bounds = map.getBounds()
      onCameraSettleRef.current?.({
        lng: center.lng,
        lat: center.lat,
        zoom: map.getZoom(),
        bounds: {
          west: bounds.getWest(),
          south: bounds.getSouth(),
          east: bounds.getEast(),
          north: bounds.getNorth(),
        },
      })
    }
    map.on('moveend', reportCamera)
    map.on('zoomend', reportCamera)
    map.on('move', updateStatusChip)
    // The chip also has to repaint when the label or the pin set changes with
    // the camera standing still (a filter chip, a city hand-off). Exposed as a
    // seam rather than duplicating the paint: one function owns the text.
    redrawStatusChipRef.current = updateStatusChip

    // One-shot on style load. Registered HERE, below the two functions it
    // calls, rather than folded into the health watch's style callback above:
    // that one is declared before them, and reading a `const` from an earlier
    // closure is a temporal-dead-zone trap waiting for the day the event fires
    // synchronously.
    // A saved camera (atlasCamera.ts) can reopen the map already at
    // street zoom, and without this shot no settle event would ever fire to
    // re-engage city view.
    map.once('style.load', () => {
      reportCamera()
      updateStatusChip()
    })

    return () => {
      // Save the camera so nav-back reopens where the user left off — the
      // map itself is NOT reused (fresh instance every show; see doc above).
      saveAtlasCamera({
        center: [map.getCenter().lng, map.getCenter().lat],
        zoom: map.getZoom(),
      })
      // The preview camera's state beside the saved centre, for the next map.
      // The preview camera effect's cleanup has already run (it is declared
      // earlier), so no move of its is pending.
      const applied = appliedPreviewCameraRef.current
      leftPreviewCameraRef.current = {
        paddingPx: map.getPadding().bottom ?? 0,
        settled:
          applied?.map === map &&
          !map.isMoving() &&
          !cameraPaddingDiffers(map, applied.paddingPx),
        slug: applied?.map === map ? applied.slug : null,
        sheetPx: applied?.map === map ? applied.sheetPx : 0,
      }
      if (w.__atlasMap === map) {
        w.__atlasMap = null
        w.__atlasMapLoaded = false
      }
      if (flyToRef) flyToRef.current = null
      redrawStatusChipRef.current = null
      clearVenueHoverRef.current = null
      clearPointerHoverRef.current = null
      setMapReady((prev) => (prev === map ? null : prev))
      setMapLoaded((prev) => (prev === map ? null : prev))
      cancelAnimationFrame(paddingFrame)
      health.stop()
      map.remove()
    }
    // pov is resolved once before this canvas mounts, flyToRef is a stable
    // ref container from AtlasGlobe, theme keys this component (a new theme
    // is a new instance, and ramp is the theme's) and leftPreviewCameraRef is GlobeCanvas's ref: all
    // are identity-stable for the canvas's lifetime; everything else this
    // effect reads is a ref or setter, so the camera never re-aims on data
    // re-renders.
  }, [pov, flyToRef, theme, ramp, leftPreviewCameraRef])

  if (unrecoverable) throw unrecoverable

  return (
    <div
      ref={globeRootRef}
      className="relative overflow-hidden"
      data-testid="globe-cursor-wrap"
      data-atlas-credit={attributionPosition === 'top-left' ? 'top' : undefined}
      style={
        attributionPosition === 'top-left'
          ? ({
              width,
              height,
              // Read by the top-left credit rule in globals.css: the credit
              // sits below the status row this component draws.
              '--atlas-top-credit-offset': `${ATLAS_TOP_CREDIT_OFFSET_PX}px`,
            } as React.CSSProperties)
          : { width, height }
      }
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
        data-testid="atlas-scene-tooltip"
        className="pointer-events-none absolute z-10 rounded border border-border bg-background/90 px-2 py-1 text-xs text-foreground backdrop-blur"
        style={{ display: 'none' }}
      />
      {/* City-view status row: the optional back-to-globe control, then the
          status chip (PSY-1539), top-left of the map pane. Its geometry (the
          top-4 inset and the size-9 control) is what ATLAS_TOP_CREDIT_OFFSET_PX
          adds up; a top-left attribution control sits below the row.
          The live region is the always-rendered WRAPPER, not the pill: a
          region that appears at the same moment its text does may never be
          tracked, so the announcement is lost. The pill inside is what
          shows and hides. */}
      <div className="pointer-events-none absolute left-4 top-4 z-10 flex items-center gap-2">
        {cityViewActive && onBackToGlobe && (
          <button
            type="button"
            onClick={onBackToGlobe}
            aria-label="Back to globe"
            data-testid="atlas-back-to-globe"
            className="pointer-events-auto flex size-9 shrink-0 items-center justify-center rounded border border-border bg-background/90 text-lg leading-none text-foreground backdrop-blur transition-colors hover:border-primary hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <span aria-hidden="true">‹</span>
          </button>
        )}
        <div aria-live="polite">
          <div
            ref={statusChipRef}
            data-testid="atlas-status-chip"
            className="rounded border border-border bg-background/90 px-2.5 py-1 font-mono text-[11px] tracking-wide text-muted-foreground backdrop-blur"
            style={{ display: 'none' }}
          />
        </div>
      </div>
      {/* Venue hover tooltip: a title, a meta line and the static CTA, the
          last two hidden when a click there opens a stack. The title and meta
          line are written as textContent, since venue names are
          contributor-editable, so no innerHTML anywhere here. */}
      <div
        ref={venueTooltipRef}
        data-testid="atlas-venue-tooltip"
        className="pointer-events-none absolute z-10 rounded border border-border bg-background/95 px-2.5 py-1.5 backdrop-blur"
        style={{ display: 'none' }}
      >
        <div ref={venueTooltipNameRef} className="text-sm text-foreground" />
        <div
          ref={venueTooltipMetaRef}
          className="font-mono text-[11px] text-muted-foreground"
        />
        <div ref={venueTooltipCtaRef} className="font-mono text-[11px] text-primary">
          click for shows →
        </div>
      </div>
    </div>
  )
}
