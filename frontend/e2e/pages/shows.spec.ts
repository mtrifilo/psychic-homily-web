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

  // The advance/door pair has to survive the whole stack to be worth
  // rendering: the list query has to select door_price, the response has to
  // carry it, and the card has to spell the pair rather than the advance half
  // alone. A component test proves only the last of those. `e2e-door-price-split`
  // is the seeded row carrying both columns.
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
  })

  test('pagination loads more shows', async ({ page }) => {
    await page.goto('/shows')

    await expect(page.locator('article').first()).toBeVisible({
      timeout: 10_000,
    })

    // Wait for "Load More" to appear (API returns has_more: true with >10 shows)
    const loadMoreButton = page.getByRole('button', { name: /load more/i })
    await expect(loadMoreButton).toBeVisible({ timeout: 5_000 })

    const initialCount = await page.locator('article').count()
    expect(initialCount).toBe(50) // Backend default limit

    await loadMoreButton.click()

    // Wait for additional shows to load
    await page.waitForFunction(
      (initial) => document.querySelectorAll('article').length > initial,
      initialCount,
      { timeout: 10_000 }
    )

    const newCount = await page.locator('article').count()
    expect(newCount).toBeGreaterThan(initialCount)
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
