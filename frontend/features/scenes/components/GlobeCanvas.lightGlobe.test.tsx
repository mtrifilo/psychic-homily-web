import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'
import GlobeCanvas from './GlobeCanvas'
import { ATLAS_COMPACT_VIEWPORT_QUERY } from '../atlasViewport'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import {
  GLOBE_COUNTRY_LINES_DATA_URL,
  GLOBE_COUNTRY_LINES_LAYER_ID,
  GLOBE_COUNTRY_LINES_SOURCE_ID,
  GLOBE_LAND_DATA_URL,
  GLOBE_LAND_LAYER_ID,
  GLOBE_LAND_SOURCE_ID,
  GLOBE_OCEAN_LAYER_ID,
  GLOBE_STATE_LINES_DATA_URL,
  GLOBE_STATE_LINES_LAYER_ID,
  GLOBE_STATE_LINES_SOURCE_ID,
  NIGHT_EARTH_LAYER_ID,
} from '../basemap/globeSurface'

/**
 * GlobeCanvas's choice of globe surface, with MapLibre stubbed down to the
 * seams that choice drives: the style handed to the constructor, the layout
 * visibility set on a live map, and the land source's data. Whether the
 * hidden raster really costs no request is a browser fact, proven by the
 * network log of the perf harness, not here.
 */
interface StubMap {
  options: { style: StubStyle }
  fire: (event: string) => void
  setLayoutProperty: ReturnType<typeof vi.fn>
  sourceSetData: (id: string) => ReturnType<typeof vi.fn>
}

interface StubStyle {
  sources: Record<string, { data?: unknown }>
  layers: { id: string; layout?: { visibility?: string } }[]
}

let maps: StubMap[] = []

const markAtlasMapReady = vi.hoisted(() => vi.fn())
vi.mock('@/lib/atlasMapReady', () => ({ markAtlasMapReady }))

vi.mock('maplibre-gl', () => {
  class StubControl {
    constructor(public options: unknown) {}
  }
  class StubMapImpl {
    handlers = new Map<string, ((e?: unknown) => void)[]>()
    setLayoutProperty = vi.fn()
    setPaintProperty = vi.fn()
    setFeatureState = vi.fn()
    removeFeatureState = vi.fn()
    addControl = vi.fn()
    removeControl = vi.fn()
    remove = vi.fn()
    flyTo = vi.fn()
    painter: object | undefined = {}
    touchZoomRotate = { disableRotation: vi.fn() }
    keyboard = { disableRotation: vi.fn() }
    canvas = document.createElement('canvas')
    container = document.createElement('div')

    constructor(public options: { style: StubStyle }) {
      maps.push(this as unknown as StubMap)
    }
    on(event: string, layerOrHandler: unknown, maybeHandler?: unknown) {
      const handler = (typeof layerOrHandler === 'function'
        ? layerOrHandler
        : maybeHandler) as (e?: unknown) => void
      const key = typeof layerOrHandler === 'string' ? `${event}:${layerOrHandler}` : event
      this.handlers.set(key, [...(this.handlers.get(key) ?? []), handler])
    }
    off(event: string, handler: (e?: unknown) => void) {
      this.handlers.set(event, (this.handlers.get(event) ?? []).filter((h) => h !== handler))
    }
    once(event: string, handler: (e?: unknown) => void) {
      this.on(event, handler)
    }
    fire(event: string) {
      for (const h of this.handlers.get(event) ?? []) h()
    }
    getLayer(id: string) {
      return this.options.style.layers.find((l) => l.id === id)
    }
    sources = new Map<string, { setData: ReturnType<typeof vi.fn> }>()
    getSource(id: string) {
      if (!(id in this.options.style.sources)) return undefined
      // One object per id, as MapLibre returns the same source every call.
      if (!this.sources.has(id)) this.sources.set(id, { setData: vi.fn() })
      return this.sources.get(id)
    }
    sourceSetData(id: string) {
      return (this.getSource(id) as { setData: ReturnType<typeof vi.fn> }).setData
    }
    isStyleLoaded() {
      return true
    }
    areTilesLoaded() {
      return true
    }
    getZoom() {
      return 1.6
    }
    getCenter() {
      return { lng: 0, lat: 0 }
    }
    getBounds() {
      return { contains: () => false }
    }
    getCanvas() {
      return this.canvas
    }
    getContainer() {
      return this.container
    }
  }
  return {
    Map: StubMapImpl,
    AttributionControl: StubControl,
    NavigationControl: StubControl,
    Marker: StubControl,
    setWorkerUrl: vi.fn(),
  }
})

