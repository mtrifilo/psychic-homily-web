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

/**
 * One light-look data file and its main-thread prefetch, once per page load.
 * `prefetch` resolves to the parsed collection, or null after a failed or
 * unusable response (the consumer then fetches the URL itself). Once a
 * consumer's wait on an unsettled prefetch has run out (`waitExpired`),
 * later consumers stop waiting on it and fetch the URL straight away; a
 * prefetch that settles after that is used again from then on.
 */
interface SurfaceFile {
  readonly url: string
  prefetch: Promise<GeoJSON.FeatureCollection | null> | null
  settled: boolean
  waitExpired: boolean
}

const surfaceFile = (url: string): SurfaceFile => ({
  url,
  prefetch: null,
  settled: false,
  waitExpired: false,
})

const LAND_FILE = surfaceFile(GLOBE_LAND_DATA_URL)
const COUNTRY_LINES_FILE = surfaceFile(GLOBE_COUNTRY_LINES_DATA_URL)
const STATE_LINES_FILE = surfaceFile(GLOBE_STATE_LINES_DATA_URL)
const PLACES_FILE = surfaceFile(GLOBE_PLACES_DATA_URL)

/** The map sources the light look fills, each from its own file, land first. */
const SOURCE_FILES: ReadonlyArray<readonly [string, SurfaceFile]> = [
  [GLOBE_LAND_SOURCE_ID, LAND_FILE],
  [GLOBE_STATE_LINES_SOURCE_ID, STATE_LINES_FILE],
  [GLOBE_COUNTRY_LINES_SOURCE_ID, COUNTRY_LINES_FILE],
]

// How long a consumer waits on an unfinished prefetch before fetching the
// file itself. The clock starts when the consumer starts waiting. The
// prefetch is not cancelled: a consumer whose wait runs out pays for a second
// download, and the prefetch, once it lands, still serves later consumers.
const PREFETCH_WAIT_MS = 10_000
const WAIT_EXPIRED = Symbol('wait expired')

function fetchCollection(url: string): Promise<GeoJSON.FeatureCollection | null> {
  return fetch(url)
    .then((response) => (response.ok ? response.json() : null))
    .then((data: unknown) =>
      (data as GeoJSON.FeatureCollection | null)?.type === 'FeatureCollection'
        ? (data as GeoJSON.FeatureCollection)
        : null,
    )
    .catch(() => null)
}

function startPrefetch(file: SurfaceFile, after?: Promise<unknown>): void {
  if (file.prefetch) return
  const fetched = after
    ? after.then(() => fetchCollection(file.url))
    : fetchCollection(file.url)
  file.prefetch = fetched.finally(() => {
    file.settled = true
  })
}

/**
 * Starts fetching the light look's files before any map exists, so a map
 * that shows the light look takes parsed data instead of fetching each file
 * from its worker after the style loads. The land file goes first; the line
 * and place files start once it has settled, so at entry they never share
 * bandwidth with it. Idempotent. After a failed or unusable response the
 * consumer fetches the URL itself, where a map source's failure reaches
 * basemapTelemetry like any source error.
 */
export function prefetchGlobeSurface(): void {
  startPrefetch(LAND_FILE)
  const afterLand = LAND_FILE.prefetch ?? undefined
  for (const file of [STATE_LINES_FILE, COUNTRY_LINES_FILE, PLACES_FILE]) {
    startPrefetch(file, afterLand)
  }
}

/** Whether a consumer should wait on the file's prefetch at all. */
const worthWaiting = (file: SurfaceFile) =>
  file.prefetch !== null && (file.settled || !file.waitExpired)

/**
 * The prefetched collection, waiting up to PREFETCH_WAIT_MS for a prefetch
 * still in flight; the file's URL when there is no usable prefetch.
 */
function prefetchedOrUrl(file: SurfaceFile): Promise<GeoJSON.FeatureCollection | string> {
  if (!worthWaiting(file) || !file.prefetch) return Promise.resolve(file.url)
  const waited = new Promise<typeof WAIT_EXPIRED>((resolve) =>
    setTimeout(() => resolve(WAIT_EXPIRED), PREFETCH_WAIT_MS),
  )
  return Promise.race([file.prefetch, waited]).then((result) => {
    if (result === WAIT_EXPIRED) {
      file.waitExpired = true
      return file.url
    }
    return result ?? file.url
  })
}

let placesData: Promise<GeoJSON.FeatureCollection | null> | null = null

/**
 * The place-label data (globePlaces.ts parses it): the prefetched collection
 * when a prefetch was started, else a fetch of the file. Shared by every map
 * in the page load; after a failure the next call fetches again.
 */
export function loadGlobePlaces(): Promise<GeoJSON.FeatureCollection | null> {
  if (!placesData) {
    const loading = prefetchedOrUrl(PLACES_FILE).then((data) =>
      typeof data === 'string' ? fetchCollection(data) : data,
    )
    placesData = loading
    void loading.then((data) => {
      if (!data && placesData === loading) placesData = null
    })
  }
  return placesData
}

type DataSource = { setData(data: string | GeoJSON.FeatureCollection): unknown }

// Per map, the light-look sources already holding (or fetching) their data.
// Each Atlas show builds a fresh map, so entries go with the map they belong to.
const sourcesRequested = new WeakMap<object, Set<string>>()

function loadSourceData(
  map: GlobeSurfaceMap,
  sourceId: string,
  source: DataSource,
  file: SurfaceFile,
): void {
  if (!worthWaiting(file)) {
    source.setData(file.url)
    return
  }
  void prefetchedOrUrl(file).then((data) => {
    // A map removed while the prefetch was in flight no longer owns this
    // source (a removed map has no style, so getSource answers undefined);
    // data sent to it would reach the worker for a map that no longer exists.
    if (map.getSource(sourceId) !== source) return
    source.setData(data)
  })
}

/**
 * Switches a live map to one look. Idempotent; loads each light-look source's
 * data at most once per map, on the first switch to the light look: the
 * prefetch's result when a prefetch was started (waiting up to
 * PREFETCH_WAIT_MS for it if it is still in flight rather than starting a
 * second download), else the file's URL, fetched by the map. Each source
 * loads independently, so one file failing leaves the others drawn.
 */
export function showGlobeSurface(map: GlobeSurfaceMap, lightGlobe: boolean): void {
  if (lightGlobe) {
    let requested = sourcesRequested.get(map)
    if (!requested) {
      requested = new Set()
      sourcesRequested.set(map, requested)
    }
    for (const [sourceId, file] of SOURCE_FILES) {
      if (requested.has(sourceId)) continue
      const source = map.getSource(sourceId) as DataSource | undefined
      if (!source) continue
      requested.add(sourceId)
      loadSourceData(map, sourceId, source, file)
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
