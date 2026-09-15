/**
 * Viewports on which focusing a text field raises a software keyboard: at or
 * below Tailwind's `md` breakpoint, plus every coarse pointer. Both halves
 * carry weight - a tablet in landscape is wider than `md` and still raises a
 * keyboard, and a narrow desktop window raises none but is the same geometry.
 *
 * The query is deliberately wider than "has a keyboard": a touch laptop and a
 * narrow mouse-driven window both match and get the pinned treatment, which
 * includes a page scroll on open that neither needed.
 */
export const SOFT_KEYBOARD_VIEWPORT_QUERY =
  '(max-width: 767px), (pointer: coarse)'

/**
 * True when a software keyboard can shrink the visual viewport under an open
 * overlay, which is the condition under which a combobox popover has to be
 * pinned below its trigger and bounded to the space left under it.
 *
 * Read this at the moment an overlay opens rather than subscribing: the answer
 * only decides how the overlay is mounted, so a subscription would add a
 * listener and a hydration-time re-render to every page that renders a closed
 * one. Returns `false` where `matchMedia` is unavailable, including on the
 * server.
 */
export function matchesSoftKeyboardViewport(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia(SOFT_KEYBOARD_VIEWPORT_QUERY).matches
}