const POV = { lat: 39.5, lng: -98.35, altitude: 1.8 }

function renderCanvas() {
  return render(
    <GlobeCanvas width={390} height={731} scenes={[]} pov={POV} onSelect={() => {}} />,
  )
}

function theMap(): StubMap {
  expect(maps).toHaveLength(1)
  return maps[0]
}

function constructedVisibility(map: StubMap, layer: string) {
  return map.options.style.layers.find((l) => l.id === layer)?.layout?.visibility
}

function lastVisibilitySet(map: StubMap, layer: string) {
  return map.setLayoutProperty.mock.calls.filter((c) => c[0] === layer).at(-1)?.[2]
}

describe('GlobeCanvas globe surface', () => {
  let restoreMatchMedia: () => void = () => {}
  function installViewport(compact: boolean) {
    const mm = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: compact })
    restoreMatchMedia = mm.restore
    return { set: (next: boolean) => mm.set(ATLAS_COMPACT_VIEWPORT_QUERY, next) }
  }
  beforeEach(() => {
    maps = []
    sessionStorage.clear()
    // The boundary and place-label files, failing: their content is the
    // basemap and placeLabels suites' concern.
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({ ok: false })))
  })
  afterEach(() => {
    restoreMatchMedia()
    vi.unstubAllGlobals()
  })

  it('builds a compact-viewport map without the night-earth raster', () => {
    installViewport(true)
    renderCanvas()
    const map = theMap()
    expect(constructedVisibility(map, NIGHT_EARTH_LAYER_ID)).toBe('none')
    expect(constructedVisibility(map, GLOBE_OCEAN_LAYER_ID)).toBe('visible')
    expect(constructedVisibility(map, GLOBE_LAND_LAYER_ID)).toBe('visible')
  })

  it('loads the land file once the compact map style loads', () => {
    installViewport(true)
    renderCanvas()
    const map = theMap()
    expect(map.sourceSetData(GLOBE_LAND_SOURCE_ID)).not.toHaveBeenCalled()
    act(() => map.fire('style.load'))
    expect(map.sourceSetData(GLOBE_LAND_SOURCE_ID)).toHaveBeenCalledTimes(1)
    expect(map.sourceSetData(GLOBE_LAND_SOURCE_ID)).toHaveBeenCalledWith(GLOBE_LAND_DATA_URL)
    expect(lastVisibilitySet(map, NIGHT_EARTH_LAYER_ID)).toBe('none')
  })

  it('builds a compact-viewport map with both boundary layers shown, and loads their files after the first full render', async () => {
    installViewport(true)
    renderCanvas()
    const map = theMap()
    expect(constructedVisibility(map, GLOBE_STATE_LINES_LAYER_ID)).toBe('visible')
    expect(constructedVisibility(map, GLOBE_COUNTRY_LINES_LAYER_ID)).toBe('visible')
    act(() => map.fire('style.load'))
    // The first full render is not in yet: nothing on top of it is fetched.
    expect(fetch).not.toHaveBeenCalled()
    act(() => map.fire('render'))
    expect(fetch).toHaveBeenCalledWith(GLOBE_STATE_LINES_DATA_URL, expect.anything())
    expect(fetch).toHaveBeenCalledWith(GLOBE_COUNTRY_LINES_DATA_URL, expect.anything())
    // The stubbed fetch fails, so each source is handed its URL instead.
    const state = map.sourceSetData(GLOBE_STATE_LINES_SOURCE_ID)
    const country = map.sourceSetData(GLOBE_COUNTRY_LINES_SOURCE_ID)
    await vi.waitFor(() => {
      expect(state).toHaveBeenCalledExactlyOnceWith(GLOBE_STATE_LINES_DATA_URL)
      expect(country).toHaveBeenCalledExactlyOnceWith(GLOBE_COUNTRY_LINES_DATA_URL)
    })
  })

  // The Atlas-ready signal holds the chrome's prefetches on /atlas; the map's
  // first full render is what releases it.
  it('releases the Atlas-ready signal at the first full render, once', () => {
    markAtlasMapReady.mockClear()
    installViewport(true)
    renderCanvas()
    const map = theMap()
    act(() => map.fire('style.load'))
    expect(markAtlasMapReady).not.toHaveBeenCalled()
    act(() => map.fire('render'))
    act(() => map.fire('render'))
    expect(markAtlasMapReady).toHaveBeenCalledTimes(1)
  })

  it('builds a wide-viewport map with the boundary layers hidden and never loads them', () => {
    installViewport(false)
    renderCanvas()
    const map = theMap()
    act(() => map.fire('style.load'))
    act(() => map.fire('render'))
    expect(constructedVisibility(map, GLOBE_STATE_LINES_LAYER_ID)).toBe('none')
    expect(constructedVisibility(map, GLOBE_COUNTRY_LINES_LAYER_ID)).toBe('none')
    expect(map.sourceSetData(GLOBE_STATE_LINES_SOURCE_ID)).not.toHaveBeenCalled()
    expect(map.sourceSetData(GLOBE_COUNTRY_LINES_SOURCE_ID)).not.toHaveBeenCalled()
    expect(fetch).not.toHaveBeenCalled()
  })

  it('builds a wide-viewport map with the raster and no land request', () => {
    installViewport(false)
    renderCanvas()
    const map = theMap()
    act(() => map.fire('style.load'))
    expect(constructedVisibility(map, NIGHT_EARTH_LAYER_ID)).toBe('visible')
    expect(constructedVisibility(map, GLOBE_LAND_LAYER_ID)).toBe('none')
    expect(map.sourceSetData(GLOBE_LAND_SOURCE_ID)).not.toHaveBeenCalled()
  })

  it('switches the live map across the breakpoint without rebuilding it', () => {
    const viewport = installViewport(false)
    renderCanvas()
    const map = theMap()
    act(() => map.fire('style.load'))

    viewport.set(true)
    expect(lastVisibilitySet(map, NIGHT_EARTH_LAYER_ID)).toBe('none')
    expect(lastVisibilitySet(map, GLOBE_LAND_LAYER_ID)).toBe('visible')
    expect(map.sourceSetData(GLOBE_LAND_SOURCE_ID)).toHaveBeenCalledWith(GLOBE_LAND_DATA_URL)

    viewport.set(false)
    expect(lastVisibilitySet(map, NIGHT_EARTH_LAYER_ID)).toBe('visible')
    expect(lastVisibilitySet(map, GLOBE_LAND_LAYER_ID)).toBe('none')
    expect(maps).toHaveLength(1)
  })

  it('loads the boundary files when a loaded wide map crosses into the light look', () => {
    const viewport = installViewport(false)
    renderCanvas()
    const map = theMap()
    act(() => map.fire('style.load'))
    act(() => map.fire('render'))
    expect(fetch).not.toHaveBeenCalled()
    act(() => viewport.set(true))
    expect(fetch).toHaveBeenCalledWith(GLOBE_STATE_LINES_DATA_URL, expect.anything())
    expect(fetch).toHaveBeenCalledWith(GLOBE_COUNTRY_LINES_DATA_URL, expect.anything())
    expect(lastVisibilitySet(map, GLOBE_STATE_LINES_LAYER_ID)).toBe('visible')
    act(() => viewport.set(false))
    expect(lastVisibilitySet(map, GLOBE_STATE_LINES_LAYER_ID)).toBe('none')
    expect(maps).toHaveLength(1)
  })
})
