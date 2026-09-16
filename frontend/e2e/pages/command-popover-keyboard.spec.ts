import { test } from '../fixtures/error-detection'
import { expect, type Locator, type Page } from '@playwright/test'

/**
 * Narrow enough to match the soft-keyboard viewport query and wide enough for
 * `/atlas` to render the globe rather than its scene list.
 */
const ATLAS_VIEWPORT = { width: 700, height: 600 }

/**
 * Wide enough that the city filter opens its combobox popover rather than the
 * bottom sheet, which takes over at or below 767px.
 */
const CITY_POPOVER_VIEWPORT = { width: 900, height: 600 }

/** Height the visual viewport is shrunk to once the "keyboard" is up. */
const ATLAS_KEYBOARD_HEIGHT = 190
const CITY_KEYBOARD_HEIGHT = 260

type ViewportShim = { __shrinkVisualViewport: (height: number) => void }

/**
 * Stands in for a software keyboard: the layout viewport keeps its height and
 * only the visual viewport shrinks, with a `resize` on the object the popover
 * positioner listens to. Playwright cannot raise a real keyboard, and
 * `setViewportSize` would shrink both viewports, which is not the geometry
 * being tested.
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
  // resize has to be dispatched there for the positioner to hear it.
  ;(window as unknown as ViewportShim).__shrinkVisualViewport = (
    next: number
  ) => {
    height = next
    real?.dispatchEvent(new Event('resize'))
  }
}

/** The popover content element Radix positions and publishes its size on. */
function popoverContent(page: Page): Locator {
  return page.locator('[data-radix-popper-content-wrapper] > *').first()
}

async function raiseKeyboard(page: Page, height: number) {
  await page.evaluate(
    h => (window as unknown as ViewportShim).__shrinkVisualViewport(h),
    height
  )
}

/** The px value Radix reports as the room the popover has on screen. */
async function availableHeightPx(content: Locator): Promise<number> {
  const raw = await content.evaluate(el =>
    getComputedStyle(el).getPropertyValue(
      '--radix-popover-content-available-height'
    )
  )
  return Number.parseFloat(raw)
}

/**
 * Radix measures the available height from the popover's OUTER top edge, while
 * the bound applies to the command column inside the popover's border. The
 * border is therefore the only thing that can paint past the line the column
 * was sized from, and it is read off the element rather than assumed.
 */
async function borderHeightPx(content: Locator): Promise<number> {
  return content.evaluate(el => {
    const style = getComputedStyle(el)
    return (
      Number.parseFloat(style.borderTopWidth) +
      Number.parseFloat(style.borderBottomWidth)
    )
  })
}

test.describe('Atlas search popover under a software keyboard', () => {
  test.use({ viewport: ATLAS_VIEWPORT })

  test('bounds the command column to the space the keyboard leaves', async ({
    page,
  }) => {
    await page.addInitScript(installVisualViewportShim)
    await page.goto('/atlas')

    const trigger = page.getByRole('combobox', { name: 'Search scenes' })
    await expect(trigger).toBeVisible({ timeout: 30_000 })
    await trigger.click()

    const content = popoverContent(page)
    const command = content.locator('[cmdk-root]')
    const list = content.locator('[cmdk-list]')
    await expect(page.getByPlaceholder('City or state…')).toBeVisible()

    // Open first, then raise the keyboard: the re-measure the shrinking visual
    // viewport triggers is what tightens the bound. Only the visual viewport
    // event is dispatched, which is all a real keyboard fires when the layout
    // viewport keeps its height.
    await raiseKeyboard(page, ATLAS_KEYBOARD_HEIGHT)
    await expect
      .poll(() => availableHeightPx(content), { timeout: 10_000 })
      .toBeLessThan(ATLAS_KEYBOARD_HEIGHT)

    // The trigger is docked to the top of the map, so there is never more room
    // above it than below and the popover stays under it.
    await expect(content).toHaveAttribute('data-side', 'bottom')

    const available = await availableHeightPx(content)
    const commandBox = (await command.boundingBox())!
    const contentBox = (await content.boundingBox())!
    const border = await borderHeightPx(content)

    // The bound itself: the column never exceeds the room Radix reported.
    expect(commandBox.height).toBeLessThanOrEqual(available)
    // Nothing but the popover's own border paints below the keyboard line.
    expect(contentBox.y + contentBox.height).toBeLessThanOrEqual(
      ATLAS_KEYBOARD_HEIGHT + border
    )
    // The rows scroll inside that bound instead of running past it.
    expect(
      await list.evaluate(el => el.scrollHeight > el.clientHeight)
    ).toBe(true)
    // The search field stays put at the top of the popover.
    const searchBox = (await page
      .getByPlaceholder('City or state…')
      .boundingBox())!
    expect(searchBox.y).toBeGreaterThanOrEqual(0)
    expect(searchBox.y + searchBox.height).toBeLessThanOrEqual(
      ATLAS_KEYBOARD_HEIGHT
    )
  })
})

test.describe('City filter popover under a software keyboard', () => {
  test.use({ viewport: CITY_POPOVER_VIEWPORT })

  test('carries the same bound on the desktop combobox popover', async ({
    page,
  }) => {
    await page.addInitScript(installVisualViewportShim)
    await page.goto('/venues')

    const trigger = page.getByTestId('city-filter-combobox')
    await expect(trigger).toBeVisible({ timeout: 30_000 })
    // The combobox path, not the bottom sheet.
    await expect(trigger).toHaveAttribute('role', 'combobox')
    await trigger.click()

    const content = popoverContent(page)
    const command = content.locator('[cmdk-root]')
    await expect(page.getByPlaceholder('Search cities...')).toBeVisible()

    const before = await availableHeightPx(content)
    await raiseKeyboard(page, CITY_KEYBOARD_HEIGHT)
    await expect
      .poll(() => availableHeightPx(content), { timeout: 10_000 })
      .toBeLessThan(before)

    // The bound is live on this surface too: the column's ceiling tracks the
    // room Radix reports. The seeded city list is short enough to fit inside
    // it, so the scrolling case is asserted on /atlas.
    const available = await availableHeightPx(content)
    const maxHeight = await command.evaluate(
      el => getComputedStyle(el).maxHeight
    )
    expect(Number.parseFloat(maxHeight)).toBe(available)

    const contentBox = (await content.boundingBox())!
    const border = await borderHeightPx(content)
    expect(contentBox.y).toBeGreaterThanOrEqual(0)
    expect(contentBox.y + contentBox.height).toBeLessThanOrEqual(
      CITY_KEYBOARD_HEIGHT + border
    )
  })
})
