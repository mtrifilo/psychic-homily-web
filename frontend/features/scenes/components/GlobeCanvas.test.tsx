import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { FlyToScene, PlaceableScene, VenuePin, VenueStackMarker } from './globeTypes'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import {
  queryRenderedCircles,
  renderedCircle,
  type RenderedCircle,
} from '@/test/renderedCircles'
import { CanvasSectionErrorBoundary } from '@/components/shared/CanvasSectionErrorBoundary'
import { AtlasMapUnrecoverableError } from '../atlasViewport'
import { ATLAS_CONTEXT_RESTORE_DEADLINE_MS, ATLAS_FIRST_RENDER_DEADLINE_MS } from './atlasMapHealth'
import { MARKER_RAMPS, sceneTooltipLabel } from './globeScale'
import { venuePinPaint } from './venuePinLayer'

/**
 * MapLibre stubbed down to the seams the sheet layout drives: controls by
 * corner, DOM markers, layer events, clicks hit-tested against the drawn marks
 * (`rendered`) and the camera report. WebGL is out of scope; the browser walk
 * in the PR is what proves the canvas paints.
 */
interface StubControl {
  kind: 'attribution' | 'navigation'
}
interface StubMarker {
  element: HTMLElement
  options: Record<string, unknown>
  lngLat: [number, number] | null
  removed: boolean
}

// Hoisted with the mock factory below, which runs before this file's body.
const stub = vi.hoisted(() => {
  const state = {
    maps: [] as InstanceType<typeof StubMap>[],
    markers: [] as StubMarker[],
    // MapLibre 6 builds a map with no painter when WebGL2 is refused.
    painterless: false,
    // Each map fires style.load in a microtask its constructor queues: after
    // the task that built it, and sooner than MapLibre's next frame.
    styleLoadAfterConstruction: false,
    // Where every map projects any location: on the canvas, near side.
    projectedPoint: { x: 195, y: 300 },
  }

  type Padding = { top: number; right: number; bottom: number; left: number }

  class StubMap {
    handlers = new Map<string, ((event: unknown) => void)[]>()
    controls: { control: StubControl; position: string }[] = []
    container: HTMLElement
    canvas = document.createElement('canvas')
    painter: object | undefined = state.painterless ? undefined : {}
    touchZoomRotate = { disableRotation: vi.fn() }
    keyboard = { disableRotation: vi.fn() }
    remove = vi.fn()
    flyTo = vi.fn()
    // The camera the preview camera effect reads and moves: an ease lands at
    // once, and `moving` stands for a movement in progress.
    center = { lng: -87.63, lat: 41.88 }
    padding: Padding = { top: 0, right: 0, bottom: 0, left: 0 }
    moving = false
    easeTo = vi.fn((options: { center?: [number, number]; padding?: Padding }) => {
      if (options.center) this.center = { lng: options.center[0], lat: options.center[1] }
      if (options.padding) this.padding = options.padding
    })
    setPadding = vi.fn((padding: Padding) => {
      this.padding = padding
    })
    getPadding() {
      return this.padding
    }
    isMoving() {
      return this.moving
    }
    // A mark drawn in `rendered` has screen-px coordinates and projects to
    // them. Every other location projects to `state.projectedPoint`, and that
    // point unprojects to the camera's centre. The preview camera cases keep
    // the centre on Chicago, the one scene, so Chicago counts as facing.
    project(lngLat: [number, number]) {
      const [x, y] = lngLat
      const drawn = Object.values(this.rendered).some((marks) =>
        marks.some(({ geometry: { coordinates } }) => coordinates[0] === x && coordinates[1] === y),
      )
      return drawn ? { x, y } : state.projectedPoint
    }
    unproject() {
      return this.center
    }
    setFeatureState = vi.fn()
    removeFeatureState = vi.fn()
    setLayoutProperty = vi.fn()
    setPaintProperty = vi.fn()
    constructor(public options: { container: HTMLElement }) {
      this.container = options.container
      state.maps.push(this)
      if (state.styleLoadAfterConstruction) {
        queueMicrotask(() => this.fire('style.load'))
      }
    }
    private add(key: string, handler: (event: unknown) => void) {
      this.handlers.set(key, [...(this.handlers.get(key) ?? []), handler])
    }
    on(event: string, layerOrHandler: unknown, maybeHandler?: unknown) {
      if (typeof layerOrHandler === 'string') {
        this.add(`${event}:${layerOrHandler}`, maybeHandler as (e: unknown) => void)
      } else {
        this.add(event, layerOrHandler as (e: unknown) => void)
      }
      return this
    }
    // As MapLibre's: the handler runs on the next event only, and `off` with
    // the handler given to `once` removes it before then.
    once(event: string, handler: (e: unknown) => void) {
      const wrapped = Object.assign(
        (e: unknown) => {
          this.off(event, handler)
          handler(e)
        },
        { onceOf: handler },
      )
      this.add(event, wrapped)
      return this
    }
    off(event: string, handler: (e: unknown) => void) {
      this.handlers.set(
        event,
        (this.handlers.get(event) ?? []).filter(
          (h) => h !== handler && (h as { onceOf?: unknown }).onceOf !== handler,
        ),
      )
      return this
    }
    // The basemap's tiles never finish here, so the first full render stays
    // pending; the map's own scene source is loaded.
    isStyleLoaded() {
      return false
    }
    isSourceLoaded = vi.fn<(id: string) => boolean>(() => true)
    // The marks each layer draws, top of draw first (see renderedCircles).
    rendered: Record<string, RenderedCircle[]> = {}
    queryRenderedFeatures(
      region: [number, number] | [[number, number], [number, number]],
      options: { layers: string[] },
    ) {
      return queryRenderedCircles(this.rendered, region, options)
    }
    // A layer event, or a map mouse event (one with a point), carries its DOM
    // event, as MapLibre's do: the canvas unless the test gives another.
    fire(key: string, event?: unknown) {
      const withOriginal =
        event &&
        typeof event === 'object' &&
        !('originalEvent' in event) &&
        (key.includes(':') || 'point' in event)
          ? { ...event, originalEvent: { target: this.canvas } }
          : event
      for (const h of this.handlers.get(key) ?? []) h(withOriginal)
    }
    addControl(control: StubControl, position: string) {
      this.controls.push({ control, position })
    }
    removeControl(control: StubControl) {
      this.controls = this.controls.filter((c) => c.control !== control)
    }
    getSource() {
      return { setData: vi.fn() }
    }
    getLayer() {
      return {}
    }
    getZoom() {
      return 13
    }
    getCenter() {
      return this.center
    }
    getBounds() {
      return {
        getWest: () => -87.7,
        getSouth: () => 41.8,
        getEast: () => -87.5,
        getNorth: () => 41.95,
        contains: () => true,
      }
    }
    getContainer() {
      return this.container
    }
    getCanvas() {
      return this.canvas
    }
  }

  class AttributionControl {
    kind = 'attribution'
  }
  class NavigationControl {
    kind = 'navigation'
  }
  class Marker implements StubMarker {
    element: HTMLElement
    lngLat: [number, number] | null = null
    removed = false
    constructor(public options: Record<string, unknown> & { element: HTMLElement }) {
      this.element = options.element
      state.markers.push(this)
    }
    setLngLat(lngLat: [number, number]) {
      this.lngLat = lngLat
      return this
    }
    addTo(map: StubMap) {
      map.container.appendChild(this.element)
      return this
    }
    remove() {
      this.removed = true
      this.element.remove()
    }
  }

  return { state, StubMap, AttributionControl, NavigationControl, Marker }
})
type StubMap = InstanceType<typeof stub.StubMap>

vi.mock('maplibre-gl', () => ({
  Map: stub.StubMap,
  Marker: stub.Marker,
  AttributionControl: stub.AttributionControl,
  NavigationControl: stub.NavigationControl,
  setWorkerUrl: vi.fn(),
}))
vi.mock('maplibre-gl/dist/maplibre-gl.css', () => ({}))

