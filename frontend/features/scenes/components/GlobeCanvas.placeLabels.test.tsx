import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, waitFor } from '@testing-library/react'
import GlobeCanvas from './GlobeCanvas'
import type { PlaceableScene } from './globeTypes'
import { ATLAS_COMPACT_VIEWPORT_QUERY } from '../atlasViewport'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import { GLOBE_PLACES_DATA_URL } from '../basemap/globeSurface'

/**
 * GlobeCanvas's place labels with MapLibre stubbed to a flat projection and
 * markers that report a rendered box from it (text 6px per character, 12px
 * tall), so the collision pass runs against known geometry. The rules
 * themselves are globePlaces.test's; this pins the wiring: the data loads on
 * the light look, scene labels and dots win, and nothing draws on the full
 * look or outside the label zoom range.
 */
interface Handler {
  (e?: unknown): void
}

let maps: StubMapShape[] = []
let zoom = 2.4

interface StubMapShape {
  container: HTMLDivElement
  fire: (event: string) => void
}

const PANE = { width: 390, height: 731 }
const project = ([lng, lat]: [number, number]) => ({
  x: 195 + (lng + 98) * 6,
  y: 365 - (lat - 39) * 6,
})

vi.mock('maplibre-gl', () => {
  class StubControl {
    constructor(public options: unknown) {}
  }
  class StubMarker {
    el: HTMLElement
    lngLat: [number, number] = [0, 0]
    constructor(
      public options: { element: HTMLElement; anchor?: string; offset?: [number, number] },
    ) {
      this.el = options.element
    }
    setLngLat(lngLat: [number, number]) {
      this.lngLat = lngLat
      return this
    }
    addTo(map: StubMapShape) {
      const { x, y } = project(this.lngLat)
      const width = (this.el.textContent ?? '').length * 6
      const height = 12
      const left = x - width / 2
      const top =
        this.options.anchor === 'top' ? y + (this.options.offset?.[1] ?? 0) : y - height / 2
      this.el.getBoundingClientRect = () =>
        ({ left, top, right: left + width, bottom: top + height, width, height }) as DOMRect
      map.container.appendChild(this.el)
      return this
    }
    remove() {
      this.el.remove()
    }
  }
  class StubMapImpl {
    handlers = new Map<string, Handler[]>()
    setLayoutProperty = vi.fn()
    setPaintProperty = vi.fn()
    setFeatureState = vi.fn()
    removeFeatureState = vi.fn()
    addControl = vi.fn()
    removeControl = vi.fn()
    remove = vi.fn()
    flyTo = vi.fn()
    touchZoomRotate = { disableRotation: vi.fn() }
    keyboard = { disableRotation: vi.fn() }
    canvas = document.createElement('canvas')
    container = document.createElement('div')

    constructor(public options: { style: { sources: Record<string, unknown>; layers: { id: string }[] } }) {
      Object.defineProperty(this.container, 'clientWidth', { value: PANE.width })
      Object.defineProperty(this.container, 'clientHeight', { value: PANE.height })
      document.body.appendChild(this.container)
      maps.push(this as unknown as StubMapShape)
    }
    on(event: string, layerOrHandler: unknown, maybeHandler?: unknown) {
      const handler = (typeof layerOrHandler === 'function' ? layerOrHandler : maybeHandler) as Handler
      const key = typeof layerOrHandler === 'string' ? `${event}:${layerOrHandler}` : event
      this.handlers.set(key, [...(this.handlers.get(key) ?? []), handler])
    }
    off(event: string, handler: Handler) {
      this.handlers.set(event, (this.handlers.get(event) ?? []).filter((h) => h !== handler))
    }
    once(event: string, handler: Handler) {
      this.on(event, handler)
    }
    fire(event: string) {
      for (const h of this.handlers.get(event) ?? []) h()
    }
    getLayer(id: string) {
      return this.options.style.layers.find((l) => l.id === id)
    }
    getSource(id: string) {
      return id in this.options.style.sources ? { setData: vi.fn() } : undefined
    }
    getZoom() {
      return zoom
    }
    getCenter() {
      return { lng: -98, lat: 39 }
    }
    getBounds() {
      return { contains: () => false, getWest: () => 0, getSouth: () => 0, getEast: () => 0, getNorth: () => 0 }
    }
    getCanvas() {
      return this.canvas
    }
    getContainer() {
      return this.container
    }
    project(lngLat: [number, number]) {
      return project(lngLat)
    }
    unproject([x, y]: [number, number]) {
      return { lng: (x - 195) / 6 - 98, lat: (365 - y) / 6 + 39 }
    }
  }
  return {
    Map: StubMapImpl,
    AttributionControl: StubControl,
    NavigationControl: StubControl,
    Marker: StubMarker,
    setWorkerUrl: vi.fn(),
  }
})

