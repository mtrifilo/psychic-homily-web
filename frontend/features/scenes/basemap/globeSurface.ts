// The Atlas globe's surface at globe zooms, in one of two looks:
//
// - full: the NASA GIBS night-earth raster (nightEarthRaster.ts), a few dozen
//   PNG tiles for one globe view;
// - light: a flat ocean plus Natural Earth 1:110m land polygons from one
//   small same-origin file, drawn in the dark tokens of the phone board.
//
// Both looks hand off to the street basemap the same way: every surface layer
// takes the raster's fade-out ramp and cutoff zoom from phBasemapFragment, so
// whichever look is showing dissolves into the streets across the same zooms.
//
// Switching looks is a layout-visibility change, never a style rebuild. A
// layer with `visibility: 'none'` leaves its source unused, and MapLibre
// neither fetches tiles for an unused source nor lists its attribution, so
// the hidden raster costs no request and drops the NASA credit with it.
import type {
  ExpressionSpecification,
  LayerSpecification,
  SourceSpecification,
} from 'maplibre-gl'
import { NIGHT_EARTH_SOURCE_ID, nightEarthSource } from './nightEarthRaster'

/** Layer ids of the two looks, in draw order (ocean under land). */
export const NIGHT_EARTH_LAYER_ID = 'earth'
export const GLOBE_OCEAN_LAYER_ID = 'globe-ocean'
export const GLOBE_LAND_LAYER_ID = 'globe-land'

/** The light look's land source id. */
export const GLOBE_LAND_SOURCE_ID = 'globeLand'

/**
 * Same-origin land polygons for the light look, built by
 * scripts/atlas-globe-land.mjs from Natural Earth 1:110m land (public domain;
 * Natural Earth's terms waive credit, so the source carries no attribution).
 * Same-origin, so the CSP's `connect-src 'self'` already admits it.
 */
export const GLOBE_LAND_DATA_URL = '/atlas/globe-land-110m.geojson'

/**
 * The `basemap_host` basemapTelemetry reports for a land-source failure that
 * carries no absolute URL: the file is served by the app itself.
 */
export const GLOBE_LAND_HOST = 'same-origin'

/**
 * The light look's palette, from the phone board (Figma Product Designs,
 * Atlas page, board 01 "Globe entry (lighter globe)"): sphere `#0A0C18`, land
 * `#111528`. Fixed rather than themed: the map stays dark in light mode, as
 * the raster and the street basemap do.
 */
export const GLOBE_OCEAN_COLOR = '#0a0c18'
export const GLOBE_LAND_COLOR = '#111528'

type Visibility = 'visible' | 'none'

/** Which look's layers are visible: the light look hides the raster. */
export function globeSurfaceVisibility(
  lightGlobe: boolean,
): Record<string, Visibility> {
  const light: Visibility = lightGlobe ? 'visible' : 'none'
  const full: Visibility = lightGlobe ? 'none' : 'visible'
  return {
    [GLOBE_OCEAN_LAYER_ID]: light,
    [GLOBE_LAND_LAYER_ID]: light,
    [NIGHT_EARTH_LAYER_ID]: full,
  }
}

/**
 * The surface sources. The land source starts EMPTY: a GeoJSON source
 * fetches its data as soon as it is added, visible or not, so
 * {@link showGlobeSurface} loads the land data the first time a map shows the
 * light look.
 */
export function globeSurfaceSources(): Record<string, SourceSpecification> {
  return {
    [NIGHT_EARTH_SOURCE_ID]: nightEarthSource(),
    [GLOBE_LAND_SOURCE_ID]: {
      type: 'geojson',
      data: { type: 'FeatureCollection', features: [] },
    },
  }
}

/**
 * The surface layers, ocean then land then raster. All three share the
 * handoff: `fadeOut` is phBasemapFragment's `rasterFadeOut` and `maxZoom` its
 * `rasterMaxZoom`, so neither look outlives the crossfade into the streets.
 *
 * The ocean is a background layer: on the globe projection MapLibre draws a
 * background on the sphere only, so space around the globe stays transparent
 * for the CSS starfield behind the canvas.
 */
