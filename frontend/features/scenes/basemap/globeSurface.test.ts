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
  GLOBE_LAND_DATA_URL,
  GLOBE_LAND_LAYER_ID,
  GLOBE_LAND_SOURCE_ID,
  GLOBE_OCEAN_LAYER_ID,
  NIGHT_EARTH_LAYER_ID,
  globeSurfaceLayers,
  globeSurfaceSources,
  globeSurfaceVisibility,
  showGlobeSurface,
  type GlobeSurfaceMap,
} from './globeSurface'

const FRONTEND_ROOT = path.resolve(__dirname, '../../..')
const LAND_FILE = path.join(FRONTEND_ROOT, 'public', GLOBE_LAND_DATA_URL)

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
  // One object, as MapLibre returns the same source instance on every call.
  const landSource = { setData }
  const layerIds = new Set([
    GLOBE_OCEAN_LAYER_ID,
    GLOBE_LAND_LAYER_ID,
    NIGHT_EARTH_LAYER_ID,
  ])
  const map = {
    getLayer: vi.fn((id: string) => (layerIds.has(id) ? { id } : undefined)),
    setLayoutProperty: vi.fn(),
    getSource: vi.fn((id: string) =>
      id === GLOBE_LAND_SOURCE_ID && opts.withLandSource !== false
        ? landSource
        : undefined,
    ),
  } satisfies GlobeSurfaceMap
  const visibilityOf = (layer: string) =>
    map.setLayoutProperty.mock.calls.filter((c) => c[0] === layer).at(-1)?.[2]
  return { map, setData, visibilityOf }
}

describe('globe surface looks', () => {
  it('hides the night-earth raster on the light globe and shows ocean and land', () => {
    expect(globeSurfaceVisibility(true)).toEqual({
      [GLOBE_OCEAN_LAYER_ID]: 'visible',
      [GLOBE_LAND_LAYER_ID]: 'visible',
      [NIGHT_EARTH_LAYER_ID]: 'none',
    })
  })

  it('shows only the raster on the full globe', () => {
    expect(globeSurfaceVisibility(false)).toEqual({
      [GLOBE_OCEAN_LAYER_ID]: 'none',
      [GLOBE_LAND_LAYER_ID]: 'none',
      [NIGHT_EARTH_LAYER_ID]: 'visible',
    })
  })

  it.each([true, false])('builds a spec-valid style fragment (lightGlobe=%s)', (light) => {
    expect(validateStyleMin(surfaceStyle(light))).toEqual([])
  })

  it('registers the land source empty, so a map that never shows the light globe never fetches it', () => {
    // A GeoJSON source fetches its data when added, visible or not.
    expect(globeSurfaceSources()[GLOBE_LAND_SOURCE_ID]).toMatchObject({
      data: { type: 'FeatureCollection', features: [] },
    })
  })

  it('keeps the NASA credit on the raster source, where only a visible raster shows it', () => {
    const sources = globeSurfaceSources()
    expect(sources[NIGHT_EARTH_SOURCE_ID]).toMatchObject({
      attribution: expect.stringContaining('NASA'),
    })
    expect(sources[GLOBE_LAND_SOURCE_ID]).not.toHaveProperty('attribution')
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
      NIGHT_EARTH_LAYER_ID,
    ])
    for (const layer of layers) {
      expect(layer.maxzoom, layer.id).toBe(MAX_ZOOM)
      const paint = layer.paint as Record<string, unknown>
      const opacity =
        paint['background-opacity'] ?? paint['fill-opacity'] ?? paint['raster-opacity']
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

  it('switches back to the raster without touching the land source', () => {
    const { map, setData, visibilityOf } = fakeMap()
    showGlobeSurface(map, false)
    expect(setData).not.toHaveBeenCalled()
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
    map.getSource.mockImplementation((id: string) =>
      id === GLOBE_LAND_SOURCE_ID ? { setData } : undefined,
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
    mod.prefetchGlobeLand()
    mod.prefetchGlobeLand()
    await settle()
    const { map, setData } = fakeMap()
    mod.showGlobeSurface(map, true)
    await vi.waitFor(() => expect(setData).toHaveBeenCalledWith(LAND))
    expect(fetchMock).toHaveBeenCalledTimes(1)
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
      expect(fetchMock).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })
})
