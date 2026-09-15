import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

const PHONE_VIEWPORT = { width: 390, height: 844 }

/**
 * Height the visual viewport is shrunk to once the "keyboard" is up. Small
 * enough that the popover, around 345px of search field plus list, cannot fit
 * under a trigger sitting where the page renders it.
 */
const KEYBOARD_VIEWPORT_HEIGHT = 302

/** `--topbar-height` in globals.css, the sticky header the trigger scrolls under. */
const TOPBAR_HEIGHT = 56

type ViewportShim = { __shrinkVisualViewport: (height: number) => void }

/**
 * Stands in for an iOS software keyboard: the layout viewport keeps its
 * height and only the visual viewport shrinks, on demand, with a `resize` on
 * the object floating-ui listens to. Playwright cannot raise a real keyboard,
 * and `setViewportSize` would shrink both viewports, which is not the geometry
 * being tested. floating-ui builds its viewport rect from `window.visualViewport`
 * and re-measures on that object's `resize`, so this drives the real path.
 */
function installVisualViewportShim() {
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
  // resize has to be dispatched there for floating-ui to hear it.
  ;(window as unknown as ViewportShim).__shrinkVisualViewport = (next: number) => {
    height = next
    real?.dispatchEvent(new Event('resize'))
  }
}

test.describe('City filter on a phone viewport', () => {
  test.use({ viewport: PHONE_VIEWPORT })

  test('opens below the trigger with the search field on screen', async ({
    page,
  }) => {
    await page.goto('/venues')

    const trigger = page.getByTestId('city-filter-combobox')
    await expect(trigger).toBeVisible({ timeout: 10_000 })
    expect(await page.evaluate(() => window.scrollY)).toBe(0)

    await trigger.click()

    const input = page.getByPlaceholder('Search cities...')
    await expect(input).toBeVisible()

    const triggerBox = (await trigger.boundingBox())!
    const inputBox = (await input.boundingBox())!

    expect(inputBox.y).toBeGreaterThanOrEqual(triggerBox.y + triggerBox.height)
    expect(inputBox.y + inputBox.height).toBeLessThanOrEqual(
      PHONE_VIEWPORT.height
    )
  })

  test('keeps the search field and the list on screen when the keyboard rises', async ({
    page,
  }) => {
    await page.addInitScript(installVisualViewportShim)
    await page.goto('/venues')

    const trigger = page.getByTestId('city-filter-combobox')
    await expect(trigger).toBeVisible({ timeout: 10_000 })
    expect(await page.evaluate(() => window.scrollY)).toBe(0)

    // Open first, then raise the keyboard: the re-measure that the shrinking
    // visual viewport triggers is what decides the popover's side.
    await trigger.click()
    const input = page.getByPlaceholder('Search cities...')
    await expect(input).toBeVisible()

    await page.evaluate(
      (height) =>
        (window as unknown as ViewportShim).__shrinkVisualViewport(height),
      KEYBOARD_VIEWPORT_HEIGHT
    )
    await page.waitForTimeout(200)

    const triggerBox = (await trigger.boundingBox())!
    const inputBox = (await input.boundingBox())!

    // Opening scrolled the trigger up for room, and its scroll margin kept it
    // clear of the sticky topbar rather than under it.
    expect(triggerBox.y).toBeGreaterThanOrEqual(TOPBAR_HEIGHT)
    // Below the trigger, not flipped over it.
    expect(inputBox.y).toBeGreaterThanOrEqual(triggerBox.y + triggerBox.height)
    // And inside the part of the screen the keyboard leaves visible.
    expect(inputBox.y + inputBox.height).toBeLessThanOrEqual(
      KEYBOARD_VIEWPORT_HEIGHT
    )

    // The list under it has to be usable too, not just the field: the first
    // option is what a user reaches for after typing.
    const firstOption = page.getByRole('option').first()
    const optionBox = (await firstOption.boundingBox())!
    expect(optionBox.y + optionBox.height).toBeLessThanOrEqual(
      KEYBOARD_VIEWPORT_HEIGHT
    )
  })
})
