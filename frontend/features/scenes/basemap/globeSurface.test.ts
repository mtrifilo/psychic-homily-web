import { describe, it, expect, vi, afterEach } from 'vitest'
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
  })

  it('loads each boundary file into its own source once, after the land', () => {
    const { map, setData, stateSetData, countrySetData, visibilityOf } = fakeMap()
    showGlobeSurface(map, true)
    showGlobeSurface(map, false)
    showGlobeSurface(map, true)
    expect(stateSetData).toHaveBeenCalledTimes(1)
    expect(stateSetData).toHaveBeenCalledWith(GLOBE_STATE_LINES_DATA_URL)
    expect(countrySetData).toHaveBeenCalledTimes(1)
    expect(countrySetData).toHaveBeenCalledWith(GLOBE_COUNTRY_LINES_DATA_URL)
    expect(setData.mock.invocationCallOrder[0]).toBeLessThan(
      stateSetData.mock.invocationCallOrder[0],
    )
    expect(visibilityOf(GLOBE_STATE_LINES_LAYER_ID)).toBe('visible')
    expect(visibilityOf(GLOBE_COUNTRY_LINES_LAYER_ID)).toBe('visible')
  })

  it('loads the boundaries even when the land source is missing', () => {
    const { map, stateSetData, countrySetData } = fakeMap({ withLandSource: false })
    showGlobeSurface(map, true)
    expect(stateSetData).toHaveBeenCalledWith(GLOBE_STATE_LINES_DATA_URL)
    expect(countrySetData).toHaveBeenCalledWith(GLOBE_COUNTRY_LINES_DATA_URL)
  })

  it('switches back to the raster without touching the light-look sources', () => {
    const { map, setData, stateSetData, countrySetData, visibilityOf } = fakeMap()
    showGlobeSurface(map, false)
    expect(setData).not.toHaveBeenCalled()
    expect(stateSetData).not.toHaveBeenCalled()
    expect(countrySetData).not.toHaveBeenCalled()
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

describe('prefetchGlobeSurface', () => {
  const LAND: GeoJSON.FeatureCollection = { type: 'FeatureCollection', features: [] }

  // Module state is once per page load by design, so each case gets a fresh
  // module instance rather than a reset seam.
  async function freshModule() {
    vi.resetModules()
    return import('./globeSurface')
  }

  function stubFetch(response: () => Promise<unknown>) {
    const fetchMock = vi.fn<(url: string, init?: RequestInit) => Promise<unknown>>(() => response())
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
    mod.prefetchGlobeSurface()
    mod.prefetchGlobeSurface()
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
    mod.prefetchGlobeSurface()
    await settle()
    const { map, setData } = fakeMap()
    mod.showGlobeSurface(map, true)
    await vi.waitFor(() => expect(setData).toHaveBeenCalledWith(mod.GLOBE_LAND_DATA_URL))
  })

  it('waits for an in-flight prefetch instead of downloading the file twice', async () => {
    let resolveFetch: (value: unknown) => void = () => {}
    stubFetch(() => new Promise((resolve) => (resolveFetch = resolve)))
    const mod = await freshModule()
    mod.prefetchGlobeSurface()
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
    mod.prefetchGlobeSurface()
    const { map, setData } = fakeMap()
    mod.showGlobeSurface(map, true)
    // A removed MapLibre map has no style, so getSource answers undefined.
    map.getSource.mockReturnValue(undefined)
    resolveFetch({ ok: true, json: () => Promise.resolve(LAND) })
    await settle()
    await settle()
    expect(setData).not.toHaveBeenCalled()
  })

  it('waits 10 s on an unfinished prefetch, then fetches the URL; later maps skip the wait', async () => {
    vi.useFakeTimers()
    try {
      stubFetch(() => new Promise(() => {}))
      const mod = await freshModule()
      mod.prefetchGlobeSurface()
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
      mod.prefetchGlobeSurface()
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

describe('prefetch order and place data', () => {
  const FC: GeoJSON.FeatureCollection = { type: 'FeatureCollection', features: [] }
  const ok = () => Promise.resolve({ ok: true, json: () => Promise.resolve(FC) })

  async function freshModule() {
    vi.resetModules()
    return import('./globeSurface')
  }

  afterEach(() => vi.unstubAllGlobals())

  it('fetches the line and place files only after the land file settles', async () => {
    let resolveLand: (value: unknown) => void = () => {}
    const fetchMock = vi.fn((url: string) =>
      url === GLOBE_LAND_DATA_URL ? new Promise((resolve) => (resolveLand = resolve)) : ok(),
    )
    vi.stubGlobal('fetch', fetchMock)
    const mod = await freshModule()
    mod.prefetchGlobeSurface()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([GLOBE_LAND_DATA_URL])
    resolveLand({ ok: false })
    await vi.waitFor(() =>
      expect(fetchMock.mock.calls.map((c) => c[0]).sort()).toEqual(
        [
          GLOBE_LAND_DATA_URL,
          GLOBE_STATE_LINES_DATA_URL,
          GLOBE_COUNTRY_LINES_DATA_URL,
          GLOBE_PLACES_DATA_URL,
        ].sort(),
      ),
    )
  })

  it('hands each line source its own prefetched collection', async () => {
    const byUrl = new Map<string, GeoJSON.FeatureCollection>([
      [GLOBE_LAND_DATA_URL, { type: 'FeatureCollection', features: [] }],
      [GLOBE_STATE_LINES_DATA_URL, { type: 'FeatureCollection', features: [] }],
      [GLOBE_COUNTRY_LINES_DATA_URL, { type: 'FeatureCollection', features: [] }],
    ])
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => Promise.resolve({ ok: true, json: () => Promise.resolve(byUrl.get(url)) })),
    )
    const mod = await freshModule()
    mod.prefetchGlobeSurface()
    const { map, setData, stateSetData, countrySetData } = fakeMap()
    mod.showGlobeSurface(map, true)
    await vi.waitFor(() => {
      expect(setData).toHaveBeenCalledWith(byUrl.get(GLOBE_LAND_DATA_URL))
      expect(stateSetData).toHaveBeenCalledWith(byUrl.get(GLOBE_STATE_LINES_DATA_URL))
      expect(countrySetData).toHaveBeenCalledWith(byUrl.get(GLOBE_COUNTRY_LINES_DATA_URL))
    })
    // Identity, not just shape: each source got the object fetched for it.
    expect(stateSetData.mock.calls[0][0]).toBe(byUrl.get(GLOBE_STATE_LINES_DATA_URL))
    expect(countrySetData.mock.calls[0][0]).toBe(byUrl.get(GLOBE_COUNTRY_LINES_DATA_URL))
  })

  it('a failed line file falls back to its URL and leaves the others on their data', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) =>
        url === GLOBE_STATE_LINES_DATA_URL ? Promise.reject(new Error('offline')) : ok(),
      ),
    )
    const mod = await freshModule()
    mod.prefetchGlobeSurface()
    const { map, setData, stateSetData, countrySetData } = fakeMap()
    mod.showGlobeSurface(map, true)
    await vi.waitFor(() => {
      expect(stateSetData).toHaveBeenCalledWith(GLOBE_STATE_LINES_DATA_URL)
      expect(countrySetData).toHaveBeenCalledWith(FC)
      expect(setData).toHaveBeenCalledWith(FC)
    })
  })

  it('serves the place data from the prefetch, fetching it once', async () => {
    const fetchMock = vi.fn<(url: string) => Promise<unknown>>(() => ok())
    vi.stubGlobal('fetch', fetchMock)
    const mod = await freshModule()
    mod.prefetchGlobeSurface()
    await expect(mod.loadGlobePlaces()).resolves.toBe(FC)
    await expect(mod.loadGlobePlaces()).resolves.toBe(FC)
    expect(fetchMock.mock.calls.filter((c) => c[0] === GLOBE_PLACES_DATA_URL)).toHaveLength(1)
  })

  it('fetches the place data itself without a prefetch, and again after a failure', async () => {
    const fetchMock = vi
      .fn<(url: string) => Promise<unknown>>()
      .mockImplementationOnce(() => Promise.reject(new Error('offline')))
      .mockImplementation(() => ok())
    vi.stubGlobal('fetch', fetchMock)
    const mod = await freshModule()
    await expect(mod.loadGlobePlaces()).resolves.toBeNull()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await expect(mod.loadGlobePlaces()).resolves.toBe(FC)
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([
      GLOBE_PLACES_DATA_URL,
      GLOBE_PLACES_DATA_URL,
    ])
  })
})
