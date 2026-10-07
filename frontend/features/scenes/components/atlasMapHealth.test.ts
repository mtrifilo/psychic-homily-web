import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { GPUInitializationError, type Map as MapLibreMap } from 'maplibre-gl'
import maplibrePackage from 'maplibre-gl/package.json'
import {
  type AtlasMapHealth,
  ATLAS_CONTEXT_RESTORE_DEADLINE_MS,
  atlasMapBoundaryReporting,
  atlasMapContextRefused,
  watchAtlasMapHealth,
} from './atlasMapHealth'
import { AtlasMapUnrecoverableError } from '../atlasViewport'

/** The map's event surface, fired by hand in MapLibre's order. */
class FakeMap {
  /** MapLibre's current style; `_loaded` once its style.load has fired. */
  style: { _loaded: boolean } | null = null
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

  it('fails as context-lost-during-restore at once when a restored style is lost again before it loads', () => {
    map.fire('style.load')
    map.fire('webglcontextlost')
    map.fire('webglcontextrestored')
    map.fire('webglcontextlost')
    expect(failureClasses()).toEqual(['context-lost-during-restore'])
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

describe('watchAtlasMapHealth style-live callback', () => {
  let map: FakeMap
  let health: AtlasMapHealth | null
  let onUnrecoverable: ReturnType<typeof vi.fn<(error: AtlasMapUnrecoverableError) => void>>
  // Each call records the watcher's own answer beside the value passed, so a
  // callback that ran before the watcher classified its event would show up.
  // `styleLive` is null for a call made before watchAtlasMapHealth returned.
  let calls: { live: boolean; styleLive: boolean | null }[]

  function watch() {
    health = watchAtlasMapHealth(map as unknown as MapLibreMap, onUnrecoverable, (live) => {
      calls.push({ live, styleLive: health === null ? null : health.styleLive() })
    })
  }

  beforeEach(() => {
    vi.useFakeTimers()
    map = new FakeMap()
    health = null
    onUnrecoverable = vi.fn<(error: AtlasMapUnrecoverableError) => void>()
    calls = []
    setVisibility('visible')
  })
  afterEach(() => {
    health?.stop()
    vi.useRealTimers()
    delete (document as { visibilityState?: unknown }).visibilityState
  })

  it('follows the style through a loss and a restore when the watch began before the style loaded', () => {
    watch()
    expect(calls).toEqual([])
    map.fire('style.load')
    map.fire('webglcontextlost')
    map.fire('webglcontextrestored')
    map.fire('style.load')
    expect(calls).toEqual([
      { live: true, styleLive: true },
      { live: false, styleLive: false },
      { live: true, styleLive: true },
    ])
    expect(onUnrecoverable).not.toHaveBeenCalled()
  })

  it('reports a style that loaded before the watch began at once, and treats a loss as a live style lost', () => {
    map.style = { _loaded: true }
    watch()
    expect(calls).toEqual([{ live: true, styleLive: null }])
    expect(health?.styleLive()).toBe(true)

    map.style = null
    map.fire('webglcontextlost')
    expect(calls.at(-1)).toEqual({ live: false, styleLive: false })
    // A loss of a loaded style waits for the restore rather than failing at once.
    expect(onUnrecoverable).not.toHaveBeenCalled()
    vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS)
    expect(onUnrecoverable.mock.calls.map(([error]) => error.failureClass)).toEqual(['context-lost'])
  })

  it('does not report a style that exists but has not loaded when the watch began', () => {
    map.style = { _loaded: false }
    watch()
    expect(calls).toEqual([])
    expect(health?.styleLive()).toBe(false)
  })

  it('stays quiet for an event that leaves the answer unchanged', () => {
    watch()
    // Lost before any style loaded: a failure, with the style still down.
    map.fire('webglcontextlost')
    expect(calls).toEqual([])
    expect(onUnrecoverable).toHaveBeenCalledTimes(1)
  })

  it('stops calling once it has reported a failure', () => {
    watch()
    map.fire('style.load')
    map.fire('webglcontextlost')
    map.fire('error', { error: gpuInitializationError() })
    // MapLibre re-sets the saved style before it asks for a context, so its
    // style.load can still follow the refusal.
    map.fire('webglcontextrestored')
    map.fire('style.load')
    expect(onUnrecoverable).toHaveBeenCalledTimes(1)
    expect(calls).toEqual([
      { live: true, styleLive: true },
      { live: false, styleLive: false },
    ])
  })

  it('stops calling once disposed', () => {
    watch()
    map.fire('style.load')
    health?.stop()
    map.fire('webglcontextlost')
    map.fire('style.load')
    expect(calls).toEqual([{ live: true, styleLive: true }])
  })
})

describe('atlasMapContextRefused', () => {
  it('is true for a map MapLibre built without a painter', () => {
    expect(atlasMapContextRefused({} as unknown as MapLibreMap)).toBe(true)
  })

  it('is false for a map with a painter', () => {
    expect(atlasMapContextRefused({ painter: {} } as unknown as MapLibreMap)).toBe(false)
  })
})

describe('atlasMapBoundaryReporting', () => {
  it('files failures under the section and tags an unrecoverable map with its failure class', () => {
    const { sentryTag, errorTags } = atlasMapBoundaryReporting('venue-mini-atlas')
    expect(sentryTag).toBe('venue-mini-atlas')
    expect(errorTags(new AtlasMapUnrecoverableError('context-lost'))).toEqual({
      atlas_map_failure: 'context-lost',
    })
  })

  it('adds no tag for any other error', () => {
    const { errorTags } = atlasMapBoundaryReporting('atlas-map')
    expect(errorTags(new Error('Loading chunk 123 failed'))).toBeUndefined()
    expect(errorTags('not an error')).toBeUndefined()
  })
})

/**
 * The MapLibre behaviour watchAtlasMapHealth relies on (see its doc): the
 * context events fire after MapLibre has acted on them, a style lost before it
 * loaded is not saved for the restore, a restore re-sets the saved style, and
 * a refused context is a GPUInitializationError by name. On a version change,
 * re-read `_contextLost`, `_contextRestored`, `_setupPainter` and the
 * constructor's painter check in src/ui/map.ts, and `serialize` and `_load`
 * (where `_loaded` is set beside `style.load`) in src/style/style.ts, then
 * bump this.
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
