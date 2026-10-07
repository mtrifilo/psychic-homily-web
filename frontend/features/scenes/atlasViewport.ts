'use client'

import { useMediaQuery } from '@/lib/hooks/common/useMediaQuery'

/**
 * The Atlas's compact viewport: any viewport narrower than Tailwind's `lg`
 * breakpoint (64rem). This query decides the map's treatment at every width
 * below 64rem wherever the map mounts (see {@link atlasRendersSceneList} for
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
 * Whether the Atlas renders its scene list (AtlasSceneList) in place of the
 * map. Two visitor conditions: the browser cannot give MapLibre the WebGL2
 * context it requires (at any width), or the visitor prefers reduced motion
 * and the pane is narrower than {@link ATLAS_REDUCED_MOTION_LIST_BELOW_PX}.
 * Separately, `mapFailed` says a map has already failed: in this mount, or,
 * for a refused WebGL2 context, anywhere in this page load (see
 * {@link markAtlasMapFailed}). Every other visitor gets the map, phones
 * included.
 */
export function atlasRendersSceneList({
  paneWidthPx,
  supportsWebGL2,
  prefersReducedMotion,
  mapFailed,
}: {
  paneWidthPx: number
  supportsWebGL2: boolean
  prefersReducedMotion: boolean
  mapFailed: boolean
}): boolean {
  if (mapFailed || !supportsWebGL2) return true
  return prefersReducedMotion && paneWidthPx < ATLAS_REDUCED_MOTION_LIST_BELOW_PX
}

/**
 * Thrown by the Atlas map when MapLibre could not get a WebGL2 context, a
 * failure that repeats on every later attempt in the same page load. Lives
 * here, not beside the map, so AtlasGlobe can recognise it without importing
 * MapLibre.
 */
export class AtlasMapContextError extends Error {
  constructor() {
    super('Atlas map: MapLibre could not get a WebGL2 context')
    this.name = 'AtlasMapContextError'
  }
}

let atlasMapFailed = false

/**
 * Records that the Atlas map failed in a way a retry would repeat
 * ({@link AtlasMapContextError}), so every later Atlas mount in this page
 * load goes straight to the scene list instead of building (and leaking)
 * another map that would fail the same way.
 */
export function markAtlasMapFailed(): void {
  atlasMapFailed = true
}

/** Whether {@link markAtlasMapFailed} has run in this page load. */
export function atlasMapFailedThisPage(): boolean {
  return atlasMapFailed
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
  webGL2Support ??= probeWebGL2(() => document.createElement('canvas'))
  return webGL2Support
}
