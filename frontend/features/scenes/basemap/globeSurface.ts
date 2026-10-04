// The Atlas globe's surface at globe zooms, in one of two looks:
//
// - full: the NASA GIBS night-earth raster (nightEarthRaster.ts), a few dozen
//   PNG tiles for one globe view;
// - light: a flat ocean, Natural Earth 1:110m land polygons, and hairline
//   country (1:110m) and state or province (1:50m) boundary lines, each from
//   one small same-origin file, drawn in the dark tokens of the phone board.
//   The light look's place labels are DOM markers (globePlaces.ts); their data
//   file loads through this module with the others.
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

/** Layer ids of the two looks, in draw order (ocean under land under lines). */
export const NIGHT_EARTH_LAYER_ID = 'earth'
export const GLOBE_OCEAN_LAYER_ID = 'globe-ocean'
export const GLOBE_LAND_LAYER_ID = 'globe-land'
export const GLOBE_STATE_LINES_LAYER_ID = 'globe-state-lines'
export const GLOBE_COUNTRY_LINES_LAYER_ID = 'globe-country-lines'

/** The light look's source ids. */
export const GLOBE_LAND_SOURCE_ID = 'globeLand'
export const GLOBE_STATE_LINES_SOURCE_ID = 'globeStateLines'
export const GLOBE_COUNTRY_LINES_SOURCE_ID = 'globeCountryLines'

/**
 * Same-origin data for the light look, built from Natural Earth (public
 * domain; Natural Earth's terms waive credit, so no source carries an
 * attribution): the land by scripts/atlas-globe-land.mjs, the lines and the
 * places by scripts/atlas-globe-overlays.mjs. Same-origin, so the CSP's
 * `connect-src 'self'` already admits them.
 */
export const GLOBE_LAND_DATA_URL = '/atlas/globe-land-110m.geojson'
export const GLOBE_COUNTRY_LINES_DATA_URL = '/atlas/globe-country-lines-110m.geojson'
export const GLOBE_STATE_LINES_DATA_URL = '/atlas/globe-state-lines-50m.geojson'
export const GLOBE_PLACES_DATA_URL = '/atlas/globe-places-110m.geojson'

/**
 * The `basemap_host` basemapTelemetry reports for a failure of one of these
 * files that carries no absolute URL: the app serves them itself.
 */
export const GLOBE_DATA_HOST = 'same-origin'

/**
 * The light look's palette, from the phone board (Figma Product Designs,
 * Atlas page, board 01 "Globe entry (lighter globe)"): sphere `#0A0C18`, land
 * `#111528`, boundary hairlines `#3A3F5C` at 0.75px, place labels `#7A7F9A`.
 * Literal values rather than DS tokens, the same in both themes: the globe
 * surface stays dark in light mode, as the raster and the street basemap do,
 * and the themed border and muted tokens are drawn for the app's paper and
 * ink grounds, not for this one.
 */
export const GLOBE_OCEAN_COLOR = '#0a0c18'
export const GLOBE_LAND_COLOR = '#111528'
export const GLOBE_BOUNDARY_COLOR = '#3a3f5c'
export const GLOBE_BOUNDARY_WIDTH_PX = 0.75
export const GLOBE_PLACE_LABEL_COLOR = '#7a7f9a'

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
    [GLOBE_STATE_LINES_LAYER_ID]: light,
    [GLOBE_COUNTRY_LINES_LAYER_ID]: light,
    [NIGHT_EARTH_LAYER_ID]: full,
  }
}

const emptyGeoJson = (): SourceSpecification => ({
  type: 'geojson',
  data: { type: 'FeatureCollection', features: [] },
})

/**
 * The surface sources. The light look's sources start EMPTY: a GeoJSON
 * source fetches its data as soon as it is added, visible or not, so
 * {@link showGlobeSurface} loads their data the first time a map shows the
 * light look.
 */
export function globeSurfaceSources(): Record<string, SourceSpecification> {
  return {
    [NIGHT_EARTH_SOURCE_ID]: nightEarthSource(),
    [GLOBE_LAND_SOURCE_ID]: emptyGeoJson(),
    [GLOBE_STATE_LINES_SOURCE_ID]: emptyGeoJson(),
    [GLOBE_COUNTRY_LINES_SOURCE_ID]: emptyGeoJson(),
  }
}

/**
 * The surface layers: ocean, land, state lines, country lines, then raster.
 * All share the handoff: `fadeOut` is phBasemapFragment's `rasterFadeOut`
 * and `maxZoom` its `rasterMaxZoom`, so no part of either look outlives the
 * crossfade into the streets.
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
  const boundaryLines = (id: string, source: string): LayerSpecification => ({
    id,
    type: 'line',
    source,
    maxzoom: maxZoom,
    layout: { visibility: visibility[id] },
    paint: {
      'line-color': GLOBE_BOUNDARY_COLOR,
      'line-width': GLOBE_BOUNDARY_WIDTH_PX,
      'line-opacity': fadeOut,
    },
  })
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
    boundaryLines(GLOBE_STATE_LINES_LAYER_ID, GLOBE_STATE_LINES_SOURCE_ID),
    boundaryLines(GLOBE_COUNTRY_LINES_LAYER_ID, GLOBE_COUNTRY_LINES_SOURCE_ID),
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

/** The ids of the map sources that fetch a same-origin light-look file. */
export const GLOBE_DATA_SOURCE_IDS: readonly string[] = [
  GLOBE_LAND_SOURCE_ID,
  GLOBE_STATE_LINES_SOURCE_ID,
  GLOBE_COUNTRY_LINES_SOURCE_ID,
]

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

