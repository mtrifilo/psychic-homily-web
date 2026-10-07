/**
 * A once-per-page-load signal that the Atlas no longer needs the network and
 * the main thread to itself: its map has drawn its first full frame, or the
 * Atlas showed the scene list instead, or the visitor started using the page,
 * or the cap below ran out. The app chrome's link prefetches wait for it on
 * `/atlas` (components/layout/nav/ChromeLink.tsx).
 *
 * Directive-free so both GlobeCanvas and AtlasGlobe (feature code) and the
 * chrome can import it. Once released it stays released for the page load, so
 * a later visit to `/atlas` in the same tab defers nothing.
 */

/** The route whose first map the signal guards. */
export const ATLAS_PATH = '/atlas'

/**
 * How long after the Atlas page mounts the signal releases on its own. The
 * deferred work is only postponed, never dropped, so the cap bounds the delay
 * for a map that is slow, stalled, or never draws (a lost WebGL context, a
 * MapLibre worker that never answers). 10 s is well past the phone profile's
 * first map (about 3.4 s) with room for a slow network.
 */
export const ATLAS_MAP_READY_CAP_MS = 10_000

/**
 * The input events that count as the visitor starting to use the page. Any of
 * them releases the signal: once someone is tapping, typing or scrolling the
 * map, a navigation may be next, and the chrome's prefetches should be in.
 */
export const ATLAS_MAP_READY_INPUT_EVENTS = ['pointerdown', 'keydown', 'wheel'] as const

let ready = false
let resolveReady: () => void = () => {}
const readyPromise = new Promise<void>(resolve => {
  resolveReady = resolve
})
const listeners = new Set<() => void>()

/** Releases the signal. Idempotent. */
export function markAtlasMapReady(): void {
  if (ready) return
  ready = true
  resolveReady()
  for (const listener of [...listeners]) listener()
}

/** Whether the signal has released in this page load. */
export function isAtlasMapReady(): boolean {
  return ready
}

/** Resolves when the signal releases (at once if it already has). */
export function whenAtlasMapReady(): Promise<void> {
  return readyPromise
}

/** `useSyncExternalStore` subscription: `listener` runs once, on release. */
export function subscribeAtlasMapReady(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/**
 * Arms the release paths that do not depend on the map: the cap and the first
 * user input. Called once by the Atlas page on mount; the returned function
 * disarms both (on unmount, or once the signal has released).
 */
export function armAtlasMapReadyFallbacks(): () => void {
  if (ready) return () => {}
  const timer = setTimeout(markAtlasMapReady, ATLAS_MAP_READY_CAP_MS)
  const onInput = () => markAtlasMapReady()
  const options = { capture: true, passive: true } as const
  for (const type of ATLAS_MAP_READY_INPUT_EVENTS) {
    window.addEventListener(type, onInput, options)
  }
  const disarm = () => {
    clearTimeout(timer)
    for (const type of ATLAS_MAP_READY_INPUT_EVENTS) {
      window.removeEventListener(type, onInput, options)
    }
    unsubscribe()
  }
  const unsubscribe = subscribeAtlasMapReady(disarm)
  return disarm
}
