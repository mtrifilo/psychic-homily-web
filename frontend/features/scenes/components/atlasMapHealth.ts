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
 * How long a constructed Atlas map may go, within one continuous stretch of
 * the page being visible, without rendering a frame that has its scene data
 * loaded, before the Atlas gives up on it (see watchAtlasMapHealth).
 *
 * Derived from the atlas-perf workflow (scripts/atlas-perf.mjs) on the PR
 * previews, phone profile (iPhone 13, 4x CPU on the main thread, Fast 4G,
 * cold cache) under SwiftShader on GitHub's runners: the slowest first
 * rendered map of 69 cold runs across 23 runs from 2026-10-07 to 2026-10-11
 * was 7975 ms (run 38167710080). That figure counts from navigation start and
 * waits on every in-view tile, so it bounds the time from construction to the
 * watched frame from above. The deadline is twice it, rounded up to 16 s; the
 * factor of two is a choice, not a measurement. The runners measure no
 * desktop: one local 1440x900 run of the same script read 6.04 s, about
 * 10.6 s at the workflow's stated 1.75x runner factor (an estimate), which
 * the deadline clears by about 1.5x. It is above the 3.5 s phone gate.
 */
export const ATLAS_FIRST_RENDER_DEADLINE_MS = 16_000

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

// Boundary sections (see atlasMapBoundaryReporting) whose map failed with an
// AtlasMapUnrecoverableError in this page load.
const sectionsFailedThisPage = new Set<string>()

/**
 * Records a failure caught by the map boundary filed under `section`. An
 * {@link AtlasMapUnrecoverableError} latches that surface for the rest of the
 * page load: {@link atlasMapSectionFailedThisPage} answers true, and the
 * surface renders its fallback without building another map, so one failure
 * costs one map and one Sentry event per surface until a reload. Any other
 * error (a failed map chunk) is not latched. The Atlas keeps its own latch,
 * `markAtlasMapFailed` in atlasViewport.ts, for the same failures.
 */
export function latchAtlasMapFailure(section: string, error: unknown): void {
  if (error instanceof AtlasMapUnrecoverableError) sectionsFailedThisPage.add(section)
}

/** Whether {@link latchAtlasMapFailure} latched `section` in this page load. */
export function atlasMapSectionFailedThisPage(section: string): boolean {
  return sectionsFailedThisPage.has(section)
}

/** Test seam: clear every latch {@link latchAtlasMapFailure} set. */
export function resetAtlasMapLatchesForTest(): void {
  sectionsFailedThisPage.clear()
}

/**
 * A timer that counts only continuously visible time: `start()` arms it and
 * listens for the page's visibility, hiding the page cancels the timer, and
 * showing it again starts the full `ms` over (a page hidden at `start()` arms
 * nothing until shown). `cancel()` clears the timer and the listener; the
 * listener exists only between the two. `onExpire` runs once per armed timer
 * that runs out, after the timer and the listener are cleared.
 */
function visibleTimeDeadline(ms: number, onExpire: () => void) {
  let timer: ReturnType<typeof setTimeout> | null = null
  let started = false

  function arm() {
    if (timer !== null || document.visibilityState === 'hidden') return
    timer = setTimeout(() => {
      timer = null
      cancel()
      onExpire()
    }, ms)
  }

  function handleVisibility() {
    if (document.visibilityState !== 'hidden') {
      arm()
    } else if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }

  function cancel() {
    if (timer !== null) clearTimeout(timer)
    timer = null
    if (!started) return
    started = false
    document.removeEventListener('visibilitychange', handleVisibility)
  }

  return {
    start() {
      if (!started) {
        started = true
        document.addEventListener('visibilitychange', handleVisibility)
      }
      arm()
    },
    cancel,
  }
}

/**
 * The map surface this module reads. The events are MapLibre's own, so each
 * one arrives after MapLibre has acted on it: `webglcontextlost` after the
 * style has been saved (or not) and destroyed, `webglcontextrestored` after
 * the restore asked for a context. That event also follows a refused restore,
 * right after the error event: the refusal leaves the destroyed painter in
 * place, so MapLibre's no-painter guard before the event passes.
 */
