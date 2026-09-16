import { test } from '../fixtures/error-detection'
import { expect, type Locator } from '@playwright/test'

const PHONE_VIEWPORT = { width: 390, height: 844 }

/**
 * Height the visual viewport is shrunk to once the "keyboard" is up. Small
 * enough that a sheet sized to the full screen would have its search field and
 * its first rows under the keyboard.
 */
const KEYBOARD_VIEWPORT_HEIGHT = 302

type ViewportShim = { __shrinkVisualViewport: (height: number) => void }

/**
 * Stands in for an iOS software keyboard: the layout viewport keeps its height
 * and only the visual viewport shrinks, on demand, with a `resize` on the
 * object the sheet listens to. Playwright cannot raise a real keyboard, and
 * `setViewportSize` would shrink both viewports, which is not the geometry
 * being tested - `interactive-widget=resizes-content` is the case where both
 * shrink, and the sheet's own fallback is what this drives.
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
  // resize has to be dispatched there for the sheet to hear it.
  ;(window as unknown as ViewportShim).__shrinkVisualViewport = (next: number) => {
    height = next
    real?.dispatchEvent(new Event('resize'))
  }
}

/**
 * The sheet slides in over 500ms, and neither `toBeVisible` nor `boundingBox`
 * waits for an animation to land - a bare measurement reads the sheet mid-slide,
 * still below the fold. Polls the bottom edge until it is inside `limit`.
 */
async function settleOnScreen(locator: Locator, limit: number) {
  await expect
    .poll(
      async () => {
        const box = await locator.boundingBox()
        return box ? Math.round(box.y + box.height) : Number.MAX_SAFE_INTEGER
      },
      { timeout: 10_000 }
    )
    .toBeLessThanOrEqual(limit)
}

/**
 * The routes that render a city filter against the e2e seed. `/artists` also
 * mounts the filter, but the seeded artists carry no city, so the filter bar
 * renders nothing there; the fifth consumer, `UpcomingShowsList`, has no route
 * pointing at it. Both are covered at the unit level instead.
 */
const MOUNT_SITES = ['/venues', '/shows', '/'] as const

test.describe('City filter opens as a bottom sheet on a phone viewport', () => {
  test.use({ viewport: PHONE_VIEWPORT })

  for (const path of MOUNT_SITES) {
    test(`opens with the search field on screen, page unmoved (${path})`, async ({
      page,
    }) => {
      await page.goto(path)

      const trigger = page.getByTestId('city-filter-combobox')
      await expect(trigger).toBeVisible({ timeout: 10_000 })
      await expect(trigger).toHaveAttribute('aria-haspopup', 'dialog')

      // Settle the scroll position the click would otherwise change, so the
      // reading after it is about the sheet and not about Playwright.
      await trigger.scrollIntoViewIfNeeded()
      const scrollBefore = await page.evaluate(() => window.scrollY)

      await trigger.click()

      const sheet = page.getByTestId('city-filter-sheet')
      await expect(sheet).toBeVisible()
      // The page never scrolls to make room: the sheet is its own layer.
      expect(await page.evaluate(() => window.scrollY)).toBe(scrollBefore)

      const search = page.getByTestId('city-filter-sheet-search')
      const list = page.getByTestId('city-filter-sheet-list')
      const apply = page.getByTestId('city-filter-sheet-apply')

      await settleOnScreen(apply, PHONE_VIEWPORT.height)

      const searchBox = (await search.boundingBox())!
      expect(searchBox.y).toBeGreaterThanOrEqual(0)
      expect(searchBox.y + searchBox.height).toBeLessThanOrEqual(
        PHONE_VIEWPORT.height
      )

      // The list scrolls inside the sheet rather than growing it past the
      // screen: it is bounded, and the apply button below it stays visible.
      const listBox = (await list.boundingBox())!
      const applyBox = (await apply.boundingBox())!
      expect(listBox.y).toBeGreaterThanOrEqual(searchBox.y + searchBox.height)
      expect(applyBox.y).toBeGreaterThanOrEqual(listBox.y + listBox.height)
      expect(applyBox.y + applyBox.height).toBeLessThanOrEqual(
        PHONE_VIEWPORT.height
      )
      expect(
        await list.evaluate(el => el.scrollHeight >= el.clientHeight)
      ).toBe(true)
    })
  }
})

for (const path of ['/venues', '/shows'] as const) {
  test.describe(`City filter on a phone viewport (${path})`, () => {
    test.use({ viewport: PHONE_VIEWPORT })

    test('keeps the search field and the list above a raised keyboard', async ({
      page,
    }) => {
      await page.addInitScript(installVisualViewportShim)
      await page.goto(path)

      const trigger = page.getByTestId('city-filter-combobox')
      await expect(trigger).toBeVisible({ timeout: 10_000 })
      await trigger.click()

      const search = page.getByTestId('city-filter-sheet-search')
      await expect(search).toBeVisible()

      // Open first, then raise the keyboard: the re-measure that the shrinking
      // visual viewport triggers is what lifts the sheet.
      await page.evaluate(
        (height) =>
          (window as unknown as ViewportShim).__shrinkVisualViewport(height),
        KEYBOARD_VIEWPORT_HEIGHT
      )
      await settleOnScreen(
        page.getByTestId('city-filter-sheet-apply'),
        KEYBOARD_VIEWPORT_HEIGHT
      )

      const searchBox = (await search.boundingBox())!
      expect(searchBox.y).toBeGreaterThanOrEqual(0)
      expect(searchBox.y + searchBox.height).toBeLessThanOrEqual(
        KEYBOARD_VIEWPORT_HEIGHT
      )

      // The apply button is the whole point of the sheet, so it has to survive
      // the keyboard too.
      const applyBox = (await page
        .getByTestId('city-filter-sheet-apply')
        .boundingBox())!
      expect(applyBox.y + applyBox.height).toBeLessThanOrEqual(
        KEYBOARD_VIEWPORT_HEIGHT
      )
    })

    test('applies the picked city on the button, not on the tick', async ({
      page,
    }) => {
      await page.goto(path)

      const trigger = page.getByTestId('city-filter-combobox')
      await expect(trigger).toBeVisible({ timeout: 10_000 })
      await trigger.click()

      const sheet = page.getByTestId('city-filter-sheet')
      await expect(sheet).toBeVisible()
      await settleOnScreen(
        page.getByTestId('city-filter-sheet-apply'),
        PHONE_VIEWPORT.height
      )

      const firstOption = sheet
        .locator('label:has([data-testid^="city-sheet-option-"])')
        .first()
      const urlBeforeTick = page.url()

      await firstOption.click()
      await expect(firstOption.locator('input')).toBeChecked()
      // A tick edits the sheet only; the page behind it is untouched.
      expect(page.url()).toBe(urlBeforeTick)

      const apply = page.getByTestId('city-filter-sheet-apply')
      await expect(apply).toHaveText(/^Show [\d,]+ \w+$/)
      await apply.click()

      await expect(sheet).toBeHidden()
      await expect(trigger).toBeFocused()
      // The applied city reaches the page as the existing `cities` param; the
      // sheet writes through the same handler the combobox always used.
      // Not the `cities=all` sentinel: a real city reached the URL.
      await expect(page).toHaveURL(/[?&]cities=(?!all(?:&|$))/)
    })
  })
}
