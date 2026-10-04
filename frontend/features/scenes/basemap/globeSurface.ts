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
 * {@link showGlobeSurface} points it at the file the first time a map shows
 * the light look.
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
// load: the parsed collection once it lands, null while in flight or after a
// failure (the map then fetches the file itself).
let landPrefetchStarted = false
let prefetchedLand: GeoJSON.FeatureCollection | null = null

/**
 * Starts fetching the land file before any map exists, so a map that shows
 * the light look can take the parsed data instead of fetching it from its
 * worker after the style loads. Idempotent. A failed or unusable response
 * leaves nothing prefetched, and the map falls back to fetching the URL,
 * where a failure reaches basemapTelemetry like any source error.
 */
export function prefetchGlobeLand(): void {
  if (landPrefetchStarted) return
  landPrefetchStarted = true
  fetch(GLOBE_LAND_DATA_URL)
    .then((response) => (response.ok ? response.json() : null))
    .then((data: unknown) => {
      if (isFeatureCollection(data)) prefetchedLand = data
    })
    .catch(() => {})
}

function isFeatureCollection(data: unknown): data is GeoJSON.FeatureCollection {
  return (
    typeof data === 'object' &&
    data !== null &&
    (data as { type?: unknown }).type === 'FeatureCollection' &&
    Array.isArray((data as { features?: unknown }).features)
  )
}

/**
 * Switches a live map to one look. Idempotent; loads the land data at most
 * once per map, on the first switch to the light look: the prefetched
 * collection when it has landed, else the file's URL (fetched by the map).
 */
export function showGlobeSurface(map: GlobeSurfaceMap, lightGlobe: boolean): void {
  if (lightGlobe && !landRequested.has(map)) {
    const source = map.getSource(GLOBE_LAND_SOURCE_ID) as
      | { setData(data: string | GeoJSON.FeatureCollection): unknown }
      | undefined
    if (source) {
      source.setData(prefetchedLand ?? GLOBE_LAND_DATA_URL)
      landRequested.add(map)
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