import GlobeCanvas, { LABEL_PAPER_GROUND_ZOOM } from './GlobeCanvas'
import { lightGroundPaperZoom } from '../basemap/phBasemap'

const CHICAGO: PlaceableScene = {
  city: 'Chicago',
  state: 'IL',
  slug: 'chicago-il',
  venue_count: 2,
  upcoming_show_count: 10,
  total_show_count: 10,
  // Zero, so no pulse ring starts an animation loop under jsdom.
  shows_this_week: 0,
  shows_calendar_week: 0,
  latitude: 41.88,
  longitude: -87.63,
} as PlaceableScene

const PINS: VenuePin[] = [
  { id: 1, name: 'Empty Bottle', lng: -87.63, lat: 41.88, upcomingShowCount: 9, nextShowLabel: '' },
  { id: 2, name: 'Hideout', lng: -87.63, lat: 41.88, upcomingShowCount: 4, nextShowLabel: '' },
  { id: 3, name: 'Thalia Hall', lng: -87.657, lat: 41.857, upcomingShowCount: 6, nextShowLabel: '' },
]
const STACK: VenueStackMarker = {
  key: '-87.630000,41.880000',
  lng: -87.63,
  lat: 41.88,
  venueIds: [1, 2],
  label: '2 venues · city centre',
  scopeLine: '2 venues at the city centre point',
}

type CanvasProps = Partial<Parameters<typeof GlobeCanvas>[0]>

function renderCanvas(props: CanvasProps = {}) {
  const all = {
    width: 390,
    height: 723,
    scenes: [CHICAGO],
    lightGlobe: false,
    pov: { lat: 41.88, lng: -87.63, altitude: 1.6 },
    onSelect: vi.fn(),
    venues: PINS,
    cityLabel: 'Chicago, IL',
    ...props,
  }
  const utils = render(<GlobeCanvas {...all} />)
  const map = stub.state.maps[stub.state.maps.length - 1]
  act(() => map.fire('style.load'))
  return { ...utils, map, props: all }
}

/** A mark drawn centred on screen point (x, y), with no rim. */
const mark = (properties: Record<string, unknown>, x: number, y: number, radiusPx = 7) =>
  renderedCircle({ x, y, radiusPx, properties })

/** Draws the stacked pair at (10, 10), Hideout on top, and Thalia Hall alone at (100, 100). */
function drawPins(map: StubMap) {
  map.rendered['venue-pins'] = [mark({ id: 2 }, 10, 10), mark({ id: 1 }, 10, 10), mark({ id: 3 }, 100, 100)]
}

/** A click (or a tap's click) on the map at screen point (x, y). */
function clickAt(map: StubMap, x: number, y: number) {
  act(() => map.fire('click', { point: { x, y } }))
}

function attributionCorners(map: StubMap) {
  return map.controls
    .filter((c) => c.control.kind === 'attribution')
    .map((c) => c.position)
}

describe('GlobeCanvas sheet-layout seams', () => {
  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    sessionStorage.clear()
  })

  // watchAtlasMapHealth learns the style is live only from style.load, so it
  // has to be attached in the task that builds the map.
  it('sees a style.load that fires right after the task that built the map', async () => {
    stub.state.styleLoadAfterConstruction = true
    try {
      render(
        <GlobeCanvas
          width={390}
          height={723}
          scenes={[CHICAGO]}
          lightGlobe={false}
          pov={{ lat: 41.88, lng: -87.63, altitude: 1.6 }}
          onSelect={vi.fn()}
          venues={PINS}
          cityLabel="Chicago, IL"
          venueStacks={[STACK]}
          onVenueStackSelect={vi.fn()}
        />,
      )
      await act(async () => {})
      // The stack markers mount only once the style is live.
      expect(screen.getByRole('button', { name: /2 venues · city centre/ })).toBeInTheDocument()
    } finally {
      stub.state.styleLoadAfterConstruction = false
    }
  })

  it('docks the attribution bottom-left by default', () => {
    const { map } = renderCanvas()
    expect(attributionCorners(map)).toEqual(['bottom-left'])
    expect(screen.getByTestId('globe-cursor-wrap')).not.toHaveAttribute('data-atlas-credit')
  })

  it('moves the attribution top-left on request, keeping exactly one', () => {
    const { map, rerender, props } = renderCanvas({ attributionPosition: 'top-left' })
    expect(attributionCorners(map)).toEqual(['top-left'])
    expect(screen.getByTestId('globe-cursor-wrap')).toHaveAttribute('data-atlas-credit', 'top')

    rerender(<GlobeCanvas {...props} attributionPosition="bottom-left" />)
    expect(attributionCorners(map)).toEqual(['bottom-left'])
  })

  it('shows the back-to-globe control in city view only when asked', () => {
    const onBackToGlobe = vi.fn()
    const { rerender, props } = renderCanvas({ onBackToGlobe })
    fireEvent.click(screen.getByRole('button', { name: 'Back to globe' }))
    expect(onBackToGlobe).toHaveBeenCalledTimes(1)

    rerender(<GlobeCanvas {...props} cityLabel={null} />)
    expect(screen.queryByRole('button', { name: 'Back to globe' })).not.toBeInTheDocument()

    rerender(<GlobeCanvas {...props} onBackToGlobe={undefined} />)
    expect(screen.queryByRole('button', { name: 'Back to globe' })).not.toBeInTheDocument()
  })

  it('draws one counted marker per stack that reports its key', () => {
    const onVenueStackSelect = vi.fn()
    renderCanvas({ venueStacks: [STACK], onVenueStackSelect })
    const marker = screen.getByRole('button', { name: /2 venues · city centre/ })
    expect(marker).toHaveTextContent('2')
    expect(marker).toHaveAttribute('aria-label', '2 venues · city centre')
    fireEvent.click(marker)
    expect(onVenueStackSelect).toHaveBeenCalledWith(STACK.key)
  })

  it('routes a tap on a stacked pin to the stack, and a lone pin to the venue', () => {
    const onVenueSelect = vi.fn()
    const onVenueStackSelect = vi.fn()
    const { map } = renderCanvas({
      venueStacks: [STACK],
      onVenueStackSelect,
      onVenueSelect,
    })
    drawPins(map)
    clickAt(map, 10, 10)
    expect(onVenueStackSelect).toHaveBeenCalledWith(STACK.key)
    expect(onVenueSelect).not.toHaveBeenCalled()

    clickAt(map, 100, 100)
    expect(onVenueSelect).toHaveBeenCalledWith(3)
  })

  it('opens a stacked pin as a venue when no stack handler is given', () => {
    const onVenueSelect = vi.fn()
    const { map } = renderCanvas({ onVenueSelect })
    drawPins(map)
    clickAt(map, 10, 10)
    expect(onVenueSelect).toHaveBeenCalledWith(2)
  })

  it('titles the tooltip of a pin that opens its stack by the stack alone', () => {
    const { map } = renderCanvas({ venueStacks: [STACK], onVenueStackSelect: vi.fn() })
    act(() =>
      map.fire('mousemove:venue-pins', {
        point: { x: 10, y: 10 },
        features: [{ properties: { id: 2 } }, { properties: { id: 1 } }],
      }),
    )
    const tooltip = screen.getByTestId('atlas-venue-tooltip')
    const cta = screen.getByText('click for shows', { exact: false })
    expect(tooltip).toHaveStyle({ display: 'block' })
    expect(tooltip).toHaveTextContent('2 venues at the city centre point')
    expect(tooltip).not.toHaveTextContent('Hideout')
    expect(tooltip).not.toHaveTextContent('upcoming')
    expect(cta).not.toBeVisible()

    // A lone pin next: its name, its counts and the CTA come back, visible.
    act(() =>
      map.fire('mousemove:venue-pins', {
        point: { x: 10, y: 10 },
        features: [{ properties: { id: 3 } }],
      }),
    )
    expect(tooltip).toHaveTextContent('Thalia Hall')
    expect(screen.getByText('6 upcoming')).toBeVisible()
    expect(cta).toBeVisible()
  })

  it('names the top venue of a stacked pin when no stack handler is given, as its click opens it', () => {
    const onVenueSelect = vi.fn()
    const { map } = renderCanvas({ venueStacks: [STACK], onVenueSelect })
    const stacked = {
      point: { x: 10, y: 10 },
      features: [{ properties: { id: 2 } }, { properties: { id: 1 } }],
    }
    act(() => map.fire('mousemove:venue-pins', stacked))
    const tooltip = screen.getByTestId('atlas-venue-tooltip')
    expect(tooltip).toHaveTextContent('Hideout')
    expect(tooltip).toHaveTextContent('4 upcoming')
    expect(tooltip).not.toHaveTextContent('2 venues')

    drawPins(map)
    clickAt(map, 10, 10)
    expect(onVenueSelect).toHaveBeenCalledWith(2)
  })

  it('titles the tooltip by the stack while the pointer is over its marker, whatever pin lies under it', () => {
    const { map } = renderCanvas({ venueStacks: [STACK], onVenueStackSelect: vi.fn() })
    const marker = screen.getByRole('button', { name: /2 venues · city centre/ })
    // The marker's label overlaps the lone Thalia Hall pin.
    act(() =>
      map.fire('mousemove:venue-pins', {
        point: { x: 10, y: 10 },
        features: [{ properties: { id: 3 } }],
        originalEvent: { target: marker.lastElementChild },
      }),
    )
    const tooltip = screen.getByTestId('atlas-venue-tooltip')
    expect(tooltip).toHaveTextContent('2 venues at the city centre point')
    expect(tooltip).not.toHaveTextContent('Thalia Hall')
    expect(screen.getByText('click for shows', { exact: false })).not.toBeVisible()
  })

  it('clears a held pin tooltip when the stack handler or the stacks change', () => {
    const { map, rerender, props } = renderCanvas({
      venueStacks: [STACK],
      onVenueStackSelect: vi.fn(),
    })
    const hoverStacked = () =>
      act(() =>
        map.fire('mousemove:venue-pins', {
          point: { x: 10, y: 10 },
          features: [{ properties: { id: 2 } }, { properties: { id: 1 } }],
        }),
      )
    const tooltip = screen.getByTestId('atlas-venue-tooltip')
    hoverStacked()
    expect(tooltip).toHaveTextContent('2 venues at the city centre point')

    rerender(<GlobeCanvas {...props} onVenueStackSelect={undefined} />)
    expect(tooltip).toHaveStyle({ display: 'none' })

    hoverStacked()
    expect(tooltip).toHaveTextContent('Hideout')
    rerender(<GlobeCanvas {...props} onVenueStackSelect={undefined} venueStacks={[]} />)
    expect(tooltip).toHaveStyle({ display: 'none' })
  })

  it('names stacked pins by their marker, not by a member label', () => {
    renderCanvas({ venueStacks: [STACK], onVenueStackSelect: vi.fn() })
    const labels = stub.state.markers
      .filter((m) => !m.removed && m.element.tagName === 'DIV')
      .map((m) => m.element.textContent)
    expect(labels).toContain('Thalia Hall')
    expect(labels).not.toContain('Empty Bottle')
    expect(labels).not.toContain('Hideout')
  })

  it('reports the viewport bounds with each camera settle', () => {
    const onCameraSettle = vi.fn()
    const { map } = renderCanvas({ onCameraSettle })
    act(() => map.fire('moveend'))
    expect(onCameraSettle).toHaveBeenLastCalledWith({
      lng: -87.63,
      lat: 41.88,
      zoom: 13,
      bounds: { west: -87.7, south: 41.8, east: -87.5, north: 41.95 },
    })
  })
})

