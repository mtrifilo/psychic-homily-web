import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type * as maplibregl from 'maplibre-gl'
import type { Box, GlobePlace } from '../basemap/globePlaces'
import { mountPlaceLabels, watchMapChrome } from './globePlaceLabels'

/**
 * The place-label controller against the controls drawn over the map, on a
 * DOM shaped like the Atlas sheet layout: a pane holding the map's root
 * (whose map container holds the canvas and the map's own controls) and,
 * beside it, the page chrome. The projection is flat: a place's lng/lat are
 * its screen x/y relative to the map container. A label measures 5.81px per
 * character and 14px tall, as Space Mono at 9.5px does in Chromium.
 */

vi.mock('maplibre-gl', () => {
  class StubMarker {
    el: HTMLElement
    constructor(options: { element: HTMLElement }) {
      this.el = options.element
    }
    setLngLat() {
      return this
    }
    addTo(map: { getCanvasContainer(): HTMLElement }) {
      map.getCanvasContainer().appendChild(this.el)
      return this
    }
    remove() {
      this.el.remove()
    }
  }
  return { Marker: StubMarker }
})

const CHAR_PX = 5.81
const LABEL_HEIGHT_PX = 14
const PANE = { width: 390, height: 723 }

type Handler = () => void

interface FakeMap {
  map: maplibregl.Map
  container: HTMLDivElement
  fire(event: string): void
}

function fakeMap(container: HTMLDivElement, canvasContainer: HTMLDivElement): FakeMap {
  const handlers = new Map<string, Handler[]>()
  const map = {
    getContainer: () => container,
    getCanvasContainer: () => canvasContainer,
    getZoom: () => 4,
    project: ([lng, lat]: [number, number]) => ({ x: lng, y: lat }),
    unproject: ([x, y]: [number, number]) => ({ lng: x, lat: y }),
    on: (event: string, handler: Handler) => {
      handlers.set(event, [...(handlers.get(event) ?? []), handler])
    },
    off: (event: string, handler: Handler) => {
      handlers.set(event, (handlers.get(event) ?? []).filter((h) => h !== handler))
    },
  }
  return {
    map: map as unknown as maplibregl.Map,
    container,
    fire: (event) => {
      for (const h of handlers.get(event) ?? []) h()
    },
  }
}

/** Gives an element a fixed screen box (the map container sits at 0,0). */
function placeAt<T extends Element>(el: T, box: Box): T {
  el.getBoundingClientRect = () =>
    ({
      ...box,
      width: box.right - box.left,
      height: box.bottom - box.top,
      x: box.left,
      y: box.top,
    }) as DOMRect
  return el
}

function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  { pointerEvents, text = '' }: { pointerEvents?: 'none' | 'auto'; text?: string } = {},
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  if (pointerEvents) node.style.pointerEvents = pointerEvents
  node.textContent = text
  return node
}

/** A place whose label is centred on `box`'s centre. */
function placeOn(name: string, rank: number, box: Box): GlobePlace {
  return { name, rank, lng: (box.left + box.right) / 2, lat: (box.top + box.bottom) / 2 }
}

// The chrome's boxes at 390x844 in the sheet layout, relative to the map
// container, as measured in Chromium at the z4 camera over (-95, 38).
const SEARCH: Box = { left: 16, top: 16, right: 182.609375, bottom: 50 }
const DRIFT: Box = { left: 16, top: 669, right: 79.375, bottom: 707 }
const GENRES: Box = { left: 99.375, top: 669, right: 189.546875, bottom: 707 }
const MY_SCENES_CHIP: Box = { left: 36, top: 108, right: 112, bottom: 132 }
const CREDIT: Box = { left: 16, top: 58, right: 230, bottom: 76 }
const CLEAR: Box = { left: 250, top: 400, right: 300, bottom: 414 }

