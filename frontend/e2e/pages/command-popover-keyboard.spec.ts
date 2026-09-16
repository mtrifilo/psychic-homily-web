import { test } from '../fixtures/error-detection'
import {
  installVisualViewportShim,
  raiseKeyboard,
} from '../helpers/visual-viewport'
import { expect, type Locator, type Page } from '@playwright/test'

/**
 * A landscape phone: the shape where a keyboard and this popover can actually
 * meet. `/atlas` swaps in a searchless scene list below 640px, so a portrait
 * phone has no atlas search at all, and the wide-but-short landscape case is
 * also the one that leaves the popover least room.
 */
const ATLAS_VIEWPORT = { width: 844, height: 390 }

/**
 * The city filter's popover needs more than 767px, below which its trigger
 * opens a bottom sheet instead. Reachable here because the `chromium` project
 * emulates no touch, so the `(pointer: coarse)` half of that query is false.
 */
const CITY_POPOVER_VIEWPORT = { width: 900, height: 600 }

/** Heights the visual viewport is shrunk to once the "keyboard" is up. */
const ATLAS_KEYBOARD_HEIGHT = 190
const CITY_KEYBOARD_HEIGHT = 260

/** The popover content element Radix positions and publishes its size on. */
function popoverContent(page: Page): Locator {
  return page.locator('[data-radix-popper-content-wrapper] > *').first()
}

/** The px value Radix reports as the room the popover has on screen. */
function availableHeightPx(content: Locator): Promise<number> {
  return content.evaluate(el =>
    Number.parseFloat(
      getComputedStyle(el).getPropertyValue(
        '--radix-popover-content-available-height'
      )
    )
  )
}

type Geometry = {
  available: number
  columnHeight: number
  /**
   * The popover's border box: the element Radix positions, the element the
   * available height is measured from, and so the element the bound is on.
   */
  contentBottom: number
  contentHeight: number
  contentMaxHeight: number
  listScrolls: boolean
  optionCount: number
}

/**
 * One round-trip for every measurement, so the whole reading is taken from a
 * single layout rather than from several that a reposition tick can fall
 * between.
 *
 * The enter animation scales and slides the popover, so a rect read while it
 * runs describes a smaller box than the one that settles on screen. Waiting it
 * out is what makes these numbers the geometry the reader gets: a reading taken
 * mid-animation understates the bottom edge by a couple of px, which is enough
 * to hide exactly the overshoot this spec exists to catch.
 */
function measure(content: Locator): Promise<Geometry> {
  return content.evaluate(async el => {
    await Promise.all(el.getAnimations().map(animation => animation.finished))
    await new Promise(resolve => requestAnimationFrame(resolve))
    const style = getComputedStyle(el)
    const rect = el.getBoundingClientRect()
    const column = el.querySelector('[cmdk-root]') as HTMLElement
    const list = el.querySelector('[cmdk-list]') as HTMLElement
    return {
      available: Number.parseFloat(
        style.getPropertyValue('--radix-popover-content-available-height')
      ),
      columnHeight: column.getBoundingClientRect().height,
      contentBottom: rect.bottom,
      contentHeight: rect.height,
      contentMaxHeight: Number.parseFloat(style.maxHeight),
      listScrolls: list.scrollHeight > list.clientHeight,
      optionCount: el.querySelectorAll('[cmdk-item]').length,
    }
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
    const search = page.getByPlaceholder('City or state…')
    await expect(search).toBeVisible()

    const unbounded = await measure(content)
    // The scrolling assertion below only means anything if the seeded scenes
    // overflow the room the keyboard leaves. Fails loudly if the seed shrinks.
    expect(unbounded.optionCount).toBeGreaterThan(1)
    expect(unbounded.listScrolls).toBe(false)

    // Open first, then raise the keyboard: the re-measure the shrinking visual
    // viewport triggers is what tightens the bound.
    await raiseKeyboard(page, ATLAS_KEYBOARD_HEIGHT)
    await expect
      .poll(() => availableHeightPx(content), { timeout: 10_000 })
      .toBeLessThan(ATLAS_KEYBOARD_HEIGHT)

    const geometry = await measure(content)
    // The bound itself: the popover's whole border box, borders included, fits
    // the room Radix reported.
    expect(geometry.contentHeight).toBeLessThanOrEqual(geometry.available)
    // So nothing at all paints below the keyboard, with nothing forgiven.
    expect(geometry.contentBottom).toBeLessThanOrEqual(ATLAS_KEYBOARD_HEIGHT)
    // The rows scroll inside that bound instead of running past it.
    expect(geometry.listScrolls).toBe(true)
    // The field being typed into is never squeezed out by the bound.
    const searchBox = (await search.boundingBox())!
    expect(searchBox.y + searchBox.height).toBeLessThanOrEqual(
      ATLAS_KEYBOARD_HEIGHT
    )

    // Lowering the keyboard gives the room back: the bound tracks the viewport
    // in both directions rather than latching at its smallest reading.
    await raiseKeyboard(page, ATLAS_VIEWPORT.height)
    await expect
      .poll(async () => (await measure(content)).listScrolls, {
        timeout: 10_000,
      })
      .toBe(false)
    const restored = await measure(content)
    // The ceiling is back to the full room, and the column has grown past the
    // squeezed height rather than latching at it. Its exact height is not
    // asserted: a relayout after scrolling moves it a few px either way.
    expect(restored.contentMaxHeight).toBeGreaterThan(geometry.available)
    expect(restored.columnHeight).toBeGreaterThan(geometry.columnHeight)
  })
})

/**
 * The city filter's own popover never meets a real keyboard: every viewport
 * that raises one opens the bottom sheet instead. It is here as the second
 * consumer of the shared bound, driven by the same shrinking visual viewport.
 */
test.describe('City filter popover on a shrinking visual viewport', () => {
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
    await expect(page.getByPlaceholder('Search cities...')).toBeVisible()

    const content = popoverContent(page)
    const before = await availableHeightPx(content)
    await raiseKeyboard(page, CITY_KEYBOARD_HEIGHT)
    await expect
      .poll(() => availableHeightPx(content), { timeout: 10_000 })
      .toBeLessThan(before)

    // The bound is live on this surface too: the popover's ceiling tracks the
    // room Radix reports. The seeded city list is short enough to fit inside
    // it, so the scrolling case is asserted on /atlas.
    const geometry = await measure(content)
    expect(geometry.contentMaxHeight).toBeCloseTo(geometry.available, 1)
    expect(geometry.contentBottom).toBeLessThanOrEqual(CITY_KEYBOARD_HEIGHT)
    // A bound that collapsed the surface would satisfy the line above too.
    expect(geometry.columnHeight).toBeGreaterThan(0)
    expect(page.getByPlaceholder('Search cities...')).toBeTruthy()
  })
})