/**
 * A fetched collection, or why there is none: `status` is the HTTP status of
 * a failed response, 0 for a network failure, and undefined for a body that
 * is not a FeatureCollection.
 */
export type CollectionResult =
  | { data: GeoJSON.FeatureCollection }
  | { data: null; status: number | undefined }

function fetchCollection(url: string): Promise<CollectionResult> {
  return fetch(url).then(
    (response): Promise<CollectionResult> | CollectionResult =>
      response.ok
        ? response.json().then(
            (body: unknown): CollectionResult =>
              (body as GeoJSON.FeatureCollection | null)?.type === 'FeatureCollection' &&
              Array.isArray((body as GeoJSON.FeatureCollection).features)
                ? { data: body as GeoJSON.FeatureCollection }
                : { data: null, status: undefined },
            (): CollectionResult => ({ data: null, status: undefined }),
          )
        : { data: null, status: response.status },
    (): CollectionResult => ({ data: null, status: 0 }),
  )
}

// The place-label file (globePlaces.ts parses it), loaded once per page load
// and shared by every map; cleared after a failure so the next load fetches
// it again.
let placesLoad: Promise<CollectionResult> | null = null

/**
 * The place-label data, fetched on first use with one retry after a failure.
 * Called when a map first shows the light look, so the request starts after
 * the map's style has loaded and never competes with the map's own code. The
 * caller reports a result without data.
 */
export function loadGlobePlaces(): Promise<CollectionResult> {
  if (!placesLoad) {
    const loading = fetchCollection(GLOBE_PLACES_DATA_URL).then((result) =>
      result.data ? result : fetchCollection(GLOBE_PLACES_DATA_URL),
    )
    placesLoad = loading
    void loading.then((result) => {
      if (!result.data && placesLoad === loading) placesLoad = null
    })
  }
  return placesLoad
}

type DataSource = { setData(data: string | GeoJSON.FeatureCollection): unknown }

/** The boundary line sources and their files, state then country. */
const BOUNDARY_FILES: ReadonlyArray<readonly [string, string]> = [
  [GLOBE_STATE_LINES_SOURCE_ID, GLOBE_STATE_LINES_DATA_URL],
  [GLOBE_COUNTRY_LINES_SOURCE_ID, GLOBE_COUNTRY_LINES_DATA_URL],
]

// Per map, the boundary sources already holding (or fetching) their data.
const boundariesRequested = new WeakMap<object, Set<string>>()

/**
 * Loads each boundary source's data once per map. The file is fetched on the
 * main thread and handed over only once it has arrived, so the source stays
 * loaded (its empty collection) until then and the map's first frame never
 * waits on it. A failed or unusable response hands the map the URL instead,
 * where a second failure reaches basemapTelemetry like any source error. Each
 * file loads on its own, so one failing leaves the other drawn.
 */
function loadBoundaries(map: GlobeSurfaceMap): void {
  let requested = boundariesRequested.get(map)
  if (!requested) {
    requested = new Set()
    boundariesRequested.set(map, requested)
  }
  for (const [sourceId, url] of BOUNDARY_FILES) {
    if (requested.has(sourceId)) continue
    const source = map.getSource(sourceId) as DataSource | undefined
    if (!source) continue
    requested.add(sourceId)
    void fetchCollection(url).then((result) => {
      // A removed map no longer owns this source (see showGlobeSurface).
      if (map.getSource(sourceId) !== source) return
      source.setData(result.data ?? url)
    })
  }
}

/**
 * Switches a live map to one look. Idempotent; on the first switch to the
 * light look it loads each light-look source's data once per map: the land
 * from the prefetch's result when a prefetch was started (waiting up to
 * LAND_PREFETCH_WAIT_MS for it if it is still in flight rather than starting
 * a second download), else from the file's URL, fetched by the map; then the
 * boundary lines (loadBoundaries).
 */
export function showGlobeSurface(map: GlobeSurfaceMap, lightGlobe: boolean): void {
  if (lightGlobe && !landRequested.has(map)) {
    const source = map.getSource(GLOBE_LAND_SOURCE_ID) as DataSource | undefined
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
  if (lightGlobe) loadBoundaries(map)
  for (const [layer, visibility] of Object.entries(
    globeSurfaceVisibility(lightGlobe),
  )) {
    if (map.getLayer(layer)) {
      map.setLayoutProperty(layer, 'visibility', visibility)
    }
  }
}