interface Atlas {
  pane: HTMLDivElement
  canvasRoot: HTMLDivElement
  search: HTMLButtonElement
  drift: HTMLButtonElement
  genres: HTMLDivElement
  myScenes: HTMLElement
  myScenesChip: HTMLButtonElement
  credit: HTMLDivElement
  fake: FakeMap
}

/**
 * The sheet layout's DOM: the search pill and a pass-through column (My
 * Scenes at its top, Drift and the genre key at its bottom) beside the map's
 * root; the credit in a pass-through control corner of MapLibre's control
 * container, beside the map's canvas.
 */
function buildAtlas(): Atlas {
  const pane = el('div')
  const canvasRoot = el('div')
  placeAt(canvasRoot, { left: 0, top: 0, right: PANE.width, bottom: PANE.height })
  const container = el('div')
  Object.defineProperty(container, 'clientWidth', { value: PANE.width })
  Object.defineProperty(container, 'clientHeight', { value: PANE.height })
  placeAt(container, { left: 0, top: 0, right: PANE.width, bottom: PANE.height })
  const canvasContainer = el('div')
  placeAt(canvasContainer, { left: 0, top: 0, right: PANE.width, bottom: PANE.height })
  // MapLibre's control container takes pointer events and has no area of
  // its own; its corners pass pointer events through.
  const controls = el('div')
  const corner = el('div', { pointerEvents: 'none' })
  const credit = placeAt(el('div', { pointerEvents: 'auto', text: 'OpenFreeMap © OpenStreetMap' }), CREDIT)
  corner.append(credit)
  controls.append(corner)
  container.append(canvasContainer, controls)
  canvasRoot.append(container)

  const search = placeAt(el('button', { text: 'Search scenes' }), SEARCH)
  const column = el('div', { pointerEvents: 'none' })
  placeAt(column, { left: 16, top: 100, right: 374, bottom: 707 })
  const myScenes = el('nav', { pointerEvents: 'none' })
  placeAt(myScenes, { left: 16, top: 104, right: 210, bottom: 136 })
  const myScenesChip = placeAt(el('button', { pointerEvents: 'auto', text: 'Phoenix' }), MY_SCENES_CHIP)
  myScenes.append(myScenesChip)
  const bottomRow = el('div', { pointerEvents: 'none' })
  const drift = placeAt(el('button', { pointerEvents: 'auto', text: 'Drift' }), DRIFT)
  const genres = placeAt(el('div', { pointerEvents: 'auto', text: 'Genres' }), GENRES)
  bottomRow.append(drift, genres)
  column.append(myScenes, bottomRow)
  pane.append(canvasRoot, search, column)
  document.body.append(pane)

  return {
    pane,
    canvasRoot,
    search,
    drift,
    genres,
    myScenes,
    myScenesChip,
    credit,
    fake: fakeMap(container, canvasContainer),
  }
}

function labelTexts() {
  return [...document.querySelectorAll('[data-testid="atlas-place-label"]')].map(
    (node) => node.textContent,
  )
}

// Resize observers created by the code under test, so a case can report a
// resize the way the browser would.
let resizeObservers: { callback: () => void; targets: Set<Element> }[] = []

class RecordingResizeObserver {
  targets = new Set<Element>()
  constructor(public callback: () => void) {
    resizeObservers.push(this)
  }
  observe(target: Element) {
    this.targets.add(target)
  }
  unobserve(target: Element) {
    this.targets.delete(target)
  }
  disconnect() {
    this.targets.clear()
  }
}

function reportResize(target: Element) {
  for (const ro of resizeObservers) if (ro.targets.has(target)) ro.callback()
}

/** Lets mutation records deliver, then runs out the chrome's settle time. */
async function settleChrome() {
  await Promise.resolve()
  vi.advanceTimersByTime(100)
}

