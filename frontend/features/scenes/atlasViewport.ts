'use client'

import { useMediaQuery } from '@/lib/hooks/common/useMediaQuery'

/**
 * The Atlas's compact viewport: any viewport narrower than Tailwind's `lg`
 * breakpoint (64rem). This query decides the map's treatment at every width
 * below 64rem wherever the map mounts (see {@link atlasSceneListReason} for
 * where it does not).
 *
 * Width rather than pointer type: the phone boards are drawn by width, and a
 * viewport at `lg` or wider is desktop-sized whether a finger or a mouse
 * drives it.
 *
 * The exact complement of Tailwind's `lg:` variant (`min-width: 64rem`), so
 * an `lg:` utility and a branch on this query switch at the same width;
 * `not all and (...)` rather than range syntax for browsers that predate
 * Media Queries Level 4 ranges.
 */
export const ATLAS_COMPACT_VIEWPORT_QUERY = 'not all and (min-width: 64rem)'

/**
 * Whether the viewport is compact right now, read straight from `matchMedia`:
 * for a decision made once, such as a map style built at construction, where
 * a hook's first value could be its server snapshot. False where there is no
 * `matchMedia` (the server, jsdom without a shim).
 */
export function isAtlasCompactViewport(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia(ATLAS_COMPACT_VIEWPORT_QUERY).matches
}

/**
 * {@link isAtlasCompactViewport} as a subscription, re-rendering when the
 * viewport crosses the `lg` breakpoint (a window resize, a tablet rotating).
 * False on the server snapshot, per useMediaQuery's contract.
 */
export function useAtlasCompactViewport(): boolean {
  return useMediaQuery(ATLAS_COMPACT_VIEWPORT_QUERY)
}

/**
 * Below this Atlas pane width (the whole frame, rail included), a visitor who
 * prefers reduced motion gets the scene list instead of the map. At and above
 * it they get the map, which honours the preference itself (GlobeCanvas drops
 * its pulse rings, and camera moves cut rather than fly).
 *
 * A product setting, not a technical limit: phones get the list, tablet and
 * desktop panes keep the map. The reduced-motion phone case in
 * e2e/pages/atlas.spec.ts runs at 390px and assumes the value stays above it.
 */
export const ATLAS_REDUCED_MOTION_LIST_BELOW_PX = 640

/**
 * Why the Atlas renders its scene list (AtlasSceneList) in place of the map:
 * - `'no-webgl2'`: the browser cannot give MapLibre the WebGL2 context it
 *   requires.
 * - `'reduced-motion'`: the visitor prefers reduced motion and the pane is
 *   narrower than {@link ATLAS_REDUCED_MOTION_LIST_BELOW_PX}.
 * - `'map-failed'`: a map has already failed, in this mount or, for an
 *   {@link AtlasMapUnrecoverableError}, anywhere in this page load (see
 *   {@link markAtlasMapFailed}).
 */
export type AtlasSceneListReason = 'no-webgl2' | 'reduced-motion' | 'map-failed'

/**
 * Why the Atlas renders its scene list in place of the map, or null when it
 * renders the map. Every visitor with none of the reasons gets the map, phones
 * included. When several hold, the one returned is the one a retry cannot
 * change: no WebGL2 holds at every width and reduced motion keeps the map off
 * however often it is retried, so a failed map is named only when it is the
 * sole reason.
 */
export function atlasSceneListReason({
  paneWidthPx,
  supportsWebGL2,
  prefersReducedMotion,
  mapFailed,
}: {
  paneWidthPx: number
  supportsWebGL2: boolean
  prefersReducedMotion: boolean
  mapFailed: boolean
}): AtlasSceneListReason | null {
  if (!supportsWebGL2) return 'no-webgl2'
  if (prefersReducedMotion && paneWidthPx < ATLAS_REDUCED_MOTION_LIST_BELOW_PX) {
    return 'reduced-motion'
  }
  if (mapFailed) return 'map-failed'
  return null
}

/**
 * The ways an Atlas map can be left unable to draw, each the value of the
 * `atlas_map_failure` Sentry tag:
 * - `context-refused`: MapLibre got no WebGL2 context at construction.
 * - `context-lost`: the context was lost and not restored within
 *   ATLAS_CONTEXT_RESTORE_DEADLINE_MS of continuously visible time.
 * - `context-lost-during-restore`: the context was lost again while a
 *   restored style was still loading, which MapLibre cannot restore either.
 * - `context-lost-before-style`: the context was lost before the first style
 *   loaded, which MapLibre cannot restore (it saves only a loaded style).
 * - `context-restore-refused`: the browser restored the context but MapLibre
 *   got no WebGL2 context back.
 * - `style-load-failed`: the style errored before it loaded.
 * - `first-render-timeout`: the map rendered no frame after its scenes
 *   source loaded within ATLAS_FIRST_RENDER_DEADLINE_MS of continuously
 *   visible time after it was built (a render frame that throws, a worker
 *   that never starts).
 */
export type AtlasMapFailureClass =
  | 'context-refused'
  | 'context-lost'
  | 'context-lost-during-restore'
  | 'context-lost-before-style'
  | 'context-restore-refused'
  | 'style-load-failed'
  | 'first-render-timeout'

const FAILURE_MESSAGES: Record<AtlasMapFailureClass, string> = {
  'context-refused': 'MapLibre could not get a WebGL2 context',
  'context-lost': 'the WebGL context was lost and not restored',
  'context-lost-during-restore': 'the WebGL context was lost again during a restore',
  'context-lost-before-style': 'the WebGL context was lost before the style loaded',
  'context-restore-refused': 'MapLibre could not get a WebGL2 context back after a restore',
  'style-load-failed': 'the map style failed to load',
  'first-render-timeout': 'the map rendered no frame of its data in time',
}

