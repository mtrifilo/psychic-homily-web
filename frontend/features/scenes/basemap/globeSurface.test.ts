import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { readFileSync, statSync } from 'node:fs'
import path from 'node:path'
import { validateStyleMin } from '@maplibre/maplibre-gl-style-spec'
import type {
  ExpressionSpecification,
  StyleSpecification,
} from 'maplibre-gl'
import { NIGHT_EARTH_SOURCE_ID } from './nightEarthRaster'
import {
  GLOBE_BOUNDARY_COLOR,
  GLOBE_BOUNDARY_WIDTH_PX,
  GLOBE_COUNTRY_LINES_DATA_URL,
  GLOBE_COUNTRY_LINES_LAYER_ID,
  GLOBE_COUNTRY_LINES_SOURCE_ID,
  GLOBE_LAND_DATA_URL,
  GLOBE_LAND_LAYER_ID,
  GLOBE_LAND_SOURCE_ID,
  GLOBE_OCEAN_LAYER_ID,
  GLOBE_PLACES_DATA_URL,
  GLOBE_STATE_LINES_DATA_URL,
  GLOBE_STATE_LINES_LAYER_ID,
  GLOBE_STATE_LINES_SOURCE_ID,
  NIGHT_EARTH_LAYER_ID,
  globeSurfaceLayers,
  globeSurfaceSources,
  globeSurfaceVisibility,
  showGlobeSurface,
  type GlobeSurfaceMap,
} from './globeSurface'

const FRONTEND_ROOT = path.resolve(__dirname, '../../..')
const LAND_FILE = path.join(FRONTEND_ROOT, 'public', GLOBE_LAND_DATA_URL)
const publicFile = (url: string) => path.join(FRONTEND_ROOT, 'public', url)

const FADE_OUT: ExpressionSpecification = [
  'interpolate',
  ['linear'],
  ['zoom'],
  5.5,
  1,
  7,
  0,
]
const MAX_ZOOM = 7.1

// Module state is once per page load by design, so each prefetch case gets a
// fresh module instance rather than a reset seam.
async function freshModule() {
  vi.resetModules()
  return import('./globeSurface')
}

function surfaceStyle(lightGlobe: boolean): StyleSpecification {
  return {
    version: 8,
    sources: globeSurfaceSources(),
    layers: globeSurfaceLayers({ lightGlobe, fadeOut: FADE_OUT, maxZoom: MAX_ZOOM }),
  }
}

function fakeMap(opts: { withLandSource?: boolean } = {}) {
  const setData = vi.fn()
  const stateSetData = vi.fn()
  const countrySetData = vi.fn()
  // One object per source, as MapLibre returns the same source instance on
  // every call.
  const sources: Record<string, { setData: typeof setData }> = {
    [GLOBE_STATE_LINES_SOURCE_ID]: { setData: stateSetData },
    [GLOBE_COUNTRY_LINES_SOURCE_ID]: { setData: countrySetData },
  }
  if (opts.withLandSource !== false) sources[GLOBE_LAND_SOURCE_ID] = { setData }
  const layerIds = new Set([
    GLOBE_OCEAN_LAYER_ID,
    GLOBE_LAND_LAYER_ID,
    GLOBE_STATE_LINES_LAYER_ID,
    GLOBE_COUNTRY_LINES_LAYER_ID,
    NIGHT_EARTH_LAYER_ID,
  ])
  const map = {
    getLayer: vi.fn((id: string) => (layerIds.has(id) ? { id } : undefined)),
    setLayoutProperty: vi.fn(),
    getSource: vi.fn((id: string): { setData: typeof setData } | undefined => sources[id]),
  } satisfies GlobeSurfaceMap
  const visibilityOf = (layer: string) =>
    map.setLayoutProperty.mock.calls.filter((c) => c[0] === layer).at(-1)?.[2]
  return { map, setData, stateSetData, countrySetData, visibilityOf }
}

