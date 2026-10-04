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
 * One light-look map source's data file and its main-thread prefetch, once
 * per page load.
 * `prefetch` resolves to the parsed collection, or null after a failed or
 * unusable response (the map then fetches the URL itself). Once a
 * map's wait on an unsettled prefetch has run out (`waitExpired`),
 * later maps stop waiting on it and fetch the URL straight away; a
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

/** The map sources the light look fills, each from its own file, land first. */
const SOURCE_FILES: ReadonlyArray<readonly [string, SurfaceFile]> = [
  [GLOBE_LAND_SOURCE_ID, LAND_FILE],
  [GLOBE_STATE_LINES_SOURCE_ID, STATE_LINES_FILE],
  [GLOBE_COUNTRY_LINES_SOURCE_ID, COUNTRY_LINES_FILE],
]

/** The ids of the map sources that fetch a same-origin light-look file. */
export const GLOBE_DATA_SOURCE_IDS: readonly string[] = SOURCE_FILES.map(([id]) => id)

// How long a consumer waits on an unfinished prefetch before fetching the
// file itself, and the longest the line and place prefetches wait on the land
// file before starting anyway. Each wait's clock starts when that wait does.
// A prefetch is not cancelled: a map whose wait runs out pays for a second
// download unless the prefetch has not started yet (then it is skipped), and
// a prefetch that lands late still serves the maps built after it.
const PREFETCH_WAIT_MS = 10_000
const WAIT_EXPIRED = Symbol('wait expired')

/** The promise's result, or WAIT_EXPIRED once `ms` pass first. */
function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T | typeof WAIT_EXPIRED> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const expired = new Promise<typeof WAIT_EXPIRED>((resolve) => {
    timer = setTimeout(() => resolve(WAIT_EXPIRED), ms)
  })
  return Promise.race([promise, expired]).finally(() => clearTimeout(timer))
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
              (body as GeoJSON.FeatureCollection | null)?.type === 'FeatureCollection'
                ? { data: body as GeoJSON.FeatureCollection }
                : { data: null, status: undefined },
            (): CollectionResult => ({ data: null, status: undefined }),
          )
        : { data: null, status: response.status },
    (): CollectionResult => ({ data: null, status: 0 }),
  )
}

function startPrefetch(file: SurfaceFile, after?: Promise<unknown>): void {
  if (file.prefetch) return
  const fetchFile = () => fetchCollection(file.url).then((result) => result.data)
  const fetched = after
    ? // A map that already gave up waiting has fetched the file itself.
      after.then(() => (file.waitExpired ? null : fetchFile()))
    : fetchFile()
  file.prefetch = fetched.finally(() => {
    file.settled = true
  })
}

// The place-label file (globePlaces.ts parses it), loaded once per page load
// and shared by every map; cleared after a failure so the next load fetches
// it again.
let placesLoad: Promise<CollectionResult> | null = null

function startPlacesFetch(after?: Promise<unknown>): Promise<CollectionResult> {
  const loading = after
    ? after.then(() => fetchCollection(GLOBE_PLACES_DATA_URL))
    : fetchCollection(GLOBE_PLACES_DATA_URL)
  placesLoad = loading
  void loading.then((result) => {
    if (!result.data && placesLoad === loading) placesLoad = null
  })
  return loading
}

/**
 * Starts fetching the light look's files before any map exists, so a map
 * that shows the light look takes parsed data instead of fetching each file
 * from its worker after the style loads. The land file goes first. The line
 * and place files start once the land file has settled (or PREFETCH_WAIT_MS
 * has passed) and `mapModule`, the map's code, has loaded, so at entry they
 * share bandwidth with neither. Idempotent. After a failed or unusable
 * response a map fetches a source's URL itself, where a failure reaches
 * basemapTelemetry like any source error; the place file's failures are
 * {@link loadGlobePlaces}'s to report.
 */
export function prefetchGlobeSurface(mapModule: Promise<unknown> = Promise.resolve()): void {
  startPrefetch(LAND_FILE)
  if (STATE_LINES_FILE.prefetch && COUNTRY_LINES_FILE.prefetch && placesLoad) return
  const land: Promise<unknown> = LAND_FILE.prefetch ?? Promise.resolve()
  const overlaysReady = Promise.all([withTimeout(land, PREFETCH_WAIT_MS), mapModule])
  startPrefetch(STATE_LINES_FILE, overlaysReady)
  startPrefetch(COUNTRY_LINES_FILE, overlaysReady)
  if (!placesLoad) startPlacesFetch(overlaysReady)
}

/**
 * The place-label data: the prefetch's result, waiting up to
 * PREFETCH_WAIT_MS for one still in flight; after a failed prefetch, or a
 * wait that ran out, one direct fetch of the file. The caller reports a
 * result without data.
 */
export function loadGlobePlaces(): Promise<CollectionResult> {
  const pending = placesLoad
  if (!pending) return startPlacesFetch()
  return withTimeout(pending, PREFETCH_WAIT_MS).then((result) =>
    result !== WAIT_EXPIRED && result.data ? result : startPlacesFetch(),
  )
}

/**
 * The prefetched collection, waiting up to PREFETCH_WAIT_MS for a prefetch
 * still in flight; the file's URL after a failed prefetch or a wait that ran
 * out.
 */
function prefetchedOrUrl(
  file: SurfaceFile,
  prefetch: Promise<GeoJSON.FeatureCollection | null>,
): Promise<GeoJSON.FeatureCollection | string> {
  return withTimeout(prefetch, PREFETCH_WAIT_MS).then((result) => {
    if (result === WAIT_EXPIRED) {
      file.waitExpired = true
      return file.url
    }
    return result ?? file.url
  })
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
  const { prefetch } = file
  // No prefetch, or one still unsettled after a wait already ran out: the
  // map fetches the file itself, now.
  if (!prefetch || (!file.settled && file.waitExpired)) {
    source.setData(file.url)
    return
  }
  void prefetchedOrUrl(file, prefetch).then((data) => {
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
