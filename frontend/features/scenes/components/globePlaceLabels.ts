import * as maplibregl from 'maplibre-gl'
import { GLOBE_PLACE_LABEL_COLOR } from '../basemap/globeSurface'
import {
  type Box,
  type GlobePlace,
  boxRelativeTo,
  facingPoint,
  pickPlaceLabels,
  placeLabelBudget,
  placeLabelsShowAt,
} from '../basemap/globePlaces'

// The board's place label: 9.5px in the app's mono face.
const PLACE_LABEL_FONT_PX = 9.5
const PLACE_LABEL_STYLE = [
  'pointer-events: none',
  'user-select: none',
  'white-space: nowrap',
  `color: ${GLOBE_PLACE_LABEL_COLOR}`,
  `font-size: ${PLACE_LABEL_FONT_PX}px`,
  'line-height: normal',
].join(';')

function placeLabelElement(name: string): HTMLDivElement {
  const el = document.createElement('div')
  el.textContent = name
  el.className = 'font-mono'
  el.dataset.testid = 'atlas-place-label'
  el.setAttribute('aria-hidden', 'true')
  el.style.cssText = PLACE_LABEL_STYLE
  return el
}

// Each place's rendered label size, measured the first time it is needed.
// The face and size are fixed, so it holds for the page load; keyed by the
// place object, so a fresh place list measures afresh.
const labelSizes = new WeakMap<GlobePlace, { width: number; height: number }>()

export interface PlaceLabelOptions {
  /** Labels show below this zoom (and from PLACE_LABEL_MIN_ZOOM up). */
  maxZoom: number
  /**
   * The screen boxes place labels must stay clear of (scene labels and
   * dots), relative to the map container, read at each layout.
   */
  obstacles: () => Box[]
  /**
   * The controls drawn over the map ({@link watchMapChrome}), obstacles too:
   * their boxes as last read are taken at each layout, and a read that
   * changes them lays the labels out again. The watch outlives a labels
   * pass, so mounting the labels again reads no chrome.
   */
  chrome: MapChrome
}

/**
 * How long the chrome must hold still after a change before its boxes are
 * read again. 100 ms is a product number, the wait before the boxes are
 * reread (and so the longest a label sits under chrome that has just moved
 * over it), not a tuning constant. A run of changes inside it (a sheet being
 * dragged, a panel filling in) costs one read and one layout rather than one
 * per frame.
 */
const CHROME_SETTLE_MS = 100

const CHROME_SUBTREE_CHANGES: MutationObserverInit = {
  attributes: true,
  characterData: true,
  childList: true,
  subtree: true,
}

interface Control {
  el: Element
  rect: DOMRect
}

/**
 * Collects, under `parent` and outside `skip`, the first elements on each
 * path down that are drawn over the map: those that take pointer events, and
 * graphics (an icon such as the My Scenes star), each with an area. Chrome
 * over the map keeps every wrapper around its controls either at
 * `pointer-events: none`, so the map takes drags through the gaps, or with no
 * area of its own (a box holding only positioned children, such as MapLibre's
 * control container); the walk passes through both kinds of wrapper. An
 * element with no area (unmounted, `hidden`, an empty credit) is passed
 * through the same way and holds no spot. A wrapper that takes pointer events
 * and has an area counts as one control, its whole box.
 */
function collectControls(parent: Element, skip: Element, out: Control[]): void {
  for (const child of parent.children) {
    if (child === skip) continue
    const rect = child.getBoundingClientRect()
    const isGraphic = child instanceof SVGSVGElement || child instanceof HTMLImageElement
    const passThrough =
      rect.width <= 0 ||
      rect.height <= 0 ||
      (!isGraphic && getComputedStyle(child).pointerEvents === 'none')
    if (passThrough) collectControls(child, skip, out)
    else out.push({ el: child, rect })
  }
}

function sameBoxes(a: readonly Box[], b: readonly Box[]): boolean {
  return (
    a.length === b.length &&
    a.every(
      (box, i) =>
        box.left === b[i].left &&
        box.top === b[i].top &&
        box.right === b[i].right &&
        box.bottom === b[i].bottom,
    )
  )
}

/** What a labels pass reads from a {@link MapChromeWatch}. */
export interface MapChrome {
  /** The controls' boxes as last read, relative to the map container. */
  boxes(): readonly Box[]
  /**
   * Runs `listener` after each read whose boxes differ from the previous
   * read's, until the returned function is called.
   */
  subscribe(listener: () => void): () => void
}

export interface MapChromeWatch extends MapChrome {
  stop(): void
}

/**
 * Keeps the boxes of the controls drawn over a map: the page's chrome
 * (`globeRoot`'s siblings, such as the search pill, Drift, the genre key and
 * the My Scenes strip) and the map's own controls (the credit), relative to
 * the map container. `globeRoot` is GlobeCanvas's outermost element, not
 * `map.getContainer()`: its parent is the map pane, and every other child of
 * that pane is chrome drawn over the map.
 *
 * The boxes are read from the DOM at the start and again once the chrome has
 * held still for CHROME_SETTLE_MS after a resize of the pane or of a control,
 * or a change inside the chrome (a control shown, hidden or restyled), never
 * per frame or per camera move. Subscribers run after a read whose boxes
 * differ from the previous read's. While the pane itself takes no pointer
 * events (a modal elsewhere on the page sets `pointer-events: none` on the
 * body), a read would miss every control that inherits its pointer events, so
 * a settled change keeps the boxes last read; the body's style is watched
 * too, so the read runs once the modal gives pointer events back.
 */
