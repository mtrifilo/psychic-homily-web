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
 * restore that lands before the page is shown again has cost nothing. The
 * value is a choice, not a measurement of browser restore times; the
 * `atlas_map_failure` tag is how to tell whether it fits.
 */
export const ATLAS_CONTEXT_RESTORE_DEADLINE_MS = 3000

/**
 * Whether MapLibre built this map without a WebGL2 context. maplibre-gl 6.0.0
 * reports a refused context as an `error` event during construction and
 * returns a map with no painter and no input handlers (`painter` is typed
 * non-optional, but it is unset on that path). Such a map never loads, and its
 * `remove()` needs the painter, so the half-built map is left alone rather
 * than removed.
 */
export function atlasMapContextRefused(map: MapLibreMap): boolean {
  return !(map as { painter?: unknown }).painter
}

/** A Sentry tag value per failure class; a fixed vocabulary, never error text. */
function atlasMapFailureTags(error: unknown): Record<string, string> | undefined {
  return error instanceof AtlasMapUnrecoverableError
    ? { atlas_map_failure: error.failureClass }
    : undefined
}

/**
 * The Sentry reporting for the error boundary around a map: failures are filed
 * under `section`, and an {@link AtlasMapUnrecoverableError} also carries an
 * `atlas_map_failure` tag naming its failure class. Spread onto the boundary.
 */
export function atlasMapBoundaryReporting(section: string): {
  sentryTag: string
  errorTags: (error: unknown) => Record<string, string> | undefined
} {
  return { sentryTag: section, errorTags: atlasMapFailureTags }
}

/**
 * The map surface this module reads. The events are MapLibre's own, so each
 * one arrives after MapLibre has acted on it: `webglcontextlost` after the
 * style has been saved (or not) and destroyed, `webglcontextrestored` after
 * the restore asked for a context. That event also follows a refused restore,
 * right after the error event: the refusal leaves the destroyed painter in
 * place, so MapLibre's no-painter guard before the event passes. `style` is
 * read once, for a style that loaded before the watch began.
 */
type HealthMap = Pick<MapLibreMap, 'on' | 'style'>

/**
 * Whether the map's current style has loaded. MapLibre 6.0.0 sets the style's
 * `_loaded` in the same call that fires `style.load`, and a lost context
 * destroys the style and nulls `map.style`.
 */
function styleHasLoaded(map: HealthMap): boolean {
  const style = map.style as { _loaded?: unknown } | null | undefined
  return style?._loaded === true
}

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
   * Cancels the deadline and stops reporting, to both callbacks. The listeners
   * stay on the map, inert, so call it only as the map is removed.
   */
  stop(): void
}

/**
 * Watches a constructed map for the failures MapLibre does not recover from
 * on its own, and calls `onUnrecoverable` once, with the failure classified,
 * for the first of them.
 *
 * `onStyleLiveChange`, when given, is called with each change of
 * {@link AtlasMapHealth.styleLive}, after this watch has classified the event
 * behind it, and at once with `true` when the style had already loaded before
 * the watch began. It is the one answer to whether the style can be written.
 *
 * MapLibre 6.0.0's behaviour this relies on:
 * - A lost context after the style loaded comes back with the style when the
 *   browser restores it: MapLibre sets the saved style again, which fires
 *   `style.load` a frame later. Only a restore that never arrives is a
 *   failure.
 * - A lost context while no style is loaded cannot come back: MapLibre saves
 *   only a loaded style, so a restore would leave a map with no style. That
 *   covers a loss before the first style loaded and a second loss while a
 *   restored style is still loading (classed apart for triage).
 * - `GPUInitializationError` on a constructed map can only come from a
 *   restore that got no context (construction reports it before any listener
 *   exists).
 * - An error that names a source is isolated to that source; the rest of the
 *   map draws. An error without a source while no style is loaded (before the
 *   first `style.load`, or between a restore and its `style.load`) means that
 *   style never loads.
 *
 * This module's own rule: while a style is live, the only `error` event that
 * gives up the map is a `GPUInitializationError`.
 *
 * Not visible here: exceptions thrown inside MapLibre's render frame (they
 * propagate out of its animation-frame callback, not as an `error` event) and
 * a worker that never starts (MapLibre reports nothing).
 */
export function watchAtlasMapHealth(
  map: HealthMap,
  onUnrecoverable: (error: AtlasMapUnrecoverableError) => void,
  onStyleLiveChange?: (live: boolean) => void,
): AtlasMapHealth {
  let styleLive = styleHasLoaded(map)
  let styleEverLoaded = styleLive
  let contextLost = false
  let settled = false
  // Cleared by stop(), so a stopped watch reports nothing.
  let reportStyleLive = onStyleLiveChange
  let deadline: ReturnType<typeof setTimeout> | null = null

  function setStyleLive(live: boolean) {
    if (live === styleLive) return
    styleLive = live
    reportStyleLive?.(live)
  }

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
    styleEverLoaded = true
    endLoss()
    setStyleLive(true)
  })

  map.on('webglcontextlost', () => {
    if (!styleLive) {
      fail(styleEverLoaded ? 'context-lost-during-restore' : 'context-lost-before-style')
      return
    }
    // MapLibre destroyed the style; a restore re-creates it and fires
    // style.load again.
    startLoss()
    setStyleLive(false)
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

  if (styleLive) reportStyleLive?.(true)

  return {
    styleLive: () => styleLive,
    stop() {
      settled = true
      reportStyleLive = undefined
      endLoss()
    },
  }
}