describe('GlobeCanvas hover on a coarse pointer', () => {
  // The literal query, not an import of it: a drifted query must fail here.
  const COARSE_POINTER_QUERY = '(pointer: coarse)'
  let matchMedia: ReturnType<typeof installMatchMedia> | null = null

  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    sessionStorage.clear()
  })
  afterEach(() => {
    matchMedia?.restore()
    matchMedia = null
  })

  const PIN_MOVE = { point: { x: 10, y: 10 }, features: [{ properties: { id: 3 } }] }
  const DOT_MOVE = {
    point: { x: 10, y: 10 },
    features: [{ properties: { slug: 'chicago-il', sortKey: 1 } }],
  }

  it('shows no tooltip, sets no highlight and never sets the pointer cursor for a pin or a dot', () => {
    matchMedia = installMatchMedia({ [COARSE_POINTER_QUERY]: true })
    const { map } = renderCanvas()
    act(() => map.fire('mousemove:venue-pins', PIN_MOVE))
    act(() => map.fire('mousemove:scene-dots', DOT_MOVE))

    expect(screen.getByTestId('atlas-venue-tooltip')).toHaveStyle({ display: 'none' })
    expect(screen.queryByText(sceneTooltipLabel(CHICAGO))).not.toBeInTheDocument()
    expect(map.setFeatureState).not.toHaveBeenCalled()
    expect(map.getCanvas().style.cursor).toBe('')
  })

  it('still opens a tapped pin as its venue, or as its stack', () => {
    matchMedia = installMatchMedia({ [COARSE_POINTER_QUERY]: true })
    const onVenueSelect = vi.fn()
    const onVenueStackSelect = vi.fn()
    const { map } = renderCanvas({ venueStacks: [STACK], onVenueSelect, onVenueStackSelect })
    drawPins(map)
    act(() => map.fire('mousemove:venue-pins', PIN_MOVE))
    clickAt(map, 100, 100)
    expect(onVenueSelect).toHaveBeenCalledWith(3)

    clickAt(map, 10, 10)
    expect(onVenueStackSelect).toHaveBeenCalledWith(STACK.key)
    expect(screen.getByTestId('atlas-venue-tooltip')).toHaveStyle({ display: 'none' })
  })

  it('clears a held hover when the pointer turns coarse, and hovers again when it turns fine', () => {
    matchMedia = installMatchMedia({ [COARSE_POINTER_QUERY]: false })
    const { map } = renderCanvas()
    act(() => map.fire('mousemove:venue-pins', PIN_MOVE))
    act(() => map.fire('mousemove:scene-dots', DOT_MOVE))
    const tooltip = screen.getByTestId('atlas-venue-tooltip')
    expect(tooltip).toHaveStyle({ display: 'block' })
    expect(screen.getByText(sceneTooltipLabel(CHICAGO))).toHaveStyle({ display: 'block' })
    expect(map.setFeatureState).toHaveBeenCalledTimes(2)

    matchMedia!.set(COARSE_POINTER_QUERY, true)
    expect(tooltip).toHaveStyle({ display: 'none' })
    expect(screen.getByText(sceneTooltipLabel(CHICAGO))).toHaveStyle({ display: 'none' })
    expect(map.removeFeatureState).toHaveBeenCalledWith({ source: 'venues', id: 3 }, 'hover')
    expect(map.removeFeatureState).toHaveBeenCalledWith(
      { source: 'scenes', id: 'chicago-il' },
      'hover',
    )
    expect(map.getCanvas().style.cursor).toBe('')
    act(() => map.fire('mousemove:venue-pins', PIN_MOVE))
    expect(tooltip).toHaveStyle({ display: 'none' })
    expect(map.setFeatureState).toHaveBeenCalledTimes(2)

    matchMedia!.set(COARSE_POINTER_QUERY, false)
    act(() => map.fire('mousemove:venue-pins', PIN_MOVE))
    expect(tooltip).toHaveStyle({ display: 'block' })
    expect(map.setFeatureState).toHaveBeenCalledTimes(3)
  })
})

