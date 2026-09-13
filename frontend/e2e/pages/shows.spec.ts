import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Shows list', () => {
  test('loads and displays upcoming shows', { tag: '@smoke' }, async ({ page }) => {
    await page.goto('/shows')

    await expect(page).toHaveTitle(/Upcoming Shows/)

    await expect(
      page.getByRole('heading', { name: /upcoming shows/i })
    ).toBeVisible()

    // Wait for show cards to render (client-side fetch)
    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Multiple shows visible
    const showCount = await page.locator('article').count()
    expect(showCount).toBeGreaterThanOrEqual(5)
  })

  test('show cards contain artist links, venue, and details link', async ({
    page,
  }) => {
    await page.goto('/shows')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    const firstShow = page.locator('article').first()

    // Has at least one link (artist or venue)
    await expect(firstShow.locator('a').first()).toBeVisible()

    // Has a "Details" link pointing to /shows/...
    await expect(
      firstShow.getByRole('link', { name: 'Details' })
    ).toBeVisible()
  })

  // A door price has to survive the whole stack to be worth recording: the
  // list query has to select door_price, the response has to carry it, and the
  // card has to read both columns rather than the advance one alone. A
  // component test proves only the last of those. `e2e-door-price-split` and
  // `e2e-door-price-only` are the two seeded rows that record one.
  test('renders the advance/door price split on a list row', async ({
    page,
  }) => {
    await page.goto('/shows')

    const splitRow = page.getByRole('article', {
      name: 'E2E [door-price-split]',
    })
    await expect(splitRow).toBeVisible({ timeout: 10_000 })
    await expect(splitRow).toContainText('$20/$25')
    // The glyphs are aria-hidden and paired with a spelled-out sibling, so a
    // screen reader is not left reading a price as punctuation.
    await expect(
      splitRow.getByText('$20 advance, $25 at the door')
    ).toBeAttached()

    // A door price with no advance price reads as a bare number, so the check
    // that matters is that it is served at all: a list gating on
    // `price != null` renders this row with no price and still passes a
    // substring assertion on the row's other text.
    const doorOnlyRow = page.getByRole('article', {
      name: 'E2E [door-price-only]',
    })
    await expect(doorOnlyRow).toBeVisible()
    await expect(doorOnlyRow).toContainText('$15')
    await expect(doorOnlyRow).not.toContainText('/$')
  })

  // PSY-2060: the list pages by NUMBER, and every page is a real `<a href>`.
  // The claims worth guarding are the ones a unit test cannot make: that the
  // page is 50 rows against the real backend, that `Later` is a link carrying
  // `?page=2` rather than a button, and that following it actually serves
  // different rows.
  test('pagination serves a second page at its own URL', async ({ page }) => {
    // Double the 30s default: this test navigates twice, and the second
    // navigation is the first compile of `?page=2` on a cold dev server. It
    // finishes in about 25s there, so this is headroom, not a hiding place.
    test.setTimeout(60_000)

    await page.goto('/shows')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    const firstPageCount = await page.locator('article').count()
    expect(firstPageCount).toBe(50) // The page size the list requests

    const firstPageLeadRow = await page
      .locator('article')
      .first()
      .getAttribute('aria-label')

    // Links, not buttons: a fetcher with no JavaScript reaches page 2 too.
    await expect(
      page.getByRole('link', { name: /^later$/i }).first()
    ).toHaveAttribute('href', /[?&]page=2(?:&|$)/)

    // Settle before clicking. The pager is a real `<a href>`, so it is in the
    // server HTML and passes Playwright's actionability checks before hydration
    // wires the router; a click in that window leaves the browser to follow the
    // href itself, and on a cold dev server each such navigation restarts a
    // compile the next one interrupts. Waiting once here is what a reader does
    // anyway.
    await page.waitForLoadState('networkidle')

    await page.getByRole('link', { name: /^Page 2\b/ }).first().click()

    await expect(page).toHaveURL(/[?&]page=2(?:&|$)/)

    // The URL moves first and the rows follow, once the list re-reads `?page=`.
    // `keepPreviousData` holds page 1 on screen throughout, so the pager's own
    // position is the signal that the page actually turned.
    await expect(page.getByText(/Page 2 of \d+/).first()).toBeVisible({
      timeout: 30_000,
    })

    await expect
      .poll(
        async () =>
          page.locator('article').first().getAttribute('aria-label'),
        { timeout: 15_000 }
      )
      .not.toBe(firstPageLeadRow)
  })

  // PSY-1623: `/shows` is the only page that links the scene-week pages into the
  // crawl graph, so the claim worth guarding is that the anchors are in the
  // RESPONSE BYTES — not merely painted after hydration. The raw body is
  // asserted directly, because the block streams inside a Suspense boundary and
  // a DOM-level check would pass even if it had become client-only.
  test('serves scene-week links in the /shows HTML', async ({ page }) => {
    const response = await page.goto('/shows')
    const html = (await response?.text()) ?? ''

    const hrefs = [...html.matchAll(/href="(\/scenes\/[a-z0-9-]+\/week)"/g)].map(
      m => m[1]
    )
    expect(hrefs.length).toBeGreaterThan(0)

    // Every row also carries its count in the accessible name, which is the
    // half that has to match the destination page (`shows_calendar_week`).
    expect(html).toMatch(/aria-label="[^"]*(shows|No shows) this week"/)

    // The rendered block agrees with the bytes, so the link is real to a reader
    // as well as to a crawler.
    const first = hrefs[0]
    await expect(page.locator(`a[href="${first}"]`).first()).toBeVisible({
      timeout: 10_000,
    })
  })
})
