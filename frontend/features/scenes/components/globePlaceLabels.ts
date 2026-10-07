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
   * The map's root element in the page. The page draws its chrome over the
   * map as this element's siblings; their controls, and the map's own
   * controls, are obstacles too (see {@link watchMapChrome}).
   */
  canvasRoot: HTMLElement
}

// How long the chrome must hold still after a change before its boxes are
// read again, so a run of changes (a sheet being dragged, a panel filling
// in) costs one read and one layout rather than one per frame.
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
 * path down that take pointer events and have an area. Chrome over the map
 * keeps every wrapper around its controls either at `pointer-events: none`,
 * so the map takes drags through the gaps, or with no area of its own (a
 * box holding only positioned children, such as MapLibre's control
 * container); the walk passes through both kinds of wrapper and stops at the
 * controls themselves. A control with no area (unmounted, `hidden`, an empty
 * credit) is passed through the same way and holds no spot.
 */
function collectControls(parent: Element, skip: Element, out: Control[]): void {
  for (const child of parent.children) {
    if (child === skip) continue
    const rect = child.getBoundingClientRect()
    const passThrough =
      rect.width <= 0 || rect.height <= 0 || getComputedStyle(child).pointerEvents === 'none'
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

export interface MapChromeWatch {
  /** The controls' boxes as last read, relative to the map container. */
  boxes(): readonly Box[]
  stop(): void
}

/**
 * Keeps the boxes of the controls drawn over a map: the page's chrome
 * (`canvasRoot`'s siblings, such as the search pill, Drift, the genre key and
 * the My Scenes chips) and the map's own controls (the credit), relative to
 * the map container.
 *
 * The boxes are read from the DOM at the start and again once the chrome has
 * held still for CHROME_SETTLE_MS after a resize of the pane or of a control,
 * or a change inside the chrome (a control shown, hidden or restyled), never
 * per frame or per camera move. `onChange` runs after a read whose boxes
 * differ from the previous read's.
 */
export function watchMapChrome(
  map: maplibregl.Map,
  canvasRoot: HTMLElement,
  onChange: () => void,
): MapChromeWatch {
  const container = map.getContainer()
  const canvasContainer = map.getCanvasContainer()
  const pane = canvasRoot.parentElement
  let boxes: Box[] = []
  let sized = new Set<Element>()
  let timer: ReturnType<typeof setTimeout> | undefined

  const changed = () => {
    clearTimeout(timer)
    timer = setTimeout(() => {
      timer = undefined
      if (read()) onChange()
    }, CHROME_SETTLE_MS)
  }
  const resizes = new ResizeObserver(changed)
  const mutations = new MutationObserver(changed)

  // Observes the chrome as it stands: the pane's own attributes and children
  // (chrome mounting and unmounting), every chrome subtree beside the canvas
  // root and beside the map's canvas (observing a node again only renews its
  // options), and each control's size. Reads the boxes and reports whether
  // they changed.
  const read = (): boolean => {
    const controls: Control[] = []
    if (pane) {
      mutations.observe(pane, { attributes: true, childList: true })
      for (const child of pane.children) {
        if (child !== canvasRoot) mutations.observe(child, CHROME_SUBTREE_CHANGES)
      }
      collectControls(pane, canvasRoot, controls)
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
  read()
  return {
    boxes: () => boxes,
    stop: () => {
      clearTimeout(timer)
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
  { maxZoom, obstacles, canvasRoot }: PlaceLabelOptions,
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

  const chrome = watchMapChrome(map, canvasRoot, layout)
  layout()
  map.on('moveend', layout)
  map.on('zoom', handleZoom)
  return () => {
    chrome.stop()
    map.off('moveend', layout)
    map.off('zoom', handleZoom)
    clear()
  }
}
