import { test, expect } from '../fixtures'

/**
 * PSY-1638: the authenticated TopBar must fit the viewport it is given.
 *
 * The bug this guards: every control in the account cluster is `shrink-0`, and
 * the search field carried a fixed `w-[220px] xl:w-[320px]`, so the whole top
 * bar was rigid. At exactly `xl` (1280px) the 8-item PrimaryNav appears AND the
 * search jumps 220→320px in the same breakpoint — 1277px of content in a
 * 1232px box. `justify-between` lays out from the start when free space is
 * negative, so the surplus fell off the right edge: the avatar's right edge
 * landed at x≈1301 in a 1280px window and `elementFromPoint` at its centre
 * returned nothing. It was found by accident (a PSY-1615 harness click that
 * could never land), not by a test — hence this one.
 *
 * Both widths that overflowed before the fix are covered, plus one that did
 * not, so the test would also catch a fix that merely moved the cliff:
 *   • 640px — the `sm` boundary, where the wordmark, search, theme toggle and
 *     account cluster all appear at once (overflowed by 10.6px).
 *   • 1280px — the `xl` boundary and a very common laptop width (21.5px).
 *   • 1440px — comfortable; a regression here would mean something far worse.
 *
 * Assertions are scoped to the <header>, not the document, so unrelated
 * wide content on the page under test cannot produce a false failure.
 */
const VIEWPORT_WIDTHS = [640, 1280, 1440] as const

test.describe('TopBar layout (authenticated)', () => {
  for (const width of VIEWPORT_WIDTHS) {
    test(`account cluster fits a ${width}px viewport`, async ({
      authenticatedPage: page,
    }) => {
      await page.setViewportSize({ width, height: 800 })
      await page.goto('/shows')

      const trigger = page.locator('button[aria-label="User menu"]')
      await expect(trigger).toBeVisible({ timeout: 15_000 })

      const measured = await page.evaluate(() => {
        const header = document.querySelector('header')
        const node = document.querySelector('button[aria-label="User menu"]')
        if (!header || !node) throw new Error('header or user-menu trigger not found')
        const rect = node.getBoundingClientRect()
        const hit = document.elementFromPoint(
          rect.left + rect.width / 2,
          rect.top + rect.height / 2
        )
        return {
          triggerRight: rect.right,
          headerScrollWidth: header.scrollWidth,
          // The failure the ticket actually described: not "it looks clipped"
          // but "no click can be aimed at the middle of the control".
          centreHitsTrigger: !!hit && node.contains(hit),
        }
      })

      expect(
        measured.triggerRight,
        `user-menu trigger runs past the ${width}px viewport`
      ).toBeLessThanOrEqual(width)
      expect(
        measured.headerScrollWidth,
        `top bar content overflows the ${width}px viewport`
      ).toBeLessThanOrEqual(width)
      expect(
        measured.centreHitsTrigger,
        'the centre of the user-menu trigger is not hittable'
      ).toBe(true)
    })
  }
})