describe('GlobeCanvas tap targets', () => {
  // The literal query, not an import of it: a drifted query must fail here.
  const COARSE_POINTER_QUERY = '(pointer: coarse)'
  let matchMedia: ReturnType<typeof installMatchMedia> | null = null

  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    sessionStorage.clear()
  })
  afterEach(() => {
    matchMedia?.restore()
    matchMedia = null
  })

  function renderWithPointer(coarse: boolean, props: CanvasProps = {}) {
    matchMedia = installMatchMedia({ [COARSE_POINTER_QUERY]: coarse })
    const onVenueSelect = vi.fn()
    const onSelect = vi.fn()
    const { map } = renderCanvas({ onVenueSelect, onSelect, ...props })
    return { map, onVenueSelect, onSelect }
  }

  // drawPins draws Thalia Hall at (100, 100) with radius 7; a tap at 117 is 10px off its edge
  // and inside the 24px box's 12px reach.
  it.each([
    { pointer: 'coarse', coarse: true, opens: [[3]] },
    { pointer: 'fine', coarse: false, opens: [] },
  ])('a click 10px off a pin\'s edge on a $pointer pointer opens it only when coarse', ({ coarse, opens }) => {
    const { map, onVenueSelect } = renderWithPointer(coarse)
    drawPins(map)
    clickAt(map, 117, 100)
    expect(onVenueSelect.mock.calls).toEqual(opens)
  })

  it('opens nothing from a coarse tap beyond the box', () => {
    const { map, onVenueSelect } = renderWithPointer(true)
    drawPins(map)
    // 13px off the pin's edge: the box's near side is 1px short of the pin.
    clickAt(map, 120, 100)
    expect(onVenueSelect).not.toHaveBeenCalled()
  })

  it('opens the nearer of two pins from a coarse tap between them', () => {
    const { map, onVenueSelect } = renderWithPointer(true)
    map.rendered['venue-pins'] = [mark({ id: 3 }, 100, 100), mark({ id: 2 }, 124, 100)]
    clickAt(map, 110, 100)
    expect(onVenueSelect).toHaveBeenLastCalledWith(3)
    clickAt(map, 114, 100)
    expect(onVenueSelect).toHaveBeenLastCalledWith(2)
  })

  it('opens the pin a coarse tap lands on, though another pin has the nearer centre', () => {
    const { map, onVenueSelect } = renderWithPointer(true)
    // The tap at 109 is inside the 11px pin (centre 9px away) and 4px off the
    // edge of the 3px pin (centre 7px away).
    map.rendered['venue-pins'] = [mark({ id: 3 }, 100, 100, 11), mark({ id: 2 }, 116, 100, 3)]
    clickAt(map, 109, 100)
    expect(onVenueSelect).toHaveBeenCalledWith(3)
  })

  it('opens the top pin of a stack a coarse tap lands on, as a fine click does', () => {
    const { map, onVenueSelect } = renderWithPointer(true)
    drawPins(map)
    clickAt(map, 12, 10)
    expect(onVenueSelect).toHaveBeenCalledWith(2)
  })

  /**
   * Renders Chicago and Evanston as two 4px dots stacked at (50, 50), Evanston
   * listed first and Chicago carrying the higher sort key.
   */
  function renderStackedDots(coarse: boolean) {
    const rendered = renderWithPointer(coarse, {
      scenes: [CHICAGO, { ...CHICAGO, slug: 'evanston-il', city: 'Evanston' }],
    })
    rendered.map.rendered['scene-dots'] = [
      mark({ slug: 'evanston-il', sortKey: 1 }, 50, 50, 4),
      mark({ slug: 'chicago-il', sortKey: 5 }, 50, 50, 4),
    ]
    return rendered
  }

  it('opens a scene from a coarse tap off its dot, and the highest sort key of a stacked hit', () => {
    const { map, onSelect } = renderStackedDots(true)
    clickAt(map, 62, 50)
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ slug: 'chicago-il' }))
  })

  it('opens the highest sort key of a stacked dot hit on a fine pointer, and nothing off the dot', () => {
    const { map, onSelect } = renderStackedDots(false)
    clickAt(map, 62, 50)
    expect(onSelect).not.toHaveBeenCalled()
    clickAt(map, 51, 50)
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ slug: 'chicago-il' }))
  })

  it('names the highest sort key of a stacked dot hover, as its click opens it', () => {
    renderWithPointer(false, {
      scenes: [CHICAGO, { ...CHICAGO, slug: 'evanston-il', city: 'Evanston' }],
    })
    const map = stub.state.maps[stub.state.maps.length - 1]
    act(() =>
      map.fire('mousemove:scene-dots', {
        point: { x: 50, y: 50 },
        features: [
          { properties: { slug: 'evanston-il', sortKey: 1 } },
          { properties: { slug: 'chicago-il', sortKey: 5 } },
        ],
      }),
    )
    expect(screen.getByText(sceneTooltipLabel(CHICAGO))).toHaveStyle({ display: 'block' })
    expect(map.setFeatureState).toHaveBeenCalledWith({ source: 'scenes', id: 'chicago-il' }, { hover: true })
  })

  it('opens nothing, on either pointer, while the style has no pin or dot layer', () => {
    for (const coarse of [true, false]) {
      const { map, onVenueSelect, onSelect } = renderWithPointer(coarse)
      drawPins(map)
      map.rendered['scene-dots'] = [mark({ slug: 'chicago-il', sortKey: 1 }, 100, 100)]
      map.getLayer = () => undefined as unknown as object
      clickAt(map, 100, 100)
      expect(onVenueSelect).not.toHaveBeenCalled()
      expect(onSelect).not.toHaveBeenCalled()
      cleanup()
      matchMedia?.restore()
      matchMedia = null
    }
  })
})

describe('GlobeCanvas pulse rings', () => {
  const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'
  let matchMedia: ReturnType<typeof installMatchMedia>

  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    sessionStorage.clear()
  })
  afterEach(() => {
    matchMedia.restore()
    vi.restoreAllMocks()
  })

  it('stops the rings when reduced motion is turned on mid-session', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
    const setData = vi.fn()
    vi.spyOn(stub.StubMap.prototype, 'getSource').mockReturnValue({ setData })
    vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(7)
    const cancel = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    renderCanvas({ scenes: [{ ...CHICAGO, shows_this_week: 2 }] })

    const ringFeatureCounts = () =>
      setData.mock.calls.map(([fc]) => (fc as { features: unknown[] }).features.length)
    expect(ringFeatureCounts()).toContain(1)
    setData.mockClear()

    matchMedia.set(REDUCED_MOTION_QUERY, true)
    expect(cancel).toHaveBeenCalledWith(7)
    expect(ringFeatureCounts()).toContain(0)
    expect(ringFeatureCounts()).not.toContain(1)
  })

  it('stops the rings while the context is lost and restarts them on the restored style', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: false })
    const setData = vi.fn()
    vi.spyOn(stub.StubMap.prototype, 'getSource').mockReturnValue({ setData })
    const raf = vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(7)
    const cancel = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const { map } = renderCanvas({ scenes: [{ ...CHICAGO, shows_this_week: 2 }] })
    expect(raf).toHaveBeenCalledTimes(1)

    act(() => map.fire('webglcontextlost'))
    expect(cancel).toHaveBeenCalledWith(7)
    act(() => map.fire('webglcontextrestored'))
    expect(raf).toHaveBeenCalledTimes(1)

    setData.mockClear()
    act(() => map.fire('style.load'))
    expect(raf).toHaveBeenCalledTimes(2)
    // The restored style gets the current scenes and rings again.
    const featureCounts = setData.mock.calls.map(
      ([fc]) => (fc as { features: unknown[] }).features.length,
    )
    expect(featureCounts.filter((n) => n === 1).length).toBeGreaterThanOrEqual(2)
  })

  it('starts the rings again when reduced motion is turned off', () => {
    matchMedia = installMatchMedia({ [REDUCED_MOTION_QUERY]: true })
    const setData = vi.fn()
    vi.spyOn(stub.StubMap.prototype, 'getSource').mockReturnValue({ setData })
    const raf = vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(7)
    renderCanvas({ scenes: [{ ...CHICAGO, shows_this_week: 2 }] })
    expect(raf).not.toHaveBeenCalled()

    matchMedia.set(REDUCED_MOTION_QUERY, false)
    expect(raf).toHaveBeenCalled()
    expect(
      setData.mock.calls.map(([fc]) => (fc as { features: unknown[] }).features.length),
    ).toContain(1)
  })
})