type HealthMap = Pick<MapLibreMap, 'on' | 'off' | 'isSourceLoaded'>

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
   * Cancels both deadlines and stops reporting, to both callbacks (a reported
   * failure has already stopped both). The listeners stay on the map, inert,
   * so call it only as the map is removed.
   */
  stop(): void
}

/**
 * Watches a constructed map for the failures MapLibre does not recover from
 * on its own, and calls `onUnrecoverable` once, with the failure classified,
 * for the first of them.
 *
 * Attach it in the same task that constructs the map, before the map can fire
 * `style.load`: the watch learns that the style is live only from that event.
 * MapLibre 6.0.0 fires it a frame after construction at the earliest (an
 * inline style loads behind `browser.frameAsync`, a style URL behind its
 * fetch), so a watch attached right after `new Map()` sees every style.
 *
 * `onStyleLiveChange`, when given, is called with each change of
 * {@link AtlasMapHealth.styleLive}, after this watch has classified the event
 * behind it. It is the one answer to whether the style can be written.
 *
 * `firstRenderSourceId`, when given, turns on the first-render watchdog
 * described below; it names a GeoJSON source the map owns.
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
 * Not visible to those events: an exception thrown inside MapLibre's render
 * frame (it propagates out of the animation-frame callback, not as an `error`
 * event) and a worker that never starts (MapLibre reports nothing). With
 * `firstRenderSourceId`, the first-render watchdog covers both: the map is
 * given up as `first-render-timeout` when no `render` event has found the
 * style live and that source loaded within ATLAS_FIRST_RENDER_DEADLINE_MS of
 * continuously visible time after the watch began. `render` fires at the end
 * of `Map._render`, so a frame that throws fires none, and a GeoJSON source
 * is loaded only once the worker has answered. The source must be one the
 * map owns, never third-party tiles: a tile request has no timeout in
 * MapLibre, so a hung tile response holds `isStyleLoaded()` and
 * `areTilesLoaded()` false indefinitely, and a map that is drawing must not be
 * given up for that. The style-live check comes first because asking for a
 * source the style has not registered fires a sourceless `error` event, which
 * this watch would read as a failed style.
 */
export function watchAtlasMapHealth(
  map: HealthMap,
  onUnrecoverable: (error: AtlasMapUnrecoverableError) => void,
  onStyleLiveChange?: (live: boolean) => void,
  firstRenderSourceId?: string,
): AtlasMapHealth {
  let styleLive = false
  let styleEverLoaded = false
  let contextLost = false
  let settled = false
  // Cleared by stop() and by a failure, so a settled watch reports nothing.
  let reportStyleLive = onStyleLiveChange
  const restoreDeadline = visibleTimeDeadline(ATLAS_CONTEXT_RESTORE_DEADLINE_MS, () =>
    fail('context-lost'),
  )
  const firstRenderDeadline = visibleTimeDeadline(ATLAS_FIRST_RENDER_DEADLINE_MS, () =>
    fail('first-render-timeout'),
  )

  function setStyleLive(live: boolean) {
    if (live === styleLive) return
    styleLive = live
    reportStyleLive?.(live)
  }

  function settle() {
    settled = true
    reportStyleLive = undefined
    endLoss()
    endFirstRenderWatch()
  }

  function fail(failureClass: AtlasMapFailureClass) {
    if (settled) return
    settle()
    onUnrecoverable(new AtlasMapUnrecoverableError(failureClass))
  }

  function startLoss() {
    contextLost = true
    if (!settled) restoreDeadline.start()
  }

  function endLoss() {
    contextLost = false
    restoreDeadline.cancel()
  }

  function handleRender() {
    if (firstRenderSourceId === undefined) return
    if (styleLive && map.isSourceLoaded(firstRenderSourceId)) endFirstRenderWatch()
  }

  function endFirstRenderWatch() {
    if (firstRenderSourceId === undefined) return
    firstRenderDeadline.cancel()
    map.off('render', handleRender)
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

  if (firstRenderSourceId !== undefined) {
    map.on('render', handleRender)
    firstRenderDeadline.start()
  }

  return {
    styleLive: () => styleLive,
    stop: settle,
  }
}