const PHOENIX = { lng: -112.07, lat: 33.45 }

const SCENES = [
  {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    venue_count: 9,
    // Past the continental label threshold, so the scene is labelled.
    upcoming_show_count: 200,
    total_show_count: 300,
    shows_this_week: 0,
    shows_calendar_week: 0,
    latitude: PHOENIX.lat,
    longitude: PHOENIX.lng,
  },
] as unknown as PlaceableScene[]

// Rank 0 overlaps the Phoenix scene label but not its dot; rank 1 sits on
// the dot; rank 2 is clear; rank 3 is clear of the scene but its label would
// land on rank 2's.
const PLACES: GeoJSON.FeatureCollection = {
  type: 'FeatureCollection',
  features: [
    { name: 'Under', rank: 0, at: [-109.17, 30.5] },
    { name: 'On Phoenix', rank: 1, at: [PHOENIX.lng, PHOENIX.lat + 1] },
    { name: 'Clear City', rank: 2, at: [-90, 45] },
    { name: 'Crowded', rank: 3, at: [-90.5, 45] },
  ].map(({ name, rank, at }) => ({
    type: 'Feature',
    properties: { name, rank },
    geometry: { type: 'Point', coordinates: at },
  })),
}

const POV = { lat: 39, lng: -98, altitude: 1.8 }

function placeLabelTexts() {
  return [...document.querySelectorAll('[data-testid="atlas-place-label"]')].map(
    (el) => el.textContent,
  )
}

describe('GlobeCanvas place labels', () => {
  let restoreMatchMedia: () => void = () => {}
  beforeEach(() => {
    maps = []
    zoom = 2.4
    sessionStorage.clear()
    // Measured label size: 6px per character, 12px tall. A positioned marker
    // element carries its own rect (StubMarker.addTo), which shadows this.
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (
      this: Element,
    ) {
      const width = (this.textContent ?? '').length * 6
      return { left: 0, top: 0, right: width, bottom: 12, width, height: 12 } as DOMRect
    })
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) =>
        Promise.resolve(
          url === GLOBE_PLACES_DATA_URL
            ? { ok: true, json: () => Promise.resolve(PLACES) }
            : { ok: false },
        ),
      ),
    )
  })
  afterEach(() => {
    restoreMatchMedia()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  async function showMap(compact: boolean) {
    restoreMatchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: compact }).restore
    render(
      <GlobeCanvas
        width={PANE.width}
        height={PANE.height}
        scenes={SCENES}
        pov={POV}
        onSelect={() => {}}
      />,
    )
    expect(maps).toHaveLength(1)
    await act(async () => maps[0].fire('style.load'))
    return maps[0]
  }

  it('labels the clear place and drops the ones on a scene label, a scene dot or a kept label', async () => {
    await showMap(true)
    await waitFor(() => expect(placeLabelTexts()).toEqual(['Clear City']))
    const label = document.querySelector<HTMLElement>('[data-testid="atlas-place-label"]')!
    expect(label.style.pointerEvents).toBe('none')
    expect(label.className).toContain('font-mono')
    // The scene label is still there: the scene won its spot.
    expect(document.querySelector('[data-testid="atlas-scene-label"]')?.textContent).toBe('Phoenix')
  })

  it('draws no place label on the full look', async () => {
    await showMap(false)
    await act(async () => {})
    expect(placeLabelTexts()).toEqual([])
    expect(fetch).not.toHaveBeenCalledWith(GLOBE_PLACES_DATA_URL)
  })

  it('clears the labels as soon as the zoom leaves the label range and lays them out again on the next settle', async () => {
    const map = await showMap(true)
    await waitFor(() => expect(placeLabelTexts()).toEqual(['Clear City']))
    zoom = 6
    act(() => map.fire('zoom'))
    expect(placeLabelTexts()).toEqual([])
    zoom = 3
    act(() => map.fire('zoom'))
    act(() => map.fire('moveend'))
    expect(placeLabelTexts()).toEqual(['Clear City'])
  })
})
