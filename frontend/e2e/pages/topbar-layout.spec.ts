import { type Page } from '@playwright/test'
import { test, expect } from '../fixtures'

/**
 * PSY-1638: the TopBar must fit the viewport it is given.
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
 *     account cluster all appear at once (authenticated bar overflowed by
 *     10.6px).
 *   • 1280px — the `xl` boundary and a very common laptop width (21.5px).
 *   • 1440px — comfortable; a regression here would mean something far worse.
 *
 * Run signed out as well as signed in: the anonymous bar never overflowed the
 * viewport (its "login / sign-up" link is ~92px against the account cluster's
 * ~185px), but it hit the same rigidity from the other side — the flex
 * algorithm took the shortfall out of the only things that could give, stacking
 * the wordmark and the login link onto two lines in a 64px-tall bar. The
 * line-count assertions are what cover that half.
 *
 * Assertions are scoped to the <header>, not the document, so unrelated wide
 * content on the page under test cannot produce a false failure.
 */
const VIEWPORT_WIDTHS = [640, 1280, 1440] as const

/**
 * Geometry of the top bar's right-most control, measured in the page.
 * `trailingSelector` is the account cluster's last item, which is the thing
 * that fell off the right edge.
 */
async function measureTopBar(page: Page, trailingSelector: string) {
  return page.evaluate((selector: string) => {
    const header = document.querySelector('header')
    const node = header?.querySelector(selector)
    if (!header || !node) throw new Error(`header or ${selector} not found`)
    const wordmark = [...header.querySelectorAll('span')].find(
      span => span.textContent === 'Psychic Homily'
    )
    if (!wordmark) throw new Error('brand wordmark not found')

    // One client rect per line box the TEXT occupies — an exact line count,
    // with no pixel-height threshold to keep in sync. It has to be a Range over
    // the contents rather than the element's own `getClientRects()`: these are
    // flex items, so they are blockified and their own rect list is always
    // length 1 no matter how many lines they render.
    const lineCount = (el: Element) => {
      const range = document.createRange()
      range.selectNodeContents(el)
      return range.getClientRects().length
    }

    const rect = node.getBoundingClientRect()
    const hit = document.elementFromPoint(
      rect.left + rect.width / 2,
      rect.top + rect.height / 2
    )
    return {
      trailingRight: rect.right,
      headerScrollWidth: header.scrollWidth,
      // The failure the ticket actually described: not "it looks clipped" but
      // "no click can be aimed at the middle of the control".
      centreHitsControl: !!hit && node.contains(hit),
      wordmarkLineCount: lineCount(wordmark),
      // Only meaningful when the trailing control is text. The signed-in
      // trailing control is the avatar button, whose contents are a block-level
      // <div>, and a Range over a block yields rects that are not line boxes.
      trailingLineCount: lineCount(node),
    }
  }, trailingSelector)
}

function assertFits(
  measured: Awaited<ReturnType<typeof measureTopBar>>,
  width: number
) {
  expect(
    measured.trailingRight,
    `the trailing top-bar control runs past the ${width}px viewport`
  ).toBeLessThanOrEqual(width)
  expect(
    measured.headerScrollWidth,
    `top bar content overflows the ${width}px viewport`
  ).toBeLessThanOrEqual(width)
  expect(
    measured.centreHitsControl,
    'the centre of the trailing top-bar control is not hittable'
  ).toBe(true)
  // The other half of the same defect, and the only thing guarding `shrink-0`
  // on the left group and on the login link: the overflow assertions above
  // still pass when those are removed — the text just wraps again instead.
  expect(
    measured.wordmarkLineCount,
    'the brand wordmark wrapped onto more than one line'
  ).toBe(1)
}

test.describe('TopBar layout', () => {
  for (const width of VIEWPORT_WIDTHS) {
    test(`account cluster fits a ${width}px viewport`, async ({
      authenticatedPage: page,
    }) => {
      await page.setViewportSize({ width, height: 800 })
      await page.goto('/shows')

      const trigger = 'button[aria-label="User menu"]'
      await expect(page.locator(trigger)).toBeVisible({ timeout: 15_000 })
      assertFits(await measureTopBar(page, trigger), width)
    })

    test(`signed-out bar fits a ${width}px viewport`, async ({ page }) => {
      await page.setViewportSize({ width, height: 800 })
      await page.goto('/shows')

      // The link carries `?returnTo=` for the page it sits on (buildAuthHref),
      // so match the path prefix, not the bare href.
      const loginLink = 'a[href^="/auth"]'
      await expect(page.locator(`header ${loginLink}`)).toBeVisible({
        timeout: 15_000,
      })
      const measured = await measureTopBar(page, loginLink)
      assertFits(measured, width)
      // The login link is plain text, so its line count is a real wrap check —
      // it used to stack "login /" over "sign-up" at 640px and 1280px.
      expect(
        measured.trailingLineCount,
        'the login / sign-up link wrapped onto more than one line'
      ).toBe(1)
    })
  }
})
