import type { Page } from '@playwright/test'
import { test, expect } from '../fixtures'
import { lookupWorkerUserId } from '../fixtures/test-fixtures-reset'
import { workerAuthFile } from '../fixtures/auth'
import { runE2ESql } from '../e2e-db'

// The Settings calendar card's links block: each read-only field must show
// the END of its address, the part that tells the two addresses apart, at
// 390 and at 1440. jsdom has no layout, so this is checked in a browser from
// what the field actually draws: its overflow, its scroll position, its
// direction and the drawn width of the address's last segment.

const CARD_HREF = '/profile?tab=settings#calendar'

async function removeCalendarLink(userId: number) {
  runE2ESql(`DELETE FROM calendar_tokens WHERE user_id = ${userId}`)
}

/**
 * What a field draws: whether its address overflows, and whether the last
 * path segment ("/saved-shows.ics", "/follows.atom") lies entirely inside the
 * visible window. The window is computed from the field's scroll position
 * and the drawn widths of the text, measured with the field's own font.
 */
async function drawnEnd(page: Page) {
  return page.getByTestId('calendar-link-field').evaluateAll(fields =>
    fields.map(node => {
      const input = node as HTMLInputElement
      const style = getComputedStyle(input)
      const context = document.createElement('canvas').getContext('2d')!
      context.font = style.font
      const value = input.value
      const suffix = value.slice(value.lastIndexOf('/'))
      const textWidth = context.measureText(value).width
      const suffixWidth = context.measureText(suffix).width
      const inner =
        input.clientWidth -
        parseFloat(style.paddingLeft) -
        parseFloat(style.paddingRight)
      const overflows = input.scrollWidth > input.clientWidth
      // In a right-to-left field the scroll origin is the right edge: a
      // scrollLeft of 0 shows the last `inner` pixels of the text. Any other
      // scroll position shows `-scrollLeft` pixels further from the end.
      const hiddenAtEnd =
        style.direction === 'rtl'
          ? Math.max(0, -input.scrollLeft)
          : Math.max(0, textWidth - inner - input.scrollLeft)
      const visibleFromEnd = inner - 0.5
      return {
        suffix,
        overflows,
        direction: style.direction,
        endVisible:
          hiddenAtEnd < 1 &&
          (overflows ? suffixWidth <= visibleFromEnd : textWidth <= inner + 0.5),
      }
    })
  )
}

test.describe('Settings calendar card: the address fields', () => {
  test.describe.configure({ mode: 'serial' })

  let workerUserId: number

  test.beforeEach(async ({}, testInfo) => {
    workerUserId = await lookupWorkerUserId(workerAuthFile(testInfo.workerIndex))
    await removeCalendarLink(workerUserId)
  })

  test.afterEach(async () => {
    await removeCalendarLink(workerUserId)
  })

  for (const width of [390, 1440]) {
    test(`each field shows the end of its address at ${width}`, async ({
      authenticatedPage: page,
    }) => {
      // The person's own action makes the link; opening Settings never does.
      const created = await page.request.post('/api/calendar/token')
      expect(created.status()).toBeLessThan(400)

      await page.setViewportSize({ width, height: 900 })
      await page.goto(CARD_HREF)
      const fields = page.getByTestId('calendar-link-field')
      await expect(fields).toHaveCount(2, { timeout: 15_000 })
      // Nothing focused inside a field, so it sits at its resting scroll.
      await page.locator('#calendar').focus()

      const drawn = await drawnEnd(page)
      expect(drawn.map(field => field.suffix)).toEqual([
        '/saved-shows.ics',
        '/follows.atom',
      ])
      for (const field of drawn) {
        expect(field.endVisible, JSON.stringify(field)).toBe(true)
      }
      if (width === 390) {
        // At 390 neither address fits, so each field must be showing its end
        // rather than the shared beginning.
        for (const field of drawn) {
          expect(field.overflows, JSON.stringify(field)).toBe(true)
          expect(field.direction).toBe('rtl')
        }
      }
    })
  }
})
