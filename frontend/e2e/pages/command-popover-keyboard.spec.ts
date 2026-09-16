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

/**
 * How far past `line` an edge paints, in whole pixels.
 *
 * Radix rounds the popover's translate to device pixels (`roundByDPR` in
 * `@floating-ui/react-dom`) while publishing the available height from the
 * UNROUNDED offset, so an anchor that lands on a fractional pixel leaves the
 * bottom edge up to half a device pixel past the line no matter how the bound
 * is written. That half pixel is Radix's own rounding, not slack in the bound:
 * whole pixels are what a reader sees, and the regression this spec guards
 * against was two of them.
 */
function wholePixelsBelow(edge: number, line: number): number {
  return Math.max(0, Math.floor(edge - line))
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

/**
 * Whether the rows scroll, as a single cheap read. Polling the full
 * {@link measure} for this one boolean would re-wait a frame per tick.
 */
function listScrolls(content: Locator): Promise<boolean> {
  return content.evaluate(el => {
    const list = el.querySelector('[cmdk-list]') as HTMLElement
    return list.scrollHeight > list.clientHeight
  })
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
 * Waits out the popover's enter animation, then takes the whole reading in one
 * round-trip so it comes from a single layout rather than from several that a
 * reposition tick can fall between.
 *
 * The wait is inside the reader, not a step beside it, because a reading taken
 * mid-animation is the failure this spec exists to catch: the enter animation
 * scales and slides the popover, so a rect read while it runs understates the
 * bottom edge by a couple of px, which is exactly the size of the overshoot
 * being asserted against. A settled reading has to be the only kind available.
 * A cancelled animation never finishes, so its rejection is swallowed rather
 * than failing the reading.
 */
function measure(content: Locator): Promise<Geometry> {
  return content.evaluate(async el => {
    await Promise.all(
      el.getAnimations().map(animation => animation.finished.catch(() => {}))
    )
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
    // The popover's whole border box, borders included, fits the room Radix
    // reported. On its own this is close to tautological, since that room IS
    // the frame's `max-height`.
    expect(geometry.contentHeight).toBeLessThanOrEqual(geometry.available)
    // The load-bearing half: the column stays inside the frame. A frame that
    // stopped being a column flex container would still satisfy the line above
    // while the column rendered at full height straight through it.
    expect(geometry.columnHeight).toBeLessThanOrEqual(geometry.contentHeight)
    // And so nothing paints below the keyboard.
    expect(wholePixelsBelow(geometry.contentBottom, ATLAS_KEYBOARD_HEIGHT)).toBe(
      0
    )
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
      .poll(() => listScrolls(content), { timeout: 10_000 })
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
    expect(geometry.columnHeight).toBeLessThanOrEqual(geometry.contentHeight)
    expect(wholePixelsBelow(geometry.contentBottom, CITY_KEYBOARD_HEIGHT)).toBe(
      0
    )
    // A bound that collapsed the surface would satisfy the line above too.
    expect(geometry.columnHeight).toBeGreaterThan(0)
    expect(page.getByPlaceholder('Search cities...')).toBeTruthy()
  })
})