describe('place labels and the controls over the map', () => {
  // The suite setup defines window.ResizeObserver writable but not
  // configurable, so it is swapped by assignment rather than stubGlobal.
  const setupResizeObserver = window.ResizeObserver
  beforeEach(() => {
    vi.useFakeTimers()
    resizeObservers = []
    window.ResizeObserver = RecordingResizeObserver as unknown as typeof ResizeObserver
    // A label's measured size; every other element without a box of its own
    // has no area.
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (
      this: Element,
    ) {
      const isLabel = this.getAttribute('data-testid') === 'atlas-place-label'
      const width = isLabel ? (this.textContent ?? '').length * CHAR_PX : 0
      const height = isLabel ? LABEL_HEIGHT_PX : 0
      return { left: 0, top: 0, right: width, bottom: height, width, height } as DOMRect
    })
  })
  afterEach(() => {
    vi.useRealTimers()
    window.ResizeObserver = setupResizeObserver
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  function mount(atlas: Atlas, places: GlobePlace[], obstacles: () => Box[] = () => []) {
    return mountPlaceLabels(atlas.fake.map, places, {
      maxZoom: 5.5,
      obstacles,
      canvasRoot: atlas.canvasRoot,
    })
  }

  it('drops Monterrey under Drift at z4 and keeps Houston (the 390x844 z4 view)', () => {
    // Projected centres measured at that camera; Monterrey's label spans
    // x 35.6 to 87.9, under Drift's right half.
    const places: GlobePlace[] = [
      { name: 'Houston', rank: 47, lng: 187, lat: 593 },
      { name: 'Monterrey', rank: 66, lng: 61.79, lat: 697.45 },
    ]
    const atlas = buildAtlas()
    mount(atlas, places)
    expect(labelTexts()).toEqual(['Houston'])
  })

  it('keeps Monterrey in that view once Drift is gone, so Drift alone dropped it', () => {
    const places: GlobePlace[] = [{ name: 'Monterrey', rank: 66, lng: 61.79, lat: 697.45 }]
    const atlas = buildAtlas()
    atlas.drift.remove()
    mount(atlas, places)
    expect(labelTexts()).toEqual(['Monterrey'])
  })

  it.each([
    ['the search pill', SEARCH],
    ['Drift', DRIFT],
    ['the Genres chip', GENRES],
    ['a My Scenes chip', MY_SCENES_CHIP],
    ['the credit', CREDIT],
  ])('drops a label under %s and keeps one clear of the chrome', (_name, box) => {
    const atlas = buildAtlas()
    mount(atlas, [placeOn('Under', 0, box), placeOn('Clear', 1, CLEAR)])
    expect(labelTexts()).toEqual(['Clear'])
  })

  it('lets a label sit inside a pass-through wrapper, clear of its controls', () => {
    // Inside the column's box and the My Scenes shell's box, on no control.
    const gap: Box = { left: 130, top: 112, right: 190, bottom: 126 }
    const atlas = buildAtlas()
    mount(atlas, [placeOn('In the gap', 0, gap)])
    expect(labelTexts()).toEqual(['In the gap'])
  })

  it('gives a control with no area no spot (an empty credit)', () => {
    const atlas = buildAtlas()
    // Collapsed to no width across the middle of the label's path.
    const mid = (CREDIT.left + CREDIT.right) / 2
    placeAt(atlas.credit, { ...CREDIT, left: mid, right: mid })
    mount(atlas, [placeOn('Where the credit would be', 0, CREDIT)])
    expect(labelTexts()).toEqual(['Where the credit would be'])
  })

  it("never counts the map's canvas or the map root's own layers as chrome", () => {
    const atlas = buildAtlas()
    // A full-pane backdrop that takes pointer events, inside the map's root.
    const backdrop = placeAt(el('div'), { left: 0, top: 0, right: PANE.width, bottom: PANE.height })
    atlas.canvasRoot.prepend(backdrop)
    mount(atlas, [placeOn('Clear', 0, CLEAR)])
    expect(labelTexts()).toEqual(['Clear'])
  })

  it('keeps scene marks as obstacles alongside the chrome', () => {
    const sceneLabel: Box = { left: 240, top: 300, right: 300, bottom: 316 }
    const atlas = buildAtlas()
    mount(
      atlas,
      [placeOn('On the scene', 0, sceneLabel), placeOn('Under Drift', 1, DRIFT), placeOn('Clear', 2, CLEAR)],
      () => [sceneLabel],
    )
    expect(labelTexts()).toEqual(['Clear'])
  })

  it('lays the labels out again once chrome shown after the layout has settled', async () => {
    const atlas = buildAtlas()
    atlas.myScenesChip.remove()
    mount(atlas, [placeOn('Under the strip', 0, MY_SCENES_CHIP)])
    expect(labelTexts()).toEqual(['Under the strip'])

    atlas.myScenes.append(atlas.myScenesChip)
    await Promise.resolve()
    vi.advanceTimersByTime(99)
    expect(labelTexts()).toEqual(['Under the strip'])
    vi.advanceTimersByTime(1)
    expect(labelTexts()).toEqual([])

    atlas.myScenesChip.remove()
    await settleChrome()
    expect(labelTexts()).toEqual(['Under the strip'])
  })

  it('reads the boxes again after a resize, so a label follows a control that moved', async () => {
    const atlas = buildAtlas()
    const lower: Box = { left: 16, top: 600, right: 79.375, bottom: 638 }
    mount(atlas, [placeOn('Above Drift', 0, lower), placeOn('Under Drift', 1, DRIFT)])
    expect(labelTexts()).toEqual(['Above Drift'])

    // The pane grew shorter: the bottom row rides up.
    placeAt(atlas.drift, lower)
    reportResize(atlas.pane)
    await settleChrome()
    expect(labelTexts()).toEqual(['Under Drift'])
  })

  it('reads the chrome once per change, not on every camera settle', async () => {
    const atlas = buildAtlas()
    const read = vi.spyOn(atlas.drift, 'getBoundingClientRect')
    mount(atlas, [placeOn('Clear', 0, CLEAR)])
    expect(read).toHaveBeenCalledTimes(1)

    atlas.fake.fire('moveend')
    atlas.fake.fire('moveend')
    atlas.fake.fire('moveend')
    expect(read).toHaveBeenCalledTimes(1)

    // A burst of changes inside the settle time costs one read.
    atlas.genres.setAttribute('data-open', 'true')
    reportResize(atlas.drift)
    await Promise.resolve()
    vi.advanceTimersByTime(50)
    atlas.genres.setAttribute('data-open', 'false')
    await settleChrome()
    expect(read).toHaveBeenCalledTimes(2)
  })

  it('leaves the labels in place when a change moves no control', async () => {
    const atlas = buildAtlas()
    mount(atlas, [placeOn('Clear', 0, CLEAR)])
    const before = document.querySelector('[data-testid="atlas-place-label"]')
    atlas.search.setAttribute('aria-expanded', 'true')
    await settleChrome()
    expect(document.querySelector('[data-testid="atlas-place-label"]')).toBe(before)
  })

  it('stops watching the chrome at teardown', async () => {
    const atlas = buildAtlas()
    const teardown = mount(atlas, [placeOn('Under the strip', 0, MY_SCENES_CHIP)])
    expect(labelTexts()).toEqual([])
    teardown()
    // Would bring the label back on a live watch.
    atlas.myScenesChip.remove()
    reportResize(atlas.pane)
    await Promise.resolve()
    expect(vi.getTimerCount()).toBe(0)
    vi.advanceTimersByTime(100)
    expect(labelTexts()).toEqual([])
  })

  it('reports boxes relative to the map container', () => {
    const atlas = buildAtlas()
    placeAt(atlas.fake.container, { left: 0, top: 56, right: PANE.width, bottom: 56 + PANE.height })
    placeAt(atlas.drift, { left: 16, top: 725, right: 79.375, bottom: 763 })
    const watch = watchMapChrome(atlas.fake.map, atlas.canvasRoot, () => {})
    expect(watch.boxes()).toContainEqual(DRIFT)
    watch.stop()
  })
})
