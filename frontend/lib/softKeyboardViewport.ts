/**
 * Viewports on which focusing a text field raises a software keyboard: at or
 * below Tailwind's `md` breakpoint, plus every coarse pointer. Both halves
 * carry weight - a tablet in landscape is wider than `md` and still raises a
 * keyboard, and a narrow desktop window raises none but is the same geometry.
 *
 * The query is deliberately wider than "has a keyboard": a touch laptop and a
 * narrow mouse-driven window both match and get the touch treatment.
 */
export const SOFT_KEYBOARD_VIEWPORT_QUERY =
  '(max-width: 767px), (pointer: coarse)'

/**
 * True when a software keyboard can shrink the visual viewport under an open
 * overlay, which is the condition under which a filter opens as a bottom sheet
 * rather than as a popover anchored to its trigger.
 *
 * Returns `false` where `matchMedia` is unavailable, including on the server,
 * which is also the server snapshot a `useSyncExternalStore` caller renders.
 */
export function matchesSoftKeyboardViewport(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia(SOFT_KEYBOARD_VIEWPORT_QUERY).matches
}

/**
 * Subscribes to changes in {@link SOFT_KEYBOARD_VIEWPORT_QUERY}. Returns a
 * no-op unsubscriber where `matchMedia` is unavailable, including on the
 * server, so a caller can wire this into `useSyncExternalStore` unguarded.
 */
export function subscribeSoftKeyboardViewport(
  onChange: () => void
): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return () => {}
  }
  const query = window.matchMedia(SOFT_KEYBOARD_VIEWPORT_QUERY)
  if (typeof query.addEventListener === 'function') {
    query.addEventListener('change', onChange)
    return () => query.removeEventListener('change', onChange)
  }
  // Safari before 14 ships only the deprecated listener pair.
  query.addListener?.(onChange)
  return () => query.removeListener?.(onChange)
}
