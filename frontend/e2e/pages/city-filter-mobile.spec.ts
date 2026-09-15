import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

const PHONE_VIEWPORT = { width: 390, height: 844 }

/**
 * Space left under the trigger once the simulated keyboard is up. Small
 * enough that the popover (search field plus list, around 345px) cannot fit
 * below the trigger, which is the geometry that had collision avoidance flip
 * it over the trigger and carry the search field off the top of the screen.
 */
const SPACE_UNDER_TRIGGER = 60

/**
 * Shrinks `window.visualViewport` the way an iOS software keyboard does: the
 * layout viewport keeps its height and only the visual viewport gets shorter.
 * Playwright cannot raise a real keyboard and `setViewportSize` would shrink
 * both viewports, so this is the only faithful simulation available here.
 * floating-ui builds its viewport rect from this object and re-measures on its
 * `resize`, so the popover sees exactly what it sees on a real phone.
 */
function installVisualViewportShim(height: number) {
  const real = window.visualViewport
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

  test('keeps the search field on screen when the keyboard shrinks the visual viewport', async ({
    page,
  }) => {
    await page.goto('/venues')

    const trigger = page.getByTestId('city-filter-combobox')
    await expect(trigger).toBeVisible({ timeout: 10_000 })

    // Measured rather than hardcoded so the case survives layout changes
    // above the filter bar: the point is the RATIO, not the pixel value.
    const closedBox = (await trigger.boundingBox())!
    const keyboardViewportHeight = Math.round(
      closedBox.y + closedBox.height + SPACE_UNDER_TRIGGER
    )
    expect(SPACE_UNDER_TRIGGER).toBeLessThan(closedBox.y)

    await page.addInitScript(installVisualViewportShim, keyboardViewportHeight)
    await page.reload()
    await expect(trigger).toBeVisible({ timeout: 10_000 })

    await trigger.click()

    const input = page.getByPlaceholder('Search cities...')
    await expect(input).toBeVisible()

    const triggerBox = (await trigger.boundingBox())!
    const inputBox = (await input.boundingBox())!

    // Below the trigger, not flipped over it.
    expect(inputBox.y).toBeGreaterThanOrEqual(triggerBox.y + triggerBox.height)
    // And inside the part of the screen the keyboard leaves visible.
    expect(inputBox.y + inputBox.height).toBeLessThanOrEqual(
      keyboardViewportHeight
    )
  })
})
