import type { ErrorEvent, Map as MapLibreMap } from 'maplibre-gl'
import {
  type AtlasMapFailureClass,
  AtlasMapUnrecoverableError,
} from '../atlasViewport'

/**
 * How long a lost WebGL context may stay lost, counted while the page is
 * visible, before the Atlas gives up on the map. MapLibre asks the browser to
 * restore every lost context, so a short loss (a GPU reset the browser
 * recovers from) comes back on its own and must not cost the visitor the map
 * for the rest of the page load. A restore still missing after this long is
 * treated as never coming: the pane would otherwise stay blank with no way
 * forward. Only visible time counts: a hidden page shows the visitor nothing,
 * so a restore that lands before the page is shown again has cost nothing.
 */
export const ATLAS_CONTEXT_RESTORE_DEADLINE_MS = 3000

/**
 * The events this module reads. They are MapLibre's own, so each one arrives
 * after MapLibre has acted on it: `webglcontextlost` after the style has been
 * saved (or not) and destroyed, `webglcontextrestored` only once a restore
 * got a working context.
 */
type HealthMap = Pick<MapLibreMap, 'on'>

/**
 * Watches a constructed map for the failures MapLibre does not recover from
 * on its own, and calls `onUnrecoverable` once, with the failure classified,
 * for the first of them. Returns a disposer that cancels the deadline and
 * stops reporting; the map's own listeners go with the map.
 *
 * MapLibre 6.0.0's behaviour this relies on:
 * - A lost context after the style loaded comes back with the style when the
 *   browser restores it. Only a restore that never arrives is a failure.
 * - A lost context before the style loaded cannot come back: MapLibre saves
 *   only a loaded style, so a restore would leave a map with no style.
 * - `GPUInitializationError` on a constructed map comes from a restore that
 *   got no context (construction reports it before any listener exists).
 * - An error that names a source is isolated to that source (the rest of the
 *   map draws), and so is any error once the style has loaded. An error
 *   without a source before the style loads means the style never loads.
 *
 * Not visible here: exceptions thrown inside MapLibre's render frame (they
 * propagate out of its animation-frame callback, not as an `error` event) and
 * a worker that never starts (MapLibre reports nothing).
 */
export function watchAtlasMapHealth(
  map: HealthMap,
  onUnrecoverable: (error: AtlasMapUnrecoverableError) => void,
): () => void {
  let styleLoaded = false
  let contextLost = false
  let settled = false
  let deadline: ReturnType<typeof setTimeout> | null = null

  function fail(failureClass: AtlasMapFailureClass) {
    if (settled) return
    settled = true
    cancelDeadline()
    onUnrecoverable(new AtlasMapUnrecoverableError(failureClass))
  }

  function cancelDeadline() {
    if (deadline === null) return
    clearTimeout(deadline)
    deadline = null
  }

  // Restarts the full deadline each time the page is shown, rather than
  // resuming what was left of it.
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
  document.addEventListener('visibilitychange', handleVisibility)

  map.on('style.load', () => {
    styleLoaded = true
  })

  map.on('webglcontextlost', () => {
    if (!styleLoaded) {
      fail('context-lost-before-style')
      return
    }
    // MapLibre destroyed the style; a restore re-creates it and fires
    // style.load again.
    styleLoaded = false
    contextLost = true
    armDeadline()
  })

  map.on('webglcontextrestored', () => {
    contextLost = false
    cancelDeadline()
  })

  map.on('error', (event: ErrorEvent & { sourceId?: unknown }) => {
    // Matched by name, which MapLibre sets in the class's constructor (the
    // published type of `event.error` carries only a message).
    const name = (event.error as { name?: unknown } | undefined)?.name
    if (name === 'GPUInitializationError') {
      fail('context-restore-refused')
      return
    }
    if (!styleLoaded && !contextLost && event.sourceId === undefined) {
      fail('style-load-failed')
    }
  })

  return () => {
    settled = true
    cancelDeadline()
    document.removeEventListener('visibilitychange', handleVisibility)
  }
}