export function watchMapChrome(map: maplibregl.Map, globeRoot: HTMLElement): MapChromeWatch {
  const container = map.getContainer()
  const canvasContainer = map.getCanvasContainer()
  const pane = globeRoot.parentElement
  const listeners = new Set<() => void>()
  let boxes: Box[] = []
  let sized = new Set<Element>()
  let timer: ReturnType<typeof setTimeout> | undefined

  const changed = () => {
    clearTimeout(timer)
    timer = setTimeout(() => {
      timer = undefined
      if (pane && getComputedStyle(pane).pointerEvents === 'none') return
      if (read()) for (const listener of listeners) listener()
    }, CHROME_SETTLE_MS)
  }
  const resizes = new ResizeObserver(changed)
  const mutations = new MutationObserver(changed)

  // Observes the chrome as it stands: the pane's own attributes and children
  // (chrome mounting and unmounting), every chrome subtree beside the globe
  // root and beside the map's canvas (observing a node again only renews its
  // options), and each control's size. Reads the boxes and reports whether
  // they changed.
  const read = (): boolean => {
    const controls: Control[] = []
    if (pane) {
      mutations.observe(pane, { attributes: true, childList: true })
      for (const child of pane.children) {
        if (child !== globeRoot) mutations.observe(child, CHROME_SUBTREE_CHANGES)
      }
      collectControls(pane, globeRoot, controls)
    }
    for (const child of container.children) {
      if (child !== canvasContainer) mutations.observe(child, CHROME_SUBTREE_CHANGES)
    }
    collectControls(container, canvasContainer, controls)

    const current = new Set(controls.map(({ el }) => el))
    for (const el of sized) if (!current.has(el)) resizes.unobserve(el)
    for (const el of current) if (!sized.has(el)) resizes.observe(el)
    sized = current

    const origin = container.getBoundingClientRect()
    const next = controls.map(({ rect }) => boxRelativeTo(rect, origin))
    const differs = !sameBoxes(boxes, next)
    boxes = next
    return differs
  }

  if (pane) resizes.observe(pane)
  mutations.observe(container.ownerDocument.body, {
    attributes: true,
    attributeFilter: ['style'],
  })
  read()
  return {
    boxes: () => boxes,
    subscribe: (listener) => {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
    stop: () => {
      clearTimeout(timer)
      listeners.clear()
      resizes.disconnect()
      mutations.disconnect()
    },
  }
}

/**
 * Draws the light globe's place labels on a live map as DOM markers and keeps
 * them current: laid out on each camera settle, cleared as soon as the zoom
 * leaves the label range. A label's box is its place's screen point plus the
 * label's rendered size (measured once per place), so a layout builds
 * markers only for the labels it keeps (globePlaces.ts picks them). Scene
 * marks and the controls over the map are the obstacles; a change in the
 * controls lays the labels out again. The collision rules hold with the
 * camera at rest: during a gesture the kept labels ride their places until
 * the settle lays them out again. Returns the teardown.
 */
export function mountPlaceLabels(
  map: maplibregl.Map,
  places: readonly GlobePlace[],
  { maxZoom, obstacles, chrome }: PlaceLabelOptions,
): () => void {
  const container = map.getContainer()
  let markers: maplibregl.Marker[] = []

  const clear = () => {
    for (const m of markers) m.remove()
    markers = []
  }

  // Measures the not-yet-measured places in one layout pass: every element is
  // attached before any is read.
  const measure = (unmeasured: readonly GlobePlace[]) => {
    if (unmeasured.length === 0) return
    const probes = unmeasured.map((place) => {
      const el = placeLabelElement(place.name)
      el.style.position = 'absolute'
      el.style.visibility = 'hidden'
      container.appendChild(el)
      return { place, el }
    })
    for (const { place, el } of probes) {
      const r = el.getBoundingClientRect()
      labelSizes.set(place, { width: r.width, height: r.height })
    }
    for (const { el } of probes) el.remove()
  }

  const layout = () => {
    const zoom = map.getZoom()
    if (!placeLabelsShowAt(zoom, maxZoom)) {
      clear()
      return
    }
    const width = container.clientWidth
    const height = container.clientHeight
    const budget = placeLabelBudget(zoom)
    const blockers = [...obstacles(), ...chrome.boxes()]

    const facing: { place: GlobePlace; x: number; y: number }[] = []
    for (const place of places) {
      const point = facingPoint(map, place.lng, place.lat, width, height)
      if (point) facing.push({ place, ...point })
    }
    measure(facing.filter(({ place }) => !labelSizes.has(place)).map(({ place }) => place))

    const candidates = facing.map(({ place, x, y }) => {
      const size = labelSizes.get(place) ?? { width: 0, height: 0 }
      return {
        place,
        box: {
          left: x - size.width / 2,
          top: y - size.height / 2,
          right: x + size.width / 2,
          bottom: y + size.height / 2,
        },
      }
    })
    const kept = pickPlaceLabels(
      candidates,
      blockers,
      { left: 0, top: 0, right: width, bottom: height },
      budget,
    )

    clear()
    markers = kept.map(({ place }) =>
      new maplibregl.Marker({
        element: placeLabelElement(place.name),
        anchor: 'center',
        opacityWhenCovered: '0',
      })
        .setLngLat([place.lng, place.lat])
        .addTo(map),
    )
  }

  // Leaving the label range mid-gesture clears at once; entering it waits for
  // the settle, where layout runs anyway.
  const handleZoom = () => {
    if (markers.length > 0 && !placeLabelsShowAt(map.getZoom(), maxZoom)) clear()
  }

  const unsubscribe = chrome.subscribe(layout)
  layout()
  map.on('moveend', layout)
  map.on('zoom', handleZoom)
  return () => {
    unsubscribe()
    map.off('moveend', layout)
    map.off('zoom', handleZoom)
    clear()
  }
}