/**
 * The canvas inside the error boundary AtlasGlobe wraps it in, with a style
 * load fired or not. Console errors are silenced: React logs every error a
 * boundary catches.
 */
function renderInBoundary({
  styleLoaded,
  venues,
}: {
  styleLoaded: boolean
  venues?: VenuePin[]
}) {
  vi.spyOn(console, 'error').mockImplementation(() => {})
  const onError = vi.fn()
  const onCameraSettle = vi.fn()
  render(
    <CanvasSectionErrorBoundary sentryTag="atlas-map-test" onError={onError}>
      <GlobeCanvas
        width={390}
        height={723}
        scenes={[CHICAGO]}
        lightGlobe={false}
        pov={{ lat: 41.88, lng: -87.63, altitude: 1.6 }}
        onSelect={vi.fn()}
        onCameraSettle={onCameraSettle}
        venues={venues}
      />
    </CanvasSectionErrorBoundary>,
  )
  const map = stub.state.maps[stub.state.maps.length - 1]
  if (styleLoaded) {
    act(() => {
      map.fire('style.load')
      map.fire('render')
    })
  }
  return { map, onError, onCameraSettle }
}

function expectFellBack(onError: ReturnType<typeof vi.fn>, failureClass: string) {
  expect(onError).toHaveBeenCalledTimes(1)
  expect(onError.mock.calls[0][0]).toBeInstanceOf(AtlasMapUnrecoverableError)
  expect(onError.mock.calls[0][0]).toMatchObject({ failureClass })
  expect(screen.queryByTestId('globe-cursor-wrap')).not.toBeInTheDocument()
}

describe('GlobeCanvas without a WebGL2 context', () => {
  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    stub.state.painterless = true
    sessionStorage.clear()
  })
  afterEach(() => {
    stub.state.painterless = false
    vi.restoreAllMocks()
  })

  it('throws to its error boundary when MapLibre comes up without a painter', () => {
    const { onError } = renderInBoundary({ styleLoaded: false })
    expectFellBack(onError, 'context-refused')
  })
})

describe('GlobeCanvas failures MapLibre reports outside React', () => {
  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    sessionStorage.clear()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('falls back when no frame renders the scene data by the first-render deadline', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: false })
    map.isSourceLoaded.mockReturnValue(false)
    act(() => {
      map.fire('style.load')
      map.fire('render')
    })
    act(() => vi.advanceTimersByTime(ATLAS_FIRST_RENDER_DEADLINE_MS - 1))
    expect(onError).not.toHaveBeenCalled()
    act(() => vi.advanceTimersByTime(1))
    expectFellBack(onError, 'first-render-timeout')
    expect(map.isSourceLoaded).toHaveBeenCalledWith('scenes')
  })

  it('keeps the map once a frame renders the scene data', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: false })
    act(() => {
      map.fire('style.load')
      map.fire('render')
    })
    act(() => vi.advanceTimersByTime(ATLAS_FIRST_RENDER_DEADLINE_MS * 10))
    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()
  })

  it('stops the first-render watchdog when the map is removed', () => {
    renderInBoundary({ styleLoaded: false })
    expect(vi.getTimerCount()).toBeGreaterThan(0)
    cleanup()
    // The watchdog's deadline was the one timer pending.
    expect(vi.getTimerCount()).toBe(0)
  })

  it('asks for no source on a frame before the style loads', () => {
    // MapLibre answers a source it has not registered with an error event,
    // which the health watch would read as a failed style.
    const { map, onError } = renderInBoundary({ styleLoaded: false })
    act(() => map.fire('render'))
    expect(map.isSourceLoaded).not.toHaveBeenCalled()
    expect(onError).not.toHaveBeenCalled()
  })

  it('reports the first failure when the health watch and the watchdog both report before a render', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: false })
    act(() => {
      map.fire('error', { error: new Error('style is invalid') })
      vi.advanceTimersByTime(ATLAS_FIRST_RENDER_DEADLINE_MS)
    })
    expectFellBack(onError, 'style-load-failed')
  })

  it('falls back when a lost context is not restored by the deadline', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: true })
    act(() => map.fire('webglcontextlost'))
    act(() => vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS - 1))
    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()

    act(() => vi.advanceTimersByTime(1))
    expectFellBack(onError, 'context-lost')
    expect(map.remove).toHaveBeenCalledTimes(1)
  })

  it('keeps the map when a lost context is restored', () => {
    const { map, onError, onCameraSettle } = renderInBoundary({ styleLoaded: true })
    act(() => map.fire('webglcontextlost'))
    act(() => vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS / 2))
    act(() => map.fire('webglcontextrestored'))
    act(() => map.fire('style.load'))
    act(() => vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10))

    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()
    expect(map.remove).not.toHaveBeenCalled()
    onCameraSettle.mockClear()
    act(() => map.fire('moveend'))
    expect(onCameraSettle).toHaveBeenCalledTimes(1)
  })

  it('falls back when the restore gets no WebGL2 context', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: true })
    act(() => map.fire('webglcontextlost'))
    const refused = new Error('WebGL2 is required to display this map.')
    refused.name = 'GPUInitializationError'
    act(() => map.fire('error', { error: refused }))
    // MapLibre still fires the restored event after a refused restore.
    act(() => map.fire('webglcontextrestored'))
    expectFellBack(onError, 'context-restore-refused')
  })

  it('falls back when the context is lost before the style loads', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: false })
    act(() => map.fire('webglcontextlost'))
    expectFellBack(onError, 'context-lost-before-style')
  })

  it('falls back on an error event during the first render, before the style loads', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: false })
    act(() => map.fire('error', { error: new Error('style is not valid') }))
    expectFellBack(onError, 'style-load-failed')
  })

  it('keeps the map on an error event after the style loaded', () => {
    const { map, onError, onCameraSettle } = renderInBoundary({ styleLoaded: true })
    act(() => map.fire('error', { error: new Error('Cannot style non-existing layer') }))
    act(() => map.fire('error', { error: new Error('AJAXError: 503'), sourceId: 'nightEarth' }))

    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()
    expect(map.remove).not.toHaveBeenCalled()
    onCameraSettle.mockClear()
    act(() => map.fire('moveend'))
    expect(onCameraSettle).toHaveBeenCalledTimes(1)
  })

  it('stops the health watch when the canvas unmounts', () => {
    const { map } = renderInBoundary({ styleLoaded: true })
    act(() => map.fire('webglcontextlost'))
    // The watch's page listener: only its disposer removes one.
    const removeListener = vi.spyOn(document, 'removeEventListener')
    cleanup()
    expect(removeListener).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
  })

  /** Feature-state writes throw, as MapLibre's do while the style is down. */
  function styleDown(map: StubMap) {
    const throwing = () => {
      throw new TypeError("Cannot read properties of null (reading 'removeFeatureState')")
    }
    map.removeFeatureState.mockImplementation(throwing)
    map.setFeatureState.mockImplementation(throwing)
  }
  function styleUp(map: StubMap) {
    map.removeFeatureState.mockReset()
    map.setFeatureState.mockReset()
  }

  it('keeps the map through a restore with a scene dot hovered', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: true })
    act(() =>
      map.fire('mousemove:scene-dots', {
        point: { x: 10, y: 10 },
        features: [{ properties: { slug: 'chicago-il', sortKey: 1 } }],
      }),
    )
    expect(map.setFeatureState).toHaveBeenCalledTimes(1)
    const tooltip = screen.getByText(sceneTooltipLabel(CHICAGO))
    expect(tooltip).toHaveStyle({ display: 'block' })

    styleDown(map)
    act(() => map.fire('webglcontextlost'))
    // The dot went with the style, and so does its tooltip.
    expect(tooltip).toHaveStyle({ display: 'none' })
    // During the loss the pointer leaves the dot; the restore's resize fires
    // movestart before the restored style loads.
    act(() => map.fire('mouseleave:scene-dots'))
    act(() => map.fire('movestart'))
    act(() => map.fire('webglcontextrestored'))
    styleUp(map)
    act(() => map.fire('style.load'))
    act(() => vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10))

    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()
    expect(map.removeFeatureState).not.toHaveBeenCalled()
  })

  it('clears a hovered venue pin and keeps the map when the context is lost', () => {
    const { map, onError } = renderInBoundary({ styleLoaded: true, venues: PINS })
    act(() =>
      map.fire('mousemove:venue-pins', {
        point: { x: 10, y: 10 },
        features: [{ properties: { id: 3 } }],
      }),
    )
    expect(map.setFeatureState).toHaveBeenCalledTimes(1)
    const tooltip = screen.getByTestId('atlas-venue-tooltip')
    expect(tooltip).toHaveStyle({ display: 'block' })

    styleDown(map)
    act(() => map.fire('webglcontextlost'))

    // The pin went with the style, and so does its tooltip.
    expect(tooltip).toHaveStyle({ display: 'none' })
    expect(map.getCanvas().style.cursor).toBe('')
    expect(onError).not.toHaveBeenCalled()
    expect(screen.getByTestId('globe-cursor-wrap')).toBeInTheDocument()
  })
})

