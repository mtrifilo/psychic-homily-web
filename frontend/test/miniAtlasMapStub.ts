import { vi } from 'vitest'

/**
 * maplibre-gl stubbed down to the seams the /venues mini Atlas drives: the
 * listeners it binds (per layer or map-wide, several per event), the GeoJSON
 * source it feeds, the feature-state it sets for a hover, the camera fit, and
 * the painter whose absence means a refused WebGL2 context. WebGL is out of
 * scope: the E2E spec is what proves the canvas paints.
 *
 * Use from a test file as
 * `vi.mock('maplibre-gl', async () => (await import('@/test/miniAtlasMapStub')).maplibreStubModule())`.
 */
export interface StubMap {
  on: ReturnType<typeof vi.fn>
  /** Calls every listener bound to `key` (`event` or `event:layer`). */
  fire: (key: string, event?: unknown) => void
  setData: ReturnType<typeof vi.fn>
  setFeatureState: ReturnType<typeof vi.fn>
  removeFeatureState: ReturnType<typeof vi.fn>
  fitBounds: ReturnType<typeof vi.fn>
  remove: ReturnType<typeof vi.fn>
  canvas: HTMLCanvasElement
  options: Record<string, unknown>
  controls: { control: unknown; position: string }[]
  painter: unknown
}

/** Every map constructed since the last {@link resetStubMaps}, in order. */
export const stubMaps: StubMap[] = []

/**
 * While false, every map constructed has no painter (a refused context), until
 * {@link resetStubMaps}. Such a map's `remove()` throws, as maplibre-gl's does.
 */
export const stubMapOptions = { grantContext: true }

export function resetStubMaps(): void {
  stubMaps.length = 0
  stubMapOptions.grantContext = true
}

export function maplibreStubModule() {
  class StubAttributionControl {
    constructor(public options: unknown) {}
  }
  class StubNavigationControl {
    constructor(public options: unknown) {}
  }
  class StubMapImpl {
    listeners = new Map<string, ((event: unknown) => void)[]>()
    setData = vi.fn()
    setFeatureState = vi.fn()
    removeFeatureState = vi.fn()
    fitBounds = vi.fn()
    remove = vi.fn(() => {
      if (!this.painter) {
        throw new TypeError("Cannot read properties of undefined (reading 'destroy')")
      }
    })
    canvas = document.createElement('canvas')
    controls: { control: unknown; position: string }[] = []
    touchZoomRotate = { disableRotation: vi.fn() }
    keyboard = { disable: vi.fn() }
    painter: unknown = stubMapOptions.grantContext ? {} : undefined

    on = vi.fn(
      (
        event: string,
        layerOrHandler: string | ((e: unknown) => void),
        maybeHandler?: (e: unknown) => void,
      ) => {
        const key =
          typeof layerOrHandler === 'string'
            ? `${event}:${layerOrHandler}`
            : event
        const handler =
          typeof layerOrHandler === 'string' ? maybeHandler : layerOrHandler
        if (!handler) return
        this.listeners.set(key, [...(this.listeners.get(key) ?? []), handler])
      },
    )

    fire = (key: string, event?: unknown) => {
      for (const handler of this.listeners.get(key) ?? []) handler(event)
    }

    constructor(public options: Record<string, unknown>) {
      stubMaps.push(this as unknown as StubMap)
    }

    getSource() {
      return { setData: this.setData }
    }
    getCanvas() {
      return this.canvas
    }
    addControl(control: unknown, position: string) {
      this.controls.push({ control, position })
    }
  }
  return {
    Map: StubMapImpl,
    AttributionControl: StubAttributionControl,
    NavigationControl: StubNavigationControl,
    setWorkerUrl: vi.fn(),
  }
}