export function globeSurfaceLayers({
  lightGlobe,
  fadeOut,
  maxZoom,
}: {
  lightGlobe: boolean
  fadeOut: ExpressionSpecification
  maxZoom: number
}): LayerSpecification[] {
  const visibility = globeSurfaceVisibility(lightGlobe)
  return [
    {
      id: GLOBE_OCEAN_LAYER_ID,
      type: 'background',
      maxzoom: maxZoom,
      layout: { visibility: visibility[GLOBE_OCEAN_LAYER_ID] },
      paint: {
        'background-color': GLOBE_OCEAN_COLOR,
        'background-opacity': fadeOut,
      },
    },
    {
      id: GLOBE_LAND_LAYER_ID,
      type: 'fill',
      source: GLOBE_LAND_SOURCE_ID,
      maxzoom: maxZoom,
      layout: { visibility: visibility[GLOBE_LAND_LAYER_ID] },
      paint: {
        'fill-color': GLOBE_LAND_COLOR,
        'fill-opacity': fadeOut,
      },
    },
    {
      id: NIGHT_EARTH_LAYER_ID,
      type: 'raster',
      source: NIGHT_EARTH_SOURCE_ID,
      maxzoom: maxZoom,
      layout: { visibility: visibility[NIGHT_EARTH_LAYER_ID] },
      paint: { 'raster-opacity': fadeOut },
    },
  ]
}

/** The slice of a MapLibre map {@link showGlobeSurface} drives. */
export interface GlobeSurfaceMap {
  getLayer(id: string): unknown
  setLayoutProperty(layer: string, name: 'visibility', value: Visibility): unknown
  getSource(id: string): unknown
}

// Maps whose land source already holds (or is fetching) the land data. Each
// Atlas show builds a fresh map, so entries go with the map they belong to.
const landRequested = new WeakSet<object>()

// The land file fetched on the main thread ahead of the map, once per page
// load. Resolves to the parsed collection, or null after a failed or unusable
// response (the map then fetches the file itself).
let landPrefetch: Promise<GeoJSON.FeatureCollection | null> | null = null
// Whether the prefetch has settled, and whether a map has given up waiting on
// it. Once a wait has run out, later maps stop waiting on a still-unsettled
// prefetch and fetch the file themselves straight away; a prefetch that
// settles after that is used again from then on.
let landPrefetchSettled = false
let landPrefetchWaitExpired = false

// How long a map waits on an unfinished prefetch before fetching the file
// itself. The clock starts when the map starts waiting. The prefetch is not
// cancelled: a map whose wait runs out pays for a second download, and the
// prefetch, once it lands, still serves the maps built after it.
const LAND_PREFETCH_WAIT_MS = 10_000
const WAIT_EXPIRED = Symbol('wait expired')

/**
 * Starts fetching the land file before any map exists, so a map that shows
 * the light look takes the parsed data instead of fetching it from its worker
 * after the style loads. Idempotent. After a failed or unusable response the
 * map fetches the URL itself, where a failure reaches basemapTelemetry like
 * any source error.
 */
export function prefetchGlobeLand(): void {
  if (landPrefetch) return
  landPrefetch = fetch(GLOBE_LAND_DATA_URL)
    .then((response) => (response.ok ? response.json() : null))
    .then((data: unknown) =>
      (data as GeoJSON.FeatureCollection | null)?.type === 'FeatureCollection'
        ? (data as GeoJSON.FeatureCollection)
        : null,
    )
    .catch(() => null)
    .finally(() => {
      landPrefetchSettled = true
    })
}

type LandSource = { setData(data: string | GeoJSON.FeatureCollection): unknown }

/**
 * Switches a live map to one look. Idempotent; loads the land data at most
 * once per map, on the first switch to the light look: the prefetch's result
 * when a prefetch was started (waiting up to LAND_PREFETCH_WAIT_MS for it if
 * it is still in flight rather than starting a second download), else the
 * file's URL, fetched by the map.
 */
export function showGlobeSurface(map: GlobeSurfaceMap, lightGlobe: boolean): void {
  if (lightGlobe && !landRequested.has(map)) {
    const source = map.getSource(GLOBE_LAND_SOURCE_ID) as LandSource | undefined
    if (source) {
      landRequested.add(map)
      if (landPrefetch && (landPrefetchSettled || !landPrefetchWaitExpired)) {
        const waited = new Promise<typeof WAIT_EXPIRED>((resolve) =>
          setTimeout(() => resolve(WAIT_EXPIRED), LAND_PREFETCH_WAIT_MS),
        )
        void Promise.race([landPrefetch, waited]).then((result) => {
          if (result === WAIT_EXPIRED) landPrefetchWaitExpired = true
          const data = result === WAIT_EXPIRED ? null : result
          // A map removed while the prefetch was in flight no longer owns this
          // source (a removed map has no style, so getSource answers
          // undefined); data sent to it would reach the worker for a map that
          // no longer exists.
          if (map.getSource(GLOBE_LAND_SOURCE_ID) !== source) return
          source.setData(data ?? GLOBE_LAND_DATA_URL)
        })
      } else {
        source.setData(GLOBE_LAND_DATA_URL)
      }
    }
  }
  for (const [layer, visibility] of Object.entries(
    globeSurfaceVisibility(lightGlobe),
  )) {
    if (map.getLayer(layer)) {
      map.setLayoutProperty(layer, 'visibility', visibility)
    }
  }
}