describe('GlobeCanvas preview camera', () => {
  // The Atlas pane on a 390x844 phone and the scene preview sheet's heights
  // in it (scenePreviewSheetPx).
  const PANE = { width: 390, height: 731 }
  const HALF_PX = 328.95
  const FULL_PX = 619
  const PEEK_PX = 120
  const NO_PADDING = { top: 0, right: 0, bottom: 0, left: 0 }
  const padded = (bottom: number) => ({ ...NO_PADDING, bottom })
  const CHICAGO_CENTER: [number, number] = [CHICAGO.longitude, CHICAGO.latitude]
  // A camera standing away from the Chicago dot, as after a pan.
  const ELSEWHERE = { lng: -100, lat: 40 }

  // The next animation frame's callback, run on demand.
  let frames: FrameRequestCallback[] = []
  function runFrame() {
    const due = frames
    frames = []
    act(() => due.forEach((cb) => cb(0)))
  }

  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    stub.state.projectedPoint = { x: 195, y: 600 }
    sessionStorage.clear()
    frames = []
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
      frames.push(cb)
      return frames.length
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  function renderPreview(props: CanvasProps = {}) {
    return renderCanvas({ ...PANE, cityLabel: null, venues: [], ...props })
  }

  /** A canvas whose camera stands away from Chicago, with Chicago then selected at `detent`. */
  function selectChicago(detent: 'peek' | 'half' | 'full' = 'half') {
    const utils = renderPreview()
    utils.map.center = ELSEWHERE
    utils.rerender(
      <GlobeCanvas {...utils.props} selected={CHICAGO} scenePreviewSheetDetent={detent} />,
    )
    return utils
  }

  it('eases a newly selected dot to the centre of the band above the Half sheet, at the same zoom', () => {
    const { map } = selectChicago()
    expect(map.easeTo).toHaveBeenCalledTimes(1)
    expect(map.easeTo).toHaveBeenCalledWith({ center: CHICAGO_CENTER, padding: padded(HALF_PX) })
  })

  it('re-centres for Full, keeps the camera for a Peek that leaves the dot clear, and re-centres back at Half', () => {
    const { map, rerender, props } = selectChicago()
    map.easeTo.mockClear()
    const at = (detent: 'peek' | 'half' | 'full') => (
      <GlobeCanvas {...props} selected={CHICAGO} scenePreviewSheetDetent={detent} />
    )

    rerender(at('full'))
    expect(map.easeTo).toHaveBeenCalledTimes(1)
    expect(map.easeTo).toHaveBeenLastCalledWith({ center: CHICAGO_CENTER, padding: padded(FULL_PX) })

    // The dot sits in the band above Full, well clear of a Peek sheet: the
    // camera, padding included, stays as it is.
    stub.state.projectedPoint = { x: 195, y: 56 }
    rerender(at('peek'))
    expect(map.easeTo).toHaveBeenCalledTimes(1)
    expect(map.padding).toEqual(padded(FULL_PX))

    rerender(at('half'))
    expect(map.easeTo).toHaveBeenCalledTimes(2)
    expect(map.easeTo).toHaveBeenLastCalledWith({ center: CHICAGO_CENTER, padding: padded(HALF_PX) })
  })

  it('re-centres for Peek when the Peek sheet covers the dot', () => {
    const { map, rerender, props } = selectChicago()
    map.easeTo.mockClear()
    // Under the Peek sheet, whose edge is at 731 - 120 = 611.
    stub.state.projectedPoint = { x: 195, y: 650 }
    rerender(<GlobeCanvas {...props} selected={CHICAGO} scenePreviewSheetDetent="peek" />)
    expect(map.easeTo).toHaveBeenCalledWith({ center: CHICAGO_CENTER, padding: padded(PEEK_PX) })
  })

  it('eases the padding back to none when the preview closes, keeping the centre', () => {
    const { map, rerender, props } = selectChicago()
    map.easeTo.mockClear()
    rerender(<GlobeCanvas {...props} selected={null} scenePreviewSheetDetent="half" />)
    expect(map.easeTo).toHaveBeenCalledTimes(1)
    expect(map.easeTo).toHaveBeenCalledWith({ padding: NO_PADDING })
  })

  it('eases a newly picked scene above a sheet already at Full', () => {
    const austin = { ...CHICAGO, city: 'Austin', state: 'TX', slug: 'austin-tx', latitude: 30.27, longitude: -97.74 }
    const { map, rerender, props } = selectChicago('full')
    map.easeTo.mockClear()
    rerender(
      <GlobeCanvas
        {...props}
        scenes={[CHICAGO, austin]}
        selected={austin}
        scenePreviewSheetDetent="full"
      />,
    )
    expect(map.easeTo).toHaveBeenCalledTimes(1)
    expect(map.easeTo).toHaveBeenCalledWith({ center: [-97.74, 30.27], padding: padded(FULL_PX) })
  })

  it('eases the padding off, keeping the centre, when the preview stops being a sheet', () => {
    const { map, rerender, props } = selectChicago()
    map.easeTo.mockClear()
    rerender(<GlobeCanvas {...props} selected={CHICAGO} scenePreviewSheetDetent={undefined} />)
    expect(map.easeTo).toHaveBeenCalledTimes(1)
    expect(map.easeTo).toHaveBeenCalledWith({ padding: NO_PADDING })
  })

  it('never moves the camera for a selection whose preview is no sheet', () => {
    const { map, rerender, props } = renderPreview()
    map.center = ELSEWHERE
    rerender(<GlobeCanvas {...props} selected={CHICAGO} />)
    rerender(<GlobeCanvas {...props} selected={null} />)
    expect(map.easeTo).not.toHaveBeenCalled()
  })

  it('waits for a movement in progress to end, then for the next frame, before moving the camera', () => {
    const { map, rerender, props } = renderPreview()
    map.moving = true
    rerender(<GlobeCanvas {...props} selected={CHICAGO} scenePreviewSheetDetent="half" />)
    expect(map.easeTo).not.toHaveBeenCalled()

    // moveend can fire from inside another camera call, so nothing moves
    // until the next frame.
    map.moving = false
    act(() => map.fire('moveend'))
    expect(map.easeTo).not.toHaveBeenCalled()
    runFrame()
    expect(map.easeTo).toHaveBeenCalledTimes(1)
    expect(map.easeTo).toHaveBeenCalledWith({ center: CHICAGO_CENTER, padding: padded(HALF_PX) })
  })

  it('waits again when another movement has started by the next frame', () => {
    const { map, rerender, props } = renderPreview()
    map.moving = true
    rerender(<GlobeCanvas {...props} selected={CHICAGO} scenePreviewSheetDetent="half" />)
    act(() => map.fire('moveend'))
    runFrame()
    expect(map.easeTo).not.toHaveBeenCalled()

    map.moving = false
    act(() => map.fire('moveend'))
    runFrame()
    expect(map.easeTo).toHaveBeenCalledTimes(1)
  })

  it('leaves a flight that opens the preview to land the dot above its sheet', () => {
    const flyToRef: { current: FlyToScene | null } = { current: null }
    const { map, rerender, props } = renderPreview({
      flyToRef,
      scenePreviewSheetDetent: 'half',
    })
    act(() => flyToRef.current?.(CHICAGO, { opensPreview: true }))
    expect(map.flyTo).toHaveBeenLastCalledWith(
      expect.objectContaining({ center: CHICAGO_CENTER, padding: padded(HALF_PX) }),
    )
    rerender(<GlobeCanvas {...props} selected={CHICAGO} />)
    // The flight lands on the dot with the sheet's padding.
    map.center = { lng: CHICAGO.longitude, lat: CHICAGO.latitude }
    map.padding = padded(HALF_PX)
    act(() => map.fire('moveend'))
    runFrame()
    expect(map.easeTo).not.toHaveBeenCalled()
  })

  it('flies any other flight unpadded', () => {
    const flyToRef: { current: FlyToScene | null } = { current: null }
    const { map } = renderPreview({ flyToRef, scenePreviewSheetDetent: 'half' })
    act(() => flyToRef.current?.(CHICAGO))
    expect(map.flyTo).toHaveBeenLastCalledWith(
      expect.objectContaining({ center: CHICAGO_CENTER, padding: NO_PADDING }),
    )
  })

  it('drops the padding once a flight lands when the preview it opened closed mid-flight', () => {
    const flyToRef: { current: FlyToScene | null } = { current: null }
    const { map, rerender, props } = renderPreview({
      flyToRef,
      scenePreviewSheetDetent: 'half',
    })
    act(() => flyToRef.current?.(CHICAGO, { opensPreview: true }))
    // The flight is in progress and has taken the sheet's padding.
    map.moving = true
    map.padding = padded(HALF_PX)
    rerender(<GlobeCanvas {...props} selected={CHICAGO} />)
    rerender(<GlobeCanvas {...props} selected={null} />)
    expect(map.easeTo).not.toHaveBeenCalled()

    map.moving = false
    act(() => map.fire('moveend'))
    runFrame()
    expect(map.easeTo).toHaveBeenCalledTimes(1)
    expect(map.easeTo).toHaveBeenCalledWith({ padding: NO_PADDING })
  })

  it('finishes the padding of a re-centring a gesture stopped part-way, keeping the centre', () => {
    const { map } = selectChicago()
    map.easeTo.mockClear()
    // A drag stopped the ease 40% of the way, then moved the camera on.
    map.padding = padded(HALF_PX * 0.4)
    map.center = ELSEWHERE
    act(() => map.fire('moveend'))
    runFrame()
    expect(map.easeTo).toHaveBeenCalledTimes(1)
    expect(map.easeTo).toHaveBeenCalledWith({ padding: padded(HALF_PX) })
    expect(map.center).toEqual(ELSEWHERE)
  })

  it('finishes dropping the padding of a closed preview that a gesture stopped part-way', () => {
    const { map, rerender, props } = selectChicago()
    rerender(<GlobeCanvas {...props} selected={null} scenePreviewSheetDetent="half" />)
    map.easeTo.mockClear()
    map.padding = padded(60)
    act(() => map.fire('moveend'))
    runFrame()
    expect(map.easeTo).toHaveBeenCalledWith({ padding: NO_PADDING })
  })

  it('leaves the padding the Peek exception kept, and waits while the camera still moves', () => {
    const { map, rerender, props } = selectChicago()
    rerender(<GlobeCanvas {...props} selected={CHICAGO} scenePreviewSheetDetent="peek" />)
    map.easeTo.mockClear()
    act(() => map.fire('moveend'))
    runFrame()
    expect(map.easeTo).not.toHaveBeenCalled()

    map.padding = padded(10)
    map.moving = true
    act(() => map.fire('moveend'))
    runFrame()
    expect(map.easeTo).not.toHaveBeenCalled()
  })

  it('keeps the camera a user panned through a lost and restored context', () => {
    const { map } = selectChicago()
    map.easeTo.mockClear()
    map.center = ELSEWHERE
    act(() => map.fire('webglcontextlost'))
    act(() => map.fire('webglcontextrestored'))
    act(() => map.fire('style.load'))
    expect(map.easeTo).not.toHaveBeenCalled()
  })

  /**
   * Builds a new map in the same canvas, as showing the page again does: a
   * new pov object re-runs the map's effect, and the component keeps its
   * refs. Returns the new map, its style loaded.
   */
  function rebuildMap(
    rerender: (ui: React.ReactElement) => void,
    props: Parameters<typeof GlobeCanvas>[0],
  ) {
    rerender(<GlobeCanvas {...props} pov={{ ...props.pov }} />)
    const map = stub.state.maps[stub.state.maps.length - 1]
    act(() => map.fire('style.load'))
    return map
  }

  it('eases a dot selected before its first map to the band above the sheet', () => {
    const { map } = renderPreview({ selected: CHICAGO, scenePreviewSheetDetent: 'half' })
    expect(map.easeTo).toHaveBeenCalledWith({ center: CHICAGO_CENTER, padding: padded(HALF_PX) })
  })

  it('rebuilds a map on a settled camera with the padding that camera had, unmoved', () => {
    const { map, rerender, props } = selectChicago()
    // Peek with the dot clear keeps Half's padding on the camera.
    const atPeek = { ...props, selected: CHICAGO, scenePreviewSheetDetent: 'peek' as const }
    rerender(<GlobeCanvas {...atPeek} />)
    expect(map.padding).toEqual(padded(HALF_PX))

    const rebuilt = rebuildMap(rerender, atPeek)
    expect(rebuilt).not.toBe(map)
    expect(rebuilt.setPadding).toHaveBeenCalledWith(padded(HALF_PX))
    expect(rebuilt.easeTo).not.toHaveBeenCalled()
    const halo = screen.getByTestId('globe-cursor-wrap').children[1] as HTMLElement
    expect(halo.style.top).toBe(`calc(50% - ${HALF_PX / 2}px)`)
  })

  it('rebuilds the map for a theme change with the padding the old camera had, unmoved', async () => {
    const { map, rerender, props } = selectChicago()
    const atPeek = { ...props, selected: CHICAGO, scenePreviewSheetDetent: 'peek' as const }
    rerender(<GlobeCanvas {...atPeek} />)
    expect(map.padding).toEqual(padded(HALF_PX))

    try {
      await act(async () => {
        document.documentElement.classList.add('dark')
      })
      const rebuilt = stub.state.maps[stub.state.maps.length - 1]
      act(() => rebuilt.fire('style.load'))
      expect(rebuilt).not.toBe(map)
      expect(map.remove).toHaveBeenCalled()
      expect(rebuilt.setPadding).toHaveBeenCalledWith(padded(HALF_PX))
      expect(rebuilt.easeTo).not.toHaveBeenCalled()
    } finally {
      document.documentElement.classList.remove('dark')
    }
  })

  it('moves once on a map rebuilt while the move for the current selection was still waiting', () => {
    const { map, rerender, props } = renderPreview()
    map.moving = true
    const selected = { ...props, selected: CHICAGO, scenePreviewSheetDetent: 'half' as const }
    rerender(<GlobeCanvas {...selected} />)
    // The movement ended; the move waits for the next frame, and the page is
    // hidden before it comes, with the camera at rest.
    map.moving = false
    act(() => map.fire('moveend'))
    const rebuilt = rebuildMap(rerender, selected)
    expect(map.easeTo).not.toHaveBeenCalled()
    expect(rebuilt.easeTo).toHaveBeenCalledTimes(1)
    expect(rebuilt.easeTo).toHaveBeenCalledWith({ center: CHICAGO_CENTER, padding: padded(HALF_PX) })
  })

  it('moves once on a map rebuilt from a camera that was still moving', () => {
    const { map, rerender, props } = selectChicago()
    // Hidden mid-ease.
    map.moving = true
    const rebuilt = rebuildMap(rerender, { ...props, selected: CHICAGO, scenePreviewSheetDetent: 'half' })
    expect(rebuilt.setPadding).toHaveBeenCalledWith(padded(HALF_PX))
    expect(rebuilt.easeTo).toHaveBeenCalledTimes(1)
    expect(rebuilt.easeTo).toHaveBeenCalledWith({ center: CHICAGO_CENTER, padding: padded(HALF_PX) })
  })

  it('keeps the halo on the centre of the padded viewport', () => {
    const { map } = renderPreview()
    const halo = screen.getByTestId('globe-cursor-wrap').children[1] as HTMLElement
    map.padding = padded(HALF_PX)
    act(() => map.fire('move'))
    expect(halo.style.top).toBe(`calc(50% - ${HALF_PX / 2}px)`)
    map.padding = NO_PADDING
    act(() => map.fire('move'))
    expect(halo.style.top).toBe('')
  })
})