describe('globe surface looks', () => {
  it('hides the night-earth raster on the light globe and shows ocean, land and boundaries', () => {
    expect(globeSurfaceVisibility(true)).toEqual({
      [GLOBE_OCEAN_LAYER_ID]: 'visible',
      [GLOBE_LAND_LAYER_ID]: 'visible',
      [GLOBE_STATE_LINES_LAYER_ID]: 'visible',
      [GLOBE_COUNTRY_LINES_LAYER_ID]: 'visible',
      [NIGHT_EARTH_LAYER_ID]: 'none',
    })
  })

  it('shows only the raster on the full globe, with no boundary lines', () => {
    expect(globeSurfaceVisibility(false)).toEqual({
      [GLOBE_OCEAN_LAYER_ID]: 'none',
      [GLOBE_LAND_LAYER_ID]: 'none',
      [GLOBE_STATE_LINES_LAYER_ID]: 'none',
      [GLOBE_COUNTRY_LINES_LAYER_ID]: 'none',
      [NIGHT_EARTH_LAYER_ID]: 'visible',
    })
  })

  it.each([true, false])('builds each layer with its look visibility (lightGlobe=%s)', (light) => {
    const layers = globeSurfaceLayers({ lightGlobe: light, fadeOut: FADE_OUT, maxZoom: MAX_ZOOM })
    const expected = globeSurfaceVisibility(light)
    for (const layer of layers) {
      expect(layer.layout?.visibility, layer.id).toBe(expected[layer.id])
    }
  })

  it.each([true, false])('builds a spec-valid style fragment (lightGlobe=%s)', (light) => {
    expect(validateStyleMin(surfaceStyle(light))).toEqual([])
  })

  it.each([GLOBE_LAND_SOURCE_ID, GLOBE_STATE_LINES_SOURCE_ID, GLOBE_COUNTRY_LINES_SOURCE_ID])(
    'registers %s empty, so a map that never shows the light globe never fetches it',
    (sourceId) => {
      // A GeoJSON source fetches its data when added, visible or not.
      expect(globeSurfaceSources()[sourceId]).toMatchObject({
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    },
  )

  it('draws both boundary layers as board hairlines from their own sources, country above state', () => {
    const layers = globeSurfaceLayers({ lightGlobe: true, fadeOut: FADE_OUT, maxZoom: MAX_ZOOM })
    const state = layers.find((l) => l.id === GLOBE_STATE_LINES_LAYER_ID)
    const country = layers.find((l) => l.id === GLOBE_COUNTRY_LINES_LAYER_ID)
    expect(state).toMatchObject({ type: 'line', source: GLOBE_STATE_LINES_SOURCE_ID })
    expect(country).toMatchObject({ type: 'line', source: GLOBE_COUNTRY_LINES_SOURCE_ID })
    for (const layer of [state, country]) {
      expect(layer?.paint).toMatchObject({
        'line-color': GLOBE_BOUNDARY_COLOR,
        'line-width': GLOBE_BOUNDARY_WIDTH_PX,
      })
      // On from the lowest zoom, like the land fill they outline.
      expect(layer).not.toHaveProperty('minzoom')
    }
    const ids = layers.map((l) => l.id)
    expect(ids.indexOf(GLOBE_COUNTRY_LINES_LAYER_ID)).toBeGreaterThan(
      ids.indexOf(GLOBE_STATE_LINES_LAYER_ID),
    )
  })

  it('keeps the NASA credit on the raster source, where only a visible raster shows it', () => {
    const sources = globeSurfaceSources()
    expect(sources[NIGHT_EARTH_SOURCE_ID]).toMatchObject({
      attribution: expect.stringContaining('NASA'),
    })
    for (const id of [GLOBE_LAND_SOURCE_ID, GLOBE_STATE_LINES_SOURCE_ID, GLOBE_COUNTRY_LINES_SOURCE_ID]) {
      expect(sources[id], id).not.toHaveProperty('attribution')
    }
  })

  it('gives every surface layer the handoff fade and cutoff, so neither look outlives the crossfade', () => {
    const layers = globeSurfaceLayers({
      lightGlobe: true,
      fadeOut: FADE_OUT,
      maxZoom: MAX_ZOOM,
    })
    expect(layers.map((l) => l.id)).toEqual([
      GLOBE_OCEAN_LAYER_ID,
      GLOBE_LAND_LAYER_ID,
      GLOBE_STATE_LINES_LAYER_ID,
      GLOBE_COUNTRY_LINES_LAYER_ID,
      NIGHT_EARTH_LAYER_ID,
    ])
    for (const layer of layers) {
      expect(layer.maxzoom, layer.id).toBe(MAX_ZOOM)
      const paint = layer.paint as Record<string, unknown>
      const opacity =
        paint['background-opacity'] ??
        paint['fill-opacity'] ??
        paint['line-opacity'] ??
        paint['raster-opacity']
      expect(opacity, layer.id).toEqual(FADE_OUT)
    }
  })
})

describe('showGlobeSurface', () => {
  // showGlobeSurface fetches nothing itself: the land comes from a prefetch
  // or the map, the boundaries from loadGlobeBoundaries.
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})))
  })
  afterEach(() => vi.unstubAllGlobals())

  it('switches a live map to the light globe and loads the land file once', () => {
    const { map, setData, visibilityOf } = fakeMap()
    showGlobeSurface(map, true)
    showGlobeSurface(map, false)
    showGlobeSurface(map, true)
    expect(setData).toHaveBeenCalledTimes(1)
    expect(setData).toHaveBeenCalledWith(GLOBE_LAND_DATA_URL)
    expect(visibilityOf(NIGHT_EARTH_LAYER_ID)).toBe('none')
    expect(visibilityOf(GLOBE_LAND_LAYER_ID)).toBe('visible')
    expect(visibilityOf(GLOBE_OCEAN_LAYER_ID)).toBe('visible')
    expect(visibilityOf(GLOBE_STATE_LINES_LAYER_ID)).toBe('visible')
    expect(visibilityOf(GLOBE_COUNTRY_LINES_LAYER_ID)).toBe('visible')
  })

  it('switches back to the raster without touching the light-look sources', () => {
    const { map, setData, stateSetData, countrySetData, visibilityOf } = fakeMap()
    showGlobeSurface(map, false)
    expect(setData).not.toHaveBeenCalled()
    expect(stateSetData).not.toHaveBeenCalled()
    expect(countrySetData).not.toHaveBeenCalled()
    expect(fetch).not.toHaveBeenCalled()
    expect(visibilityOf(GLOBE_STATE_LINES_LAYER_ID)).toBe('none')
    expect(visibilityOf(NIGHT_EARTH_LAYER_ID)).toBe('visible')
    expect(visibilityOf(GLOBE_LAND_LAYER_ID)).toBe('none')
  })

  it('tracks the land request per map, so a fresh map loads its own copy', () => {
    const first = fakeMap()
    const second = fakeMap()
    showGlobeSurface(first.map, true)
    showGlobeSurface(second.map, true)
    expect(first.setData).toHaveBeenCalledTimes(1)
    expect(second.setData).toHaveBeenCalledTimes(1)
  })

  it('asks again for the land of a style re-created on the same map', () => {
    const { map, setData } = fakeMap()
    showGlobeSurface(map, true)
    expect(setData).toHaveBeenCalledTimes(1)
    // A restored WebGL context re-creates the style, and with it every source.
    const restoredSetData = vi.fn()
    const restoredLand = { setData: restoredSetData }
    const restoredOther = { setData: vi.fn() }
    map.getSource.mockImplementation((id: string) =>
      id === GLOBE_LAND_SOURCE_ID ? restoredLand : restoredOther,
    )
    showGlobeSurface(map, true)
    expect(restoredSetData).toHaveBeenCalledWith(GLOBE_LAND_DATA_URL)
    expect(setData).toHaveBeenCalledTimes(1)
  })

  it('retries the land request on a later switch when the source was not there yet', () => {
    const { map } = fakeMap({ withLandSource: false })
    expect(() => showGlobeSurface(map, true)).not.toThrow()
    const setData = vi.fn()
    const other = { setData: vi.fn() }
    map.getSource.mockImplementation((id: string) =>
      id === GLOBE_LAND_SOURCE_ID ? { setData } : other,
    )
    showGlobeSurface(map, true)
    expect(setData).toHaveBeenCalledWith(GLOBE_LAND_DATA_URL)
  })
})