/**
 * Thrown by the Atlas map when it cannot draw and the Atlas stops waiting for
 * it to: every {@link AtlasMapFailureClass}. For `context-lost` and
 * `first-render-timeout` that is a decision at a deadline, not proof the map
 * would never have drawn. AtlasGlobe swaps in the scene list for the rest of
 * the page load, or until the visitor's retry ({@link atlasMapRetryCanSucceed}).
 * Lives here, not beside the map, so AtlasGlobe can recognise it without
 * importing MapLibre.
 */
export class AtlasMapUnrecoverableError extends Error {
  readonly failureClass: AtlasMapFailureClass

  constructor(failureClass: AtlasMapFailureClass) {
    super(`Atlas map: ${FAILURE_MESSAGES[failureClass]}`)
    this.name = 'AtlasMapUnrecoverableError'
    this.failureClass = failureClass
  }
}

let atlasMapFailure: AtlasMapFailureClass | null = null

/**
 * Records that the Atlas map failed with an {@link AtlasMapUnrecoverableError}
 * of `failureClass`, so every later Atlas mount in this page load goes
 * straight to the scene list instead of building another map, until a
 * visitor's retry clears it ({@link clearAtlasMapFailure}). For a refused
 * context and an invalid inline style a new map would fail the same way; for
 * the loss classes and a first render that never came it is a policy: one
 * map that died is enough to stop building more in this page load unless the
 * visitor asks for one.
 */
export function markAtlasMapFailed(failureClass: AtlasMapFailureClass): void {
  atlasMapFailure = failureClass
}

/**
 * The class {@link markAtlasMapFailed} recorded in this page load, or null
 * when no failure is recorded.
 */
export function atlasMapFailureThisPage(): AtlasMapFailureClass | null {
  return atlasMapFailure
}

/**
 * Drops the recorded failure, so the next Atlas mount builds a map again. For
 * a visitor's retry, after {@link reprobeAtlasWebGL2} passed.
 */
export function clearAtlasMapFailure(): void {
  atlasMapFailure = null
}

/**
 * Whether a new map can draw after a failure of this class, which is when the
 * scene list offers the visitor a retry. Null is a failure that carries no
 * class. Each press builds one map, and a retried map that fails again is
 * reported to Sentry like the first; the visitor's presses are the only
 * bound on the loss classes and `first-render-timeout`.
 * - The loss classes and `context-restore-refused`: the browser took the
 *   context away from a map that had one, and a new map asks for a new
 *   context on a new canvas, which a recovered GPU gives. A new map the
 *   browser refuses fails as `context-refused`, which offers no retry, so a
 *   GPU that keeps refusing costs one more attempt at most.
 * - `context-refused`: MapLibre was refused a context after the WebGL2 probe
 *   passed, so a passing probe predicts nothing for it. A refused map also
 *   stays behind for the page load: maplibre-gl registers a module-global
 *   image-request throttle control before its painter check, and only
 *   `remove()`, which needs the painter, unregisters it.
 * - `style-load-failed`: the style is inline, so a new map gets the same
 *   style and fails the same way.
 * - `first-render-timeout`: the timed-out map had a context and is removed
 *   like any other, and MapLibre terminates its workers once no map holds
 *   them, so a new map starts on new workers. A render frame that throws on
 *   the same data throws again, which costs that retry one more deadline.
 * - No class: a canvas module that failed to import, or a thrown effect.
 *   React.lazy keeps a rejected import and throws it again on every mount, so
 *   a retry would report the same failure again.
 */
export function atlasMapRetryCanSucceed(failureClass: AtlasMapFailureClass | null): boolean {
  return failureClass !== null && RETRY_CAN_SUCCEED[failureClass]
}

const RETRY_CAN_SUCCEED: Record<AtlasMapFailureClass, boolean> = {
  'context-refused': false,
  'context-lost': true,
  'context-lost-during-restore': true,
  'context-lost-before-style': true,
  'context-restore-refused': true,
  'style-load-failed': false,
  'first-render-timeout': true,
}

/**
 * Whether a canvas from `createCanvas` yields a WebGL2 context, the only
 * context type MapLibre asks for. The probe asks for the smallest drawing
 * buffer that still answers the question, releases its context at once
 * (browsers cap the live contexts a page may hold), and counts a throwing
 * canvas as no support.
 */
export function probeWebGL2(createCanvas: () => HTMLCanvasElement): boolean {
  try {
    const canvas = createCanvas()
    canvas.width = 1
    canvas.height = 1
    const gl = canvas.getContext('webgl2', {
      antialias: false,
      depth: false,
      stencil: false,
    })
    if (!gl) return false
    gl.getExtension('WEBGL_lose_context')?.loseContext()
    return true
  } catch {
    return false
  }
}

let webGL2Support: boolean | undefined

/**
 * {@link probeWebGL2} on a fresh `<canvas>`, probed once per page load and the
 * answer kept, since each probe allocates a GPU context. Client only: false
 * where there is no `document`, and that answer is not kept.
 */
export function atlasSupportsWebGL2(): boolean {
  if (typeof document === 'undefined') return false
  webGL2Support ??= reprobeAtlasWebGL2()
  return webGL2Support
}

/**
 * {@link probeWebGL2} on a fresh `<canvas>` at every call; false where there
 * is no `document`. {@link atlasSupportsWebGL2} keeps its first answer for the
 * page load. A visitor's retry calls this directly, which leaves that kept
 * answer alone, so a failed retry changes nothing the Atlas renders.
 */
export function reprobeAtlasWebGL2(): boolean {
  if (typeof document === 'undefined') return false
  return probeWebGL2(() => document.createElement('canvas'))
}