describe('GlobeCanvas theme', () => {
  type StyleLayer = { id: string; paint?: Record<string, unknown> }
  const layer = (map: StubMap, id: string) =>
    (map.options as unknown as { style: { layers: StyleLayer[] } }).style.layers.find(
      (l) => l.id === id,
    )
  const venueLabel = (name: string) => screen.getByText(name, { selector: 'div' })
  const ground = () => screen.getByTestId('globe-cursor-wrap').dataset.atlasGround
  function zoomTo(map: StubMap, zoom: number) {
    map.getZoom = () => zoom
    act(() => map.fire('zoom'))
  }

  beforeEach(() => {
    stub.state.maps = []
    stub.state.markers = []
    sessionStorage.clear()
  })
  afterEach(() => {
    document.documentElement.classList.remove('dark')
  })

  it('draws the dark street style and the dark ramp in the dark theme', () => {
    document.documentElement.classList.add('dark')
    const { map } = renderCanvas()
    expect(layer(map, 'ph-background')?.paint?.['background-color']).toBe('#0d0805')
    expect(layer(map, 'ph-water')?.paint?.['fill-color']).toBe('#090c12')
    const dots = layer(map, 'scene-dots')?.paint
    expect((dots?.['circle-color'] as unknown[])[2]).toBe(MARKER_RAMPS.dark.hovered)
    expect(dots?.['circle-stroke-color']).toBe('rgba(255,230,194,0.35)')
    expect(dots?.['circle-stroke-width']).toBe(1)
    expect(layer(map, 'venue-pins')?.paint).toEqual(venuePinPaint(MARKER_RAMPS.dark))
  })

  it('draws the light street style and the light ramp in the light theme', () => {
    const { map } = renderCanvas()
    expect(layer(map, 'ph-background')?.paint?.['background-color']).toEqual([
      'interpolate',
      ['linear'],
      ['zoom'],
      5.5,
      '#0d0805',
      7,
      '#f4f1ea',
    ])
    expect(layer(map, 'ph-water')?.paint?.['fill-color']).toBe('#dbe4ea')
    const dots = layer(map, 'scene-dots')?.paint
    expect((dots?.['circle-color'] as unknown[])[2]).toBe(MARKER_RAMPS.light.hovered)
    expect(dots?.['circle-stroke-color']).toEqual([
      'case',
      ['get', 'isSelected'],
      '#f4f1ea',
      'rgba(255,230,194,0.35)',
    ])
    expect(dots?.['circle-stroke-width']).toEqual(['case', ['get', 'isSelected'], 1.5, 1])
    expect(layer(map, 'venue-pins')?.paint).toEqual(venuePinPaint(MARKER_RAMPS.light))
  })

  it('draws every map label in the ink and halo of the ground, cream on a dark halo by default', () => {
    const { rerender, props } = renderCanvas()
    for (const label of [venueLabel('Thalia Hall')]) {
      expect(label.style.color).toBe('var(--atlas-label-ink, #ffe6c2)')
      expect(label.style.textShadow).toBe('var(--atlas-label-halo, 0 1px 4px rgba(0,0,0,0.9))')
    }
    rerender(<GlobeCanvas {...props} cityLabel={null} venues={[]} />)
    const scene = screen.getAllByTestId('atlas-scene-label')[0]
    expect(scene.style.color).toBe('var(--atlas-label-ink, #ffe6c2)')
    expect(scene.style.textShadow).toBe('var(--atlas-label-halo, 0 1px 4px rgba(0,0,0,0.9))')
  })

  it('marks the ground paper in the light theme from the zoom where the light ground passes mid-grey', () => {
    expect(LABEL_PAPER_GROUND_ZOOM).toBe(lightGroundPaperZoom(5.5, 7))
    const { map } = renderCanvas()
    // The stub opens at zoom 13, on the streets.
    expect(ground()).toBe('paper')
    zoomTo(map, 6.5)
    expect(ground()).toBeUndefined()
    zoomTo(map, 6.6)
    expect(ground()).toBe('paper')
    zoomTo(map, 3)
    expect(ground()).toBeUndefined()
  })

  it('never marks the ground paper in the dark theme', () => {
    document.documentElement.classList.add('dark')
    const { map } = renderCanvas()
    expect(ground()).toBeUndefined()
    zoomTo(map, 8)
    expect(ground()).toBeUndefined()
  })

  it('stops following the zoom when the map goes', () => {
    const { map, unmount } = renderCanvas()
    const listening = map.handlers.get('zoom') ?? []
    unmount()
    // The stub keeps the map's own listeners past remove(), which MapLibre
    // drops; the ground's listener is the one this component takes off.
    expect(map.handlers.get('zoom')).toHaveLength(listening.length - 1)
  })

  it('sets the paper-ground ink and halo from the theme tokens in globals.css', () => {
    const css = readFileSync(path.resolve(__dirname, '../../../app/globals.css'), 'utf8')
    const at = css.indexOf("[data-atlas-ground='paper'] {")
    expect(at).toBeGreaterThan(-1)
    const rule = css.slice(at, css.indexOf('}', at))
    expect(rule).toContain('--atlas-label-ink: var(--foreground);')
    expect(rule).toMatch(/--atlas-label-halo:[^;]*var\(--background\)/)
  })

  it('remounts the map for a theme change, removing the old one and keeping the camera', async () => {
    const { map } = renderCanvas()
    map.center = { lng: -87.7, lat: 41.9 }
    // A remount runs every effect cleanup together, the map's removal last,
    // so nothing still holds the old map when it goes.
    let controlsAtRemoval: number | null = null
    map.remove.mockImplementation(() => {
      controlsAtRemoval = map.controls.length
    })
    await act(async () => {
      document.documentElement.classList.add('dark')
    })
    expect(stub.state.maps).toHaveLength(2)
    expect(map.remove).toHaveBeenCalledTimes(1)
    expect(controlsAtRemoval).toBe(0)
    const rebuilt = stub.state.maps[1]
    const options = rebuilt.options as unknown as { center: [number, number]; zoom: number }
    expect(options.center).toEqual([-87.7, 41.9])
    expect(options.zoom).toBe(13)
    expect(layer(rebuilt, 'ph-background')?.paint?.['background-color']).toBe('#0d0805')
    // One live map: the old map's labels went with it, and the dark ground
    // took over from the paper one.
    act(() => rebuilt.fire('style.load'))
    expect(screen.getAllByText('Thalia Hall', { selector: 'div' })).toHaveLength(1)
    expect(ground()).toBeUndefined()
  })

  it('keeps the map through an html class change that leaves the theme as it was', async () => {
    renderCanvas()
    try {
      await act(async () => {
        document.documentElement.classList.add('some-other-class')
      })
      expect(stub.state.maps).toHaveLength(1)
    } finally {
      document.documentElement.classList.remove('some-other-class')
    }
  })
})
