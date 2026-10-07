/**
 * A once-per-page-load signal that the Atlas's first-map window is over. It
 * says nothing about whether a map exists: it releases when the map draws its
 * first full frame, when the Atlas knows no map will draw (the scene list, the
 * error state, nothing to place), or, while the Atlas page has armed them
 * (armAtlasMapReadyFallbacks), on the visitor's first input or at the cap
 * below, whichever comes first. The app chrome's link prefetches wait for it
 * on `/atlas` (components/layout/nav/ChromeLink.tsx).
 *
 * Once released it stays released for the page load, so a later visit to
 * `/atlas` in the same tab holds nothing.
 */

/**
 * How long after the Atlas page mounts the signal releases on its own. Held
 * links resume their viewport prefetch on release (a hover or touchstart
 * during the hold prefetches nothing), so the cap bounds the delay for a map
 * that is slow, stalled, or never draws (a lost WebGL context, a MapLibre
 * worker that never answers).
 */
export const ATLAS_MAP_READY_CAP_MS = 10_000

/**
 * The input events that count as the visitor starting to use the page. Any of
 * them releases the signal, so the chrome's links prefetch from then on.
 */
export const ATLAS_MAP_READY_INPUT_EVENTS = ['pointerdown', 'keydown', 'wheel'] as const

let ready = false
const listeners = new Set<() => void>()

/** Releases the signal. Idempotent. */
export function markAtlasMapReady(): void {
  if (ready) return
  ready = true
  for (const listener of [...listeners]) listener()
}

/** Whether the signal has released in this page load. */
export function isAtlasMapReady(): boolean {
  return ready
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
 * user input. The Atlas page arms it on every mount; arms are independent, and
 * the returned function disarms this one (on unmount, or once the signal has
 * released).
 */
export function armAtlasMapReadyFallbacks(): () => void {
  if (ready) return () => {}
  const options = { capture: true, passive: true } as const
  // A listener of this arm's own, so disarming one arm never removes the
  // input release of another.
  const onInput = () => markAtlasMapReady()
  let unsubscribe = () => {}
  const timer = setTimeout(markAtlasMapReady, ATLAS_MAP_READY_CAP_MS)
  const disarm = () => {
    clearTimeout(timer)
    for (const type of ATLAS_MAP_READY_INPUT_EVENTS) {
      window.removeEventListener(type, onInput, options)
    }
    unsubscribe()
  }
  unsubscribe = subscribeAtlasMapReady(disarm)
  for (const type of ATLAS_MAP_READY_INPUT_EVENTS) {
    window.addEventListener(type, onInput, options)
  }
  return disarm
}