describe('globe land data file', () => {
  const raw = readFileSync(LAND_FILE, 'utf8')
  const data = JSON.parse(raw) as GeoJSON.FeatureCollection

  it('stays small enough for the phone entry budget', () => {
    // The light globe exists to fit a phone entry budget; Natural Earth's
    // 1:50m land is ~900 KB at this precision, 1:110m ~76 KB. A file past
    // this bound has swapped in a heavier dataset.
    expect(statSync(LAND_FILE).size).toBeLessThan(100 * 1024)
  })

  it('is one MultiPolygon feature of closed rings inside lng/lat bounds', () => {
    expect(data.type).toBe('FeatureCollection')
    expect(data.features).toHaveLength(1)
    const geometry = data.features[0].geometry
    expect(geometry.type).toBe('MultiPolygon')
    const polygons = (geometry as GeoJSON.MultiPolygon).coordinates
    expect(polygons.length).toBeGreaterThan(100)
    const problems: string[] = []
    polygons.forEach((polygon, p) =>
      polygon.forEach((ring, r) => {
        const [first, last] = [ring[0], ring[ring.length - 1]]
        if (ring.length < 4) problems.push(`${p}/${r}: ${ring.length} positions`)
        if (first[0] !== last[0] || first[1] !== last[1]) problems.push(`${p}/${r}: open ring`)
        if (ring.some(([lng, lat]) => Math.abs(lng) > 180 || Math.abs(lat) > 90)) {
          problems.push(`${p}/${r}: out of bounds`)
        }
      }),
    )
    expect(problems).toEqual([])
  })
})

