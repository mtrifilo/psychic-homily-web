import type { ErrorEvent, Map as MapLibreMap } from 'maplibre-gl'
import {
  type AtlasMapFailureClass,
  AtlasMapUnrecoverableError,
} from '../atlasViewport'

/**
 * How long a lost WebGL context may stay lost, within one continuous stretch
 * of the page being visible, before the Atlas gives up on the map. Hiding the
 * page stops the clock, and showing it again starts the full deadline over.
 * MapLibre asks the browser to restore every lost context, so a short loss (a
 * GPU reset the browser recovers from) comes back on its own and must not cost
 * the visitor the map for the rest of the page load. A restore still missing
 * after this long is treated as never coming: the pane would otherwise stay
 * blank with no way forward. A hidden page shows the visitor nothing, so a
 * restore that lands before the page is shown again has cost nothing.
 */
export const ATLAS_CONTEXT_RESTORE_DEADLINE_MS = 3000

/**
 * The events this module reads. They are MapLibre's own, so each one arrives
 * after MapLibre has acted on it: `webglcontextlost` after the style has been
 * saved (or not) and destroyed, `webglcontextrestored` after the restore asked
 * for a context (it also follows a refused one, right after the error event).
 */
type HealthMap = Pick<MapLibreMap, 'on'>

export interface AtlasMapHealth {
  /**
   * Whether the map's style can be written: true from a `style.load` until the
   * next lost context, false before the first `style.load` and from a loss
   * until the restored style's `style.load`. While false, feature-state and
   * layout writes throw. Narrower than MapLibre's `isStyleLoaded`, which also
   * waits on sources and images.
   */
  styleLive(): boolean
  /**
   * Cancels the deadline and stops reporting. The listeners stay on the map,
   * inert, so call it only as the map is removed.
   */
  stop(): void
}

/**
 * Watches a constructed map for the failures MapLibre does not recover from
 * on its own, and calls `onUnrecoverable` once, with the failure classified,
 * for the first of them.
 *
 * MapLibre 6.0.0's behaviour this relies on:
 * - A lost context after the style loaded comes back with the style when the
 *   browser restores it: MapLibre sets the saved style again, which fires
 *   `style.load` a frame later. Only a restore that never arrives is a
 *   failure.
 * - A lost context while no style is loaded cannot come back: MapLibre saves
 *   only a loaded style, so a restore would leave a map with no style. That
 *   covers a loss before the first style loaded and a second loss while a
 *   restored style is still loading.
 * - `GPUInitializationError` on a constructed map can only come from a
 *   restore that got no context (construction reports it before any listener
 *   exists).
 * - An error that names a source is isolated to that source; the rest of the
 *   map draws. An error without a source while no style is loaded (before the
 *   first `style.load`, or between a restore and its `style.load`) means that
 *   style never loads.
 *
 * This module's own rule: once the style has loaded, an `error` event is
 * never a reason to give up the map.
 *
 * Not visible here: exceptions thrown inside MapLibre's render frame (they
 * propagate out of its animation-frame callback, not as an `error` event) and
 * a worker that never starts (MapLibre reports nothing).
 */
export function watchAtlasMapHealth(
  map: HealthMap,
  onUnrecoverable: (error: AtlasMapUnrecoverableError) => void,
): AtlasMapHealth {
  let styleLive = false
  let styleEverLoaded = false
  let contextLost = false
  let settled = false
  let deadline: ReturnType<typeof setTimeout> | null = null

  function fail(failureClass: AtlasMapFailureClass) {
    if (settled) return
    settled = true
    endLoss()
    onUnrecoverable(new AtlasMapUnrecoverableError(failureClass))
  }

  function cancelDeadline() {
    if (deadline === null) return
    clearTimeout(deadline)
    deadline = null
  }

  function armDeadline() {
    if (settled || !contextLost || deadline !== null) return
    if (document.visibilityState === 'hidden') return
    deadline = setTimeout(() => {
      deadline = null
      fail('context-lost')
    }, ATLAS_CONTEXT_RESTORE_DEADLINE_MS)
  }

  function handleVisibility() {
    if (document.visibilityState === 'hidden') cancelDeadline()
    else armDeadline()
  }

  // The page listener lives only while a loss is being timed.
  function startLoss() {
    contextLost = true
    document.addEventListener('visibilitychange', handleVisibility)
    armDeadline()
  }

  function endLoss() {
    contextLost = false
    cancelDeadline()
    document.removeEventListener('visibilitychange', handleVisibility)
  }

  // A style.load during a loss is the restored style, and so the recovery,
  // even when webglcontextrestored never arrives (a listener that throws
  // inside MapLibre's restore stops it before that event fires).
  map.on('style.load', () => {
    styleLive = true
    styleEverLoaded = true
    endLoss()
  })

  map.on('webglcontextlost', () => {
    if (!styleLive) {
      fail(styleEverLoaded ? 'context-lost' : 'context-lost-before-style')
      return
    }
    // MapLibre destroyed the style; a restore re-creates it and fires
    // style.load again.
    styleLive = false
    startLoss()
  })

  map.on('webglcontextrestored', () => {
    endLoss()
  })

  map.on('error', (event: ErrorEvent & { sourceId?: unknown }) => {
    // Matched by name, which MapLibre sets in the class's constructor (the
    // published type of `event.error` carries only a message).
    const name = (event.error as { name?: unknown } | undefined)?.name
    if (name === 'GPUInitializationError') {
      fail('context-restore-refused')
      return
    }
    if (!styleLive && !contextLost && event.sourceId === undefined) {
      fail('style-load-failed')
    }
  })

  return {
    styleLive: () => styleLive,
    stop() {
      settled = true
      endLoss()
    },
  }
}
