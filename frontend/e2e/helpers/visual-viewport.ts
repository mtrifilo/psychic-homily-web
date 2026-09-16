import type { Page } from '@playwright/test'

export type ViewportShim = {
  __shrinkVisualViewport: (height: number) => void
}

/**
 * Stands in for a software keyboard: the layout viewport keeps its height and
 * only the visual viewport shrinks, on demand, with a `resize` on the object
 * the surface under test listens to. Playwright cannot raise a real keyboard,
 * and `setViewportSize` would shrink both viewports, which is not the geometry
 * being tested - `interactive-widget=resizes-content` is the case where both
 * shrink.
 *
 * Pass to `page.addInitScript`, then drive with {@link raiseKeyboard}.
 */
export function installVisualViewportShim() {
  const real = window.visualViewport
  let height = real ? real.height : window.innerHeight
  const shim = {
    get width() {
      return real ? real.width : window.innerWidth
    },
    get height() {
      return height
    },
    offsetLeft: 0,
    offsetTop: 0,
    pageLeft: 0,
    pageTop: 0,
    scale: 1,
    onresize: null,
    onscroll: null,
    addEventListener: (type: string, listener: EventListener) =>
      real?.addEventListener(type, listener),
    removeEventListener: (type: string, listener: EventListener) =>
      real?.removeEventListener(type, listener),
    dispatchEvent: () => true,
  }
  Object.defineProperty(window, 'visualViewport', {
    configurable: true,
    get: () => shim,
  })
  // Listeners registered through the shim land on the real object, so the
  // resize has to be dispatched there for the surface to hear it.
  ;(window as unknown as ViewportShim).__shrinkVisualViewport = (
    next: number
  ) => {
    height = next
    real?.dispatchEvent(new Event('resize'))
  }
}

/**
 * Shrinks the shimmed visual viewport to `height`, as a keyboard rising would.
 * Only the visual-viewport `resize` fires, which is all a real keyboard fires
 * when the layout viewport keeps its height.
 */
export async function raiseKeyboard(page: Page, height: number) {
  await page.evaluate(
    h => (window as unknown as ViewportShim).__shrinkVisualViewport(h),
    height
  )
}