describe('prefetchGlobeLand', () => {
  const LAND: GeoJSON.FeatureCollection = { type: 'FeatureCollection', features: [] }

  // Answers the land file; any other request stays in flight. Either way a
  // request rejects as soon as its abort signal fires, as a real fetch does.
  function stubFetch(response: () => Promise<unknown>) {
    const fetchMock = vi.fn<(url: string, init?: RequestInit) => Promise<unknown>>(
      (url, init) =>
        new Promise((resolve, reject) => {
          init?.signal?.addEventListener('abort', () =>
            reject(new DOMException('aborted', 'AbortError')),
          )
          ;(url === GLOBE_LAND_DATA_URL ? response() : new Promise(() => {})).then(resolve, reject)
        }),
    )
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

  afterEach(() => vi.unstubAllGlobals())

  it('hands a map the prefetched collection instead of the URL, fetching once', async () => {
    const fetchMock = stubFetch(() =>
      Promise.resolve({ ok: true, json: () => Promise.resolve(LAND) }),
    )
    const mod = await freshModule()
    mod.prefetchGlobeLand()
    mod.prefetchGlobeLand()
    await settle()
    const { map, setData } = fakeMap()
    mod.showGlobeSurface(map, true)
    await vi.waitFor(() => expect(setData).toHaveBeenCalledWith(LAND))
    expect(fetchMock.mock.calls.filter((c) => c[0] === mod.GLOBE_LAND_DATA_URL)).toHaveLength(1)
    expect(fetchMock.mock.calls[0][0]).toBe(mod.GLOBE_LAND_DATA_URL)
  })

  it.each([
    ['a failed request', () => Promise.reject(new Error('offline'))],
    ['an HTTP error', () => Promise.resolve({ ok: false, json: () => Promise.resolve(LAND) })],
    [
      'a body that is not a FeatureCollection',
      () => Promise.resolve({ ok: true, json: () => Promise.resolve({ type: 'Feature' }) }),
    ],
  ])('falls back to the URL after %s', async (_label, response) => {
    stubFetch(response)
    const mod = await freshModule()
    mod.prefetchGlobeLand()
    await settle()
    const { map, setData } = fakeMap()
    mod.showGlobeSurface(map, true)
    await vi.waitFor(() => expect(setData).toHaveBeenCalledWith(mod.GLOBE_LAND_DATA_URL))
  })

  it('waits for an in-flight prefetch instead of downloading the file twice', async () => {
    let resolveFetch: (value: unknown) => void = () => {}
    stubFetch(() => new Promise((resolve) => (resolveFetch = resolve)))
    const mod = await freshModule()
    mod.prefetchGlobeLand()
    const { map, setData } = fakeMap()
    mod.showGlobeSurface(map, true)
    expect(setData).not.toHaveBeenCalled()
    resolveFetch({ ok: true, json: () => Promise.resolve(LAND) })
    await vi.waitFor(() => expect(setData).toHaveBeenCalledWith(LAND))
    expect(setData).toHaveBeenCalledTimes(1)
  })

  it('drops the prefetch for a map removed while it was in flight', async () => {
    let resolveFetch: (value: unknown) => void = () => {}
    stubFetch(() => new Promise((resolve) => (resolveFetch = resolve)))
    const mod = await freshModule()
    mod.prefetchGlobeLand()
    const { map, setData } = fakeMap()
    mod.showGlobeSurface(map, true)
    // A removed MapLibre map has no style, so getSource answers undefined.
    map.getSource.mockReturnValue(undefined)
    resolveFetch({ ok: true, json: () => Promise.resolve(LAND) })
    await settle()
    await settle()
    expect(setData).not.toHaveBeenCalled()
  })

  it('gives a style re-created while the prefetch was in flight the land once it lands', async () => {
    let resolveFetch: (value: unknown) => void = () => {}
    stubFetch(() => new Promise((resolve) => (resolveFetch = resolve)))
    const mod = await freshModule()
    mod.prefetchGlobeLand()
    const { map, setData } = fakeMap()
    mod.showGlobeSurface(map, true)
    // The context is lost and restored mid-fetch: the map now holds new sources.
    const restoredSetData = vi.fn()
    const restoredLand = { setData: restoredSetData }
    const restoredOther = { setData: vi.fn() }
    map.getSource.mockImplementation((id: string) =>
      id === mod.GLOBE_LAND_SOURCE_ID ? restoredLand : restoredOther,
    )
    mod.showGlobeSurface(map, true)
    resolveFetch({ ok: true, json: () => Promise.resolve(LAND) })
    await vi.waitFor(() => expect(restoredSetData).toHaveBeenCalledWith(LAND))
    expect(setData).not.toHaveBeenCalled()
  })

  it('waits 10 s on an unfinished prefetch, then fetches the URL; later maps skip the wait', async () => {
    vi.useFakeTimers()
    try {
      stubFetch(() => new Promise(() => {}))
      const mod = await freshModule()
      mod.prefetchGlobeLand()
      const first = fakeMap()
      mod.showGlobeSurface(first.map, true)
      await vi.advanceTimersByTimeAsync(9_999)
      expect(first.setData).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(1)
      expect(first.setData).toHaveBeenCalledWith(mod.GLOBE_LAND_DATA_URL)

      const second = fakeMap()
      mod.showGlobeSurface(second.map, true)
      expect(second.setData).toHaveBeenCalledWith(mod.GLOBE_LAND_DATA_URL)
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not cancel a slow prefetch: once it lands, later maps take its data', async () => {
    vi.useFakeTimers()
    try {
      let resolveFetch: (value: unknown) => void = () => {}
      const fetchMock = stubFetch(() => new Promise((resolve) => (resolveFetch = resolve)))
      const mod = await freshModule()
      mod.prefetchGlobeLand()
      const first = fakeMap()
      mod.showGlobeSurface(first.map, true)
      await vi.advanceTimersByTimeAsync(10_000)
      expect(first.setData).toHaveBeenCalledWith(mod.GLOBE_LAND_DATA_URL)

      resolveFetch({ ok: true, json: () => Promise.resolve(LAND) })
      await vi.advanceTimersByTimeAsync(0)
      const later = fakeMap()
      mod.showGlobeSurface(later.map, true)
      await vi.advanceTimersByTimeAsync(0)
      expect(later.setData).toHaveBeenCalledWith(LAND)
      expect(fetchMock.mock.calls.filter((c) => c[0] === mod.GLOBE_LAND_DATA_URL)).toHaveLength(1)
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('globe boundary data files', () => {
  it.each([
    [GLOBE_COUNTRY_LINES_DATA_URL, 60 * 1024],
    [GLOBE_STATE_LINES_DATA_URL, 120 * 1024],
  ])('%s is one MultiLineString of in-bounds lines, under its size bound', (url, maxBytes) => {
    // The bounds hold the 1:110m country and 1:50m state files with room to
    // spare; a file past them has swapped in a heavier scale.
    expect(statSync(publicFile(url)).size).toBeLessThan(maxBytes)
    const data = JSON.parse(readFileSync(publicFile(url), 'utf8')) as GeoJSON.FeatureCollection
    expect(data.type).toBe('FeatureCollection')
    expect(data.features).toHaveLength(1)
    const geometry = data.features[0].geometry as GeoJSON.MultiLineString
    expect(geometry.type).toBe('MultiLineString')
    expect(geometry.coordinates.length).toBeGreaterThan(100)
    const problems = geometry.coordinates.flatMap((line, i) =>
      line.length < 2 || line.some(([lng, lat]) => Math.abs(lng) > 180 || Math.abs(lat) > 90)
        ? [i]
        : [],
    )
    expect(problems).toEqual([])
  })
})

describe('globe places data file', () => {
  const data = JSON.parse(
    readFileSync(publicFile(GLOBE_PLACES_DATA_URL), 'utf8'),
  ) as GeoJSON.FeatureCollection<GeoJSON.Point, { name: string; rank: number }>

  it('holds every 1:110m place once, ranked 0..n-1 in file order, name and rank only', () => {
    expect(data.features).toHaveLength(243)
    data.features.forEach((feature, i) => {
      expect(feature.properties).toEqual({ name: expect.any(String), rank: i })
      expect(feature.geometry.type).toBe('Point')
    })
  })

  it('ranks national capitals and the largest cities ahead of small capitals', () => {
    const rank = (name: string) =>
      data.features.find((f) => f.properties.name === name)?.properties.rank ?? Infinity
    expect(rank('New York')).toBeLessThan(rank('Belmopan'))
    expect(rank('Los Angeles')).toBeLessThan(rank('Nassau'))
    expect(rank('Washington, D.C.')).toBeLessThan(rank('Ottawa'))
  })
})

describe('boundary lines and place data', () => {
  const FC: GeoJSON.FeatureCollection = { type: 'FeatureCollection', features: [] }
  const ok = (data: unknown = FC) => Promise.resolve({ ok: true, json: () => Promise.resolve(data) })
  const settle = () => new Promise((resolve) => setTimeout(resolve, 0))
  const fetchedUrls = (fetchMock: { mock: { calls: unknown[][] } }) =>
    fetchMock.mock.calls.map((c) => c[0] as string)

  afterEach(() => vi.unstubAllGlobals())

  it('prefetches only the land file ahead of the map', async () => {
    const fetchMock = vi.fn<(url: string) => Promise<unknown>>(() => ok())
    vi.stubGlobal('fetch', fetchMock)
    const mod = await freshModule()
    mod.prefetchGlobeLand()
    await settle()
    expect(fetchedUrls(fetchMock)).toEqual([GLOBE_LAND_DATA_URL])
  })

  it('fetches each boundary file once per map and hands its source the collection once it arrives', async () => {
    const byUrl = new Map<string, GeoJSON.FeatureCollection>(
      [GLOBE_STATE_LINES_DATA_URL, GLOBE_COUNTRY_LINES_DATA_URL].map((url) => [
        url,
        { type: 'FeatureCollection', features: [] },
      ]),
    )
    const pending = new Map<string, (value: unknown) => void>()
    const fetchMock = vi.fn(
      (url: string) => new Promise((resolve) => pending.set(url, resolve)),
    )
    vi.stubGlobal('fetch', fetchMock)
    const mod = await freshModule()
    const { map, stateSetData, countrySetData } = fakeMap()
    mod.loadGlobeBoundaries(map)
    mod.loadGlobeBoundaries(map)
    // Until a file arrives its source keeps its empty collection.
    expect(stateSetData).not.toHaveBeenCalled()
    expect(fetchedUrls(fetchMock).sort()).toEqual(
      [GLOBE_STATE_LINES_DATA_URL, GLOBE_COUNTRY_LINES_DATA_URL].sort(),
    )
    for (const [url, data] of byUrl) pending.get(url)?.({ ok: true, json: () => Promise.resolve(data) })
    // Identity, not shape: the two collections are equal, so only the object
    // each source received tells them apart.
    await vi.waitFor(() => {
      expect(stateSetData.mock.calls[0]?.[0]).toBe(byUrl.get(GLOBE_STATE_LINES_DATA_URL))
      expect(countrySetData.mock.calls[0]?.[0]).toBe(byUrl.get(GLOBE_COUNTRY_LINES_DATA_URL))
    })
    expect(stateSetData).toHaveBeenCalledTimes(1)
    expect(countrySetData).toHaveBeenCalledTimes(1)
  })

  it('hands a failed boundary file to the map as a URL and leaves the other on its data', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) =>
        url === GLOBE_STATE_LINES_DATA_URL ? Promise.reject(new Error('offline')) : ok(),
      ),
    )
    const mod = await freshModule()
    const { map, stateSetData, countrySetData } = fakeMap()
    mod.loadGlobeBoundaries(map)
    await vi.waitFor(() => {
      expect(stateSetData).toHaveBeenCalledWith(GLOBE_STATE_LINES_DATA_URL)
      expect(countrySetData).toHaveBeenCalledWith(FC)
    })
  })

  it('loads the boundaries even when the land source is missing', async () => {
    vi.stubGlobal('fetch', vi.fn(() => ok()))
    const mod = await freshModule()
    const { map, stateSetData, countrySetData } = fakeMap({ withLandSource: false })
    mod.loadGlobeBoundaries(map)
    await vi.waitFor(() => {
      expect(stateSetData).toHaveBeenCalledWith(FC)
      expect(countrySetData).toHaveBeenCalledWith(FC)
    })
  })

  it('drops boundary data for a map removed while the file was in flight', async () => {
    let resolveFetch: (value: unknown) => void = () => {}
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) =>
        url === GLOBE_STATE_LINES_DATA_URL
          ? new Promise((resolve) => (resolveFetch = resolve))
          : new Promise(() => {}),
      ),
    )
    const mod = await freshModule()
    const { map, stateSetData } = fakeMap()
    mod.loadGlobeBoundaries(map)
    map.getSource.mockReturnValue(undefined)
    resolveFetch({ ok: true, json: () => Promise.resolve(FC) })
    await settle()
    await settle()
    expect(stateSetData).not.toHaveBeenCalled()
  })

  it('loads the boundaries again into a style re-created on the same map', async () => {
    vi.stubGlobal('fetch', vi.fn(() => ok()))
    const mod = await freshModule()
    const { map, stateSetData } = fakeMap()
    mod.loadGlobeBoundaries(map)
    await vi.waitFor(() => expect(stateSetData).toHaveBeenCalledWith(FC))
    const restored = { setData: vi.fn() }
    const restoredOther = { setData: vi.fn() }
    map.getSource.mockImplementation((id: string) =>
      id === GLOBE_STATE_LINES_SOURCE_ID ? restored : restoredOther,
    )
    mod.loadGlobeBoundaries(map)
    await vi.waitFor(() => expect(restored.setData).toHaveBeenCalledWith(FC))
    expect(stateSetData).toHaveBeenCalledTimes(1)
  })

  it('loads the place data once and shares it', async () => {
    const fetchMock = vi.fn<(url: string) => Promise<unknown>>(() => ok())
    vi.stubGlobal('fetch', fetchMock)
    const mod = await freshModule()
    await expect(mod.loadGlobePlaces()).resolves.toEqual({ data: FC })
    await expect(mod.loadGlobePlaces()).resolves.toEqual({ data: FC })
    expect(fetchedUrls(fetchMock)).toEqual([GLOBE_PLACES_DATA_URL])
  })

  it('retries a failed place fetch once', async () => {
    const fetchMock = vi
      .fn<(url: string) => Promise<unknown>>()
      .mockImplementationOnce(() => Promise.resolve({ ok: false, status: 503 }))
      .mockImplementation(() => ok())
    vi.stubGlobal('fetch', fetchMock)
    const mod = await freshModule()
    await expect(mod.loadGlobePlaces()).resolves.toEqual({ data: FC })
    expect(fetchedUrls(fetchMock)).toEqual([GLOBE_PLACES_DATA_URL, GLOBE_PLACES_DATA_URL])
  })

  it.each([
    ['a network failure', () => Promise.reject(new Error('offline')), 0],
    ['an HTTP error', () => Promise.resolve({ ok: false, status: 404 }), 404],
    ['a body that is not a FeatureCollection', () => ok({ type: 'Feature' }), undefined],
    ['a FeatureCollection without features', () => ok({ type: 'FeatureCollection' }), undefined],
    [
      'a body that is not JSON',
      () => Promise.resolve({ ok: true, json: () => Promise.reject(new SyntaxError('bad')) }),
      undefined,
    ],
  ])(
    'reports %s with its status after the retry, and fetches again on the next load',
    async (_label, fail, status) => {
      const fetchMock = vi
        .fn<(url: string) => Promise<unknown>>()
        .mockImplementationOnce(fail)
        .mockImplementationOnce(fail)
        .mockImplementation(() => ok())
      vi.stubGlobal('fetch', fetchMock)
      const mod = await freshModule()
      await expect(mod.loadGlobePlaces()).resolves.toEqual({ data: null, status })
      await settle()
      await expect(mod.loadGlobePlaces()).resolves.toEqual({ data: FC })
      expect(fetchedUrls(fetchMock)).toHaveLength(3)
    },
  )

  // A fetch that never answers until its abort signal fires, as a stalled
  // connection does.
  const stalled = (_url: string, init?: RequestInit) =>
    new Promise((_resolve, reject) =>
      init?.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError'))),
    )

  it('gives up on a stalled place fetch after 10 s, retries once, and reports a network failure', async () => {
    vi.useFakeTimers()
    try {
      const fetchMock = vi.fn(stalled)
      vi.stubGlobal('fetch', fetchMock)
      const mod = await freshModule()
      const loading = mod.loadGlobePlaces()
      await vi.advanceTimersByTimeAsync(10_000)
      expect(fetchMock).toHaveBeenCalledTimes(2)
      await vi.advanceTimersByTimeAsync(10_000)
      await expect(loading).resolves.toEqual({ data: null, status: 0 })
      // Cleared like any failure: the next load asks again.
      fetchMock.mockImplementation(() => ok())
      await expect(mod.loadGlobePlaces()).resolves.toEqual({ data: FC })
    } finally {
      vi.useRealTimers()
    }
  })

  it('hands a map the URL of a boundary file whose fetch stalled', async () => {
    vi.useFakeTimers()
    try {
      vi.stubGlobal('fetch', vi.fn(stalled))
      const mod = await freshModule()
      const { map, stateSetData } = fakeMap()
      mod.loadGlobeBoundaries(map)
      await vi.advanceTimersByTimeAsync(9_999)
      expect(stateSetData).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(1)
      expect(stateSetData).toHaveBeenCalledWith(GLOBE_STATE_LINES_DATA_URL)
    } finally {
      vi.useRealTimers()
    }
  })

  it('validates the land prefetch like the other files: a collection without features is unusable', async () => {
    vi.stubGlobal('fetch', vi.fn(() => ok({ type: 'FeatureCollection' })))
    const mod = await freshModule()
    mod.prefetchGlobeLand()
    await settle()
    const { map, setData } = fakeMap()
    mod.showGlobeSurface(map, true)
    await vi.waitFor(() => expect(setData).toHaveBeenCalledWith(GLOBE_LAND_DATA_URL))
  })

  it('never aborts the land prefetch, so a slow one still serves later maps', async () => {
    const fetchMock = vi.fn<(url: string, init?: RequestInit) => Promise<unknown>>(() => ok())
    vi.stubGlobal('fetch', fetchMock)
    const mod = await freshModule()
    mod.prefetchGlobeLand()
    await settle()
    expect(fetchMock.mock.calls[0][1]?.signal).toBeUndefined()
  })
})
