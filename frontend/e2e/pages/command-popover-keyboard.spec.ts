import { test } from '../fixtures/error-detection'
import {
  installVisualViewportShim,
  raiseKeyboard,
} from '../helpers/visual-viewport'
import { expect, type Locator, type Page } from '@playwright/test'

/**
 * Narrow enough to match the soft-keyboard viewport query and wide enough for
 * `/atlas` to render the globe rather than its scene list, which it swaps in
 * below 640px.
 */
const ATLAS_VIEWPORT = { width: 700, height: 600 }

/**
 * Wide enough that the city filter opens its combobox popover rather than the
 * bottom sheet, which takes over at or below 767px.
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
  /**
   * Radix measures the available height from the popover's OUTER top edge,
   * while the bound applies to the command column inside the popover's border.
   * The border is therefore the only thing that can paint past the line the
   * column was sized from, and it is read off the element rather than assumed.
   */
  border: number
  columnHeight: number
  columnMaxHeight: number
  contentBottom: number
  listScrolls: boolean
}

/**
 * One round-trip for every measurement, so the whole reading is taken from a
 * single layout rather than from six that the popover's reposition ticks can
 * fall between.
 */
function measure(content: Locator): Promise<Geometry> {
  return content.evaluate(el => {
    const style = getComputedStyle(el)
    const column = el.querySelector('[cmdk-root]') as HTMLElement
    const list = el.querySelector('[cmdk-list]') as HTMLElement
    const contentRect = el.getBoundingClientRect()
    return {
      available: Number.parseFloat(
        style.getPropertyValue('--radix-popover-content-available-height')
      ),
      border:
        Number.parseFloat(style.borderTopWidth) +
        Number.parseFloat(style.borderBottomWidth),
      columnHeight: column.getBoundingClientRect().height,
      columnMaxHeight: Number.parseFloat(getComputedStyle(column).maxHeight),
      contentBottom: contentRect.bottom,
      listScrolls: list.scrollHeight > list.clientHeight,
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

    // Open first, then raise the keyboard: the re-measure the shrinking visual
    // viewport triggers is what tightens the bound.
    await raiseKeyboard(page, ATLAS_KEYBOARD_HEIGHT)
    await expect
      .poll(() => availableHeightPx(content), { timeout: 10_000 })
      .toBeLessThan(ATLAS_KEYBOARD_HEIGHT)

    // The trigger is docked to the top of the map, so there is never more room
    // above it than below and the popover stays under it.
    await expect(content).toHaveAttribute('data-side', 'bottom')

    const geometry = await measure(content)
    // The bound itself: the column never exceeds the room Radix reported.
    expect(geometry.columnHeight).toBeLessThanOrEqual(geometry.available)
    // Nothing but the popover's own border paints below the keyboard line.
    expect(geometry.contentBottom).toBeLessThanOrEqual(
      ATLAS_KEYBOARD_HEIGHT + geometry.border
    )
    // The rows scroll inside that bound instead of running past it.
    expect(geometry.listScrolls).toBe(true)

    // The search field stays pinned at the top of the popover, above the line.
    const searchBox = (await search.boundingBox())!
    expect(searchBox.y + searchBox.height).toBeLessThanOrEqual(
      ATLAS_KEYBOARD_HEIGHT
    )
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

    // The bound is live on this surface too: the column's ceiling tracks the
    // room Radix reports. The seeded city list is short enough to fit inside
    // it, so the scrolling case is asserted on /atlas.
    const geometry = await measure(content)
    expect(geometry.columnMaxHeight).toBe(geometry.available)
    expect(geometry.contentBottom).toBeLessThanOrEqual(
      CITY_KEYBOARD_HEIGHT + geometry.border
    )
  })
})
