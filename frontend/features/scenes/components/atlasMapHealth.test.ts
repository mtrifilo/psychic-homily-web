import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { GPUInitializationError, type Map as MapLibreMap } from 'maplibre-gl'
import maplibrePackage from 'maplibre-gl/package.json'
import {
  type AtlasMapHealth,
  ATLAS_CONTEXT_RESTORE_DEADLINE_MS,
  watchAtlasMapHealth,
} from './atlasMapHealth'
import { AtlasMapUnrecoverableError } from '../atlasViewport'

/** The map's event surface, fired by hand in MapLibre's order. */
class FakeMap {
  private handlers = new Map<string, ((event: unknown) => void)[]>()
  on(type: string, handler: (event: unknown) => void) {
    this.handlers.set(type, [...(this.handlers.get(type) ?? []), handler])
    return this
  }
  fire(type: string, event: unknown = {}) {
    for (const handler of this.handlers.get(type) ?? []) handler(event)
  }
}

function gpuInitializationError(): Error {
  const error = new Error('WebGL2 is required to display this map.')
  error.name = 'GPUInitializationError'
  return error
}

function setVisibility(state: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    get: () => state,
  })
  document.dispatchEvent(new Event('visibilitychange'))
}

describe('watchAtlasMapHealth', () => {
  let map: FakeMap
  let onUnrecoverable: ReturnType<typeof vi.fn<(error: AtlasMapUnrecoverableError) => void>>
  let health: AtlasMapHealth
  const stop = () => health.stop()

  function watch() {
    health = watchAtlasMapHealth(map as unknown as MapLibreMap, onUnrecoverable)
  }
  function failureClasses() {
    return onUnrecoverable.mock.calls.map(([error]) => {
      expect(error).toBeInstanceOf(AtlasMapUnrecoverableError)
      return error.failureClass
    })
  }

  beforeEach(() => {
    vi.useFakeTimers()
    map = new FakeMap()
    onUnrecoverable = vi.fn<(error: AtlasMapUnrecoverableError) => void>()
    setVisibility('visible')
    watch()
  })
  afterEach(() => {
    stop()
    vi.useRealTimers()
    // Back to jsdom's own getter.
    delete (document as { visibilityState?: unknown }).visibilityState
  })

  describe('a lost context', () => {
    beforeEach(() => map.fire('style.load'))

    it('fails as context-lost once the deadline passes without a restore', () => {
      map.fire('webglcontextlost')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS - 1)
      expect(onUnrecoverable).not.toHaveBeenCalled()
      vi.advanceTimersByTime(1)
      expect(failureClasses()).toEqual(['context-lost'])
    })

    it('keeps the map when the context is restored before the deadline', () => {
      map.fire('webglcontextlost')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS - 1)
      map.fire('webglcontextrestored')
      map.fire('style.load')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10)
      expect(onUnrecoverable).not.toHaveBeenCalled()
    })

    it('keeps the map when the restored style loads without a restored event', () => {
      map.fire('webglcontextlost')
      map.fire('style.load')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10)
      expect(onUnrecoverable).not.toHaveBeenCalled()
    })

    it('reports the style down from the loss until the restored style loads', () => {
      expect(health.styleLive()).toBe(true)
      map.fire('webglcontextlost')
      expect(health.styleLive()).toBe(false)
      map.fire('webglcontextrestored')
      expect(health.styleLive()).toBe(false)
      map.fire('style.load')
      expect(health.styleLive()).toBe(true)
    })

    it('gives a second loss after a restore its own deadline', () => {
      map.fire('webglcontextlost')
      map.fire('webglcontextrestored')
      map.fire('style.load')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS - 1)
      map.fire('webglcontextlost')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS - 1)
      expect(onUnrecoverable).not.toHaveBeenCalled()
      vi.advanceTimersByTime(1)
      expect(failureClasses()).toEqual(['context-lost'])
    })

    it('restarts the deadline in full each time the page is shown', () => {
      setVisibility('hidden')
      map.fire('webglcontextlost')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10)
      expect(onUnrecoverable).not.toHaveBeenCalled()

      setVisibility('visible')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS - 1)
      // Hidden again before the deadline: the clock stops, then restarts in full.
      setVisibility('hidden')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10)
      setVisibility('visible')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS - 1)
      expect(onUnrecoverable).not.toHaveBeenCalled()
      vi.advanceTimersByTime(1)
      expect(failureClasses()).toEqual(['context-lost'])
    })

    it('fails as context-restore-refused when the restore gets no WebGL2 context', () => {
      map.fire('webglcontextlost')
      map.fire('error', { error: gpuInitializationError() })
      // MapLibre still fires the restored event after a refused restore.
      map.fire('webglcontextrestored')
      expect(failureClasses()).toEqual(['context-restore-refused'])
      // Reported once: the deadline that was running does not report again.
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS)
      expect(onUnrecoverable).toHaveBeenCalledTimes(1)
    })

    it('ignores an error without a source while the context is lost', () => {
      map.fire('webglcontextlost')
      map.fire('error', { error: new Error('Cannot style non-existing layer') })
      map.fire('webglcontextrestored')
      map.fire('style.load')
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10)
      expect(onUnrecoverable).not.toHaveBeenCalled()
    })

    it('stops counting once disposed', () => {
      map.fire('webglcontextlost')
      stop()
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10)
      expect(onUnrecoverable).not.toHaveBeenCalled()
    })
  })

  it('fails as context-lost-before-style at once when the style had not loaded', () => {
    map.fire('webglcontextlost')
    expect(failureClasses()).toEqual(['context-lost-before-style'])
  })

  it('fails as context-lost at once when a restored style is lost again before it loads', () => {
    map.fire('style.load')
    map.fire('webglcontextlost')
    map.fire('webglcontextrestored')
    map.fire('webglcontextlost')
    expect(failureClasses()).toEqual(['context-lost'])
  })

  describe('an error event', () => {
    it('fails as style-load-failed during the first render, before the style loads', () => {
      map.fire('error', { error: new Error('layers[3]: missing required property "source"') })
      expect(failureClasses()).toEqual(['style-load-failed'])
    })

    it('fails as style-load-failed when a restored style errors before it loads', () => {
      map.fire('style.load')
      map.fire('webglcontextlost')
      map.fire('webglcontextrestored')
      map.fire('error', { error: new Error('style is not valid') })
      expect(failureClasses()).toEqual(['style-load-failed'])
    })

    it('keeps the map for a source error during the first render', () => {
      map.fire('error', { error: new Error('AJAXError: 503'), sourceId: 'openmaptiles' })
      expect(onUnrecoverable).not.toHaveBeenCalled()
    })

    it('keeps the map for any error once the style has loaded', () => {
      map.fire('style.load')
      map.fire('error', { error: new Error('Cannot style non-existing layer') })
      map.fire('error', { error: new Error('AJAXError: 503'), sourceId: 'nightEarth' })
      map.fire('error', { error: { message: 'no name at all' } })
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS * 10)
      expect(onUnrecoverable).not.toHaveBeenCalled()
    })

    it('fails as context-restore-refused for a GPU error even on a loaded style', () => {
      map.fire('style.load')
      map.fire('error', { error: gpuInitializationError() })
      expect(failureClasses()).toEqual(['context-restore-refused'])
    })
  })

  it('reports only the first failure', () => {
    map.fire('error', { error: new Error('style is not valid') })
    map.fire('webglcontextlost')
    map.fire('error', { error: gpuInitializationError() })
    expect(failureClasses()).toEqual(['style-load-failed'])
  })

  it('reports no style before the first style.load', () => {
    expect(health.styleLive()).toBe(false)
  })

  it('listens for visibility only while a loss is being timed', () => {
    const add = vi.spyOn(document, 'addEventListener')
    const remove = vi.spyOn(document, 'removeEventListener')
    const visibilityCalls = (spy: typeof add) =>
      spy.mock.calls.filter(([type]) => type === 'visibilitychange').length
    map.fire('style.load')
    expect(visibilityCalls(add)).toBe(0)
    map.fire('webglcontextlost')
    expect(visibilityCalls(add)).toBe(1)
    map.fire('webglcontextrestored')
    expect(visibilityCalls(remove)).toBeGreaterThanOrEqual(1)
    add.mockRestore()
    remove.mockRestore()
  })

  it('stops listening for visibility once disposed during a loss', () => {
    map.fire('style.load')
    map.fire('webglcontextlost')
    const remove = vi.spyOn(document, 'removeEventListener')
    stop()
    expect(remove).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
    remove.mockRestore()
  })
})

/**
 * The MapLibre behaviour watchAtlasMapHealth relies on (see its doc): the
 * context events fire after MapLibre has acted on them, a style lost before it
 * loaded is not saved for the restore, a restore re-sets the saved style, and
 * a refused context is a GPUInitializationError by name. On a version change,
 * re-read `_contextLost`, `_contextRestored` and `_setupPainter` in
 * src/ui/map.ts and `serialize` in src/style/style.ts, then bump this.
 */
describe('maplibre context-loss contract', () => {
  it('is pinned to the verified maplibre-gl version', () => {
    expect(
      maplibrePackage.version,
      're-verify the context-loss behaviour atlasMapHealth.ts documents, then bump this',
    ).toBe('6.0.0')
  })

  it('names a refused context GPUInitializationError', () => {
    expect(new GPUInitializationError({}, null).name).toBe('GPUInitializationError')
  })
})
