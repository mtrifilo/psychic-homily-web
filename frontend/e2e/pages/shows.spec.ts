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

    // Links, not buttons, so the deep pages are crawlable and bookmarkable.
    // Deliberately NOT a claim that a no-JavaScript fetcher sees page 2's rows
    // there: the route seeds page 1 and never reads `searchParams`, so the rows
    // are swapped client-side and `?page=N` canonicalizes back to `/shows`.
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

    // Page 1 writes no `page`, so stepping back lands on the bare URL and the
    // list follows it. This is the half a unit test cannot make: it exercises
    // the browser's own history, not a mocked param.
    await page.goBack()
    await expect(page).toHaveURL(/\/shows$/)
    await expect
      .poll(
        async () =>
          page.locator('article').first().getAttribute('aria-label'),
        { timeout: 15_000 }
      )
      .toBe(firstPageLeadRow)
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

/**
 * The date-addressed lists (PSY-2061). Every case drives the MONTH STRIP rather
 * than a hard-coded month: which months have shows is a property of the seed,
 * and a literal here would rot the first time the seed moved.
 */
test.describe('Shows month and day routes', () => {
  /** The first month the strip offers, as the served HTML carries it. */
  async function firstMonthHref(page: import('@playwright/test').Page) {
    await page.goto('/shows')
    const strip = page.getByTestId('month-strip')
    await expect(strip).toBeVisible({ timeout: 15_000 })
    const href = await strip
      .locator('a[href^="/shows/"]')
      .first()
      .getAttribute('href')
    expect(href).toMatch(/^\/shows\/\d{4}\/\d{2}$/)
    return href as string
  }

  test('the root carries the month strip as real links', async ({ page }) => {
    const response = await page.goto('/shows')
    const html = (await response?.text()) ?? ''

    // In the RESPONSE BYTES, not merely painted after hydration: the strip is
    // the crawl path into the month family.
    expect(html).toMatch(/href="\/shows\/\d{4}\/\d{2}"/)
  })

  test('a month deep link renders the day-grouped list with its month active', async ({
    page,
  }) => {
    test.setTimeout(60_000)

    const href = await firstMonthHref(page)
    const [, , year, month] = href.split('/')

    const response = await page.goto(href)
    expect(response?.status()).toBe(200)

    await expect(page).toHaveTitle(/Shows in \w+ \d{4}/)
    await expect(
      page.getByRole('heading', { level: 1, name: /^Shows in \w+ \d{4}$/ })
    ).toBeVisible()

    // The rows, grouped under day headings that link to their own day page.
    await expect(page.getByTestId('day-grouped-show-list')).toBeVisible({
      timeout: 15_000,
    })
    const dayHeading = page.locator(`a[href^="/shows/${year}/${month}/"]`).first()
    await expect(dayHeading).toBeVisible()

    // The strip marks this month, and the mark is on the month's OWN link.
    const current = page.getByTestId('month-strip').locator('[aria-current="page"]')
    await expect(current).toHaveAttribute('href', href)
  })

  test('pages inside a month at the month URL', async ({ page }) => {
    test.setTimeout(60_000)

    const href = await firstMonthHref(page)
    await page.goto(href)
    await expect(page.getByTestId('day-grouped-show-list')).toBeVisible({
      timeout: 15_000,
    })

    const page2 = page.getByRole('link', { name: /^Page 2\b/ }).first()
    if ((await page2.count()) === 0) {
      test.skip(
        true,
        'the seeded month fits one page; the pager is exercised on the root above'
      )
      return
    }

    // Nested, not onto the root: `?page=` means the same thing inside a month
    // as it does on the list, and only the list it pages through changes.
    await expect(page2).toHaveAttribute('href', `${href}?page=2`)
    await page.waitForLoadState('networkidle')
    await page2.click()
    await expect(page).toHaveURL(new RegExp(`${href}\\?page=2$`))
  })

  test('a day deep link renders that day', async ({ page }) => {
    test.setTimeout(60_000)

    const href = await firstMonthHref(page)
    await page.goto(href)
    await expect(page.getByTestId('day-grouped-show-list')).toBeVisible({
      timeout: 15_000,
    })

    const dayHref = await page
      .locator('h2 a[href^="/shows/"]')
      .first()
      .getAttribute('href')
    expect(dayHref).toMatch(/^\/shows\/\d{4}\/\d{2}\/\d{2}$/)

    const response = await page.goto(dayHref as string)
    expect(response?.status()).toBe(200)
    await expect(
      page.getByRole('heading', { level: 1, name: /^Shows on \w+ \d+, \d{4}$/ })
    ).toBeVisible()
  })

  /**
   * SHAPE is settled by `proxy.ts` before anything renders, so these are real
   * HTTP 404s rather than a not-found body at 200 — the soft-404 the proxy
   * exists to prevent.
   */
  for (const path of [
    '/shows/2026/13',
    '/shows/2026/9',
    '/shows/2026/11/31',
    '/shows/2027/02/29',
  ]) {
    test(`${path} returns HTTP 404`, async ({ page }) => {
      const response = await page.goto(path)
      expect(response?.status()).toBe(404)
    })
  }

  /**
   * The legacy Hugo form still owns a final segment that is NOT day-shaped:
   * `/shows/{yyyy}/{mm}/{slug}` flattens to `/shows/{slug}`, which is evaluated
   * before the proxy and before the router. The day route's grammar is excluded
   * from that pattern in `next.config.ts`, and this is the other half of that
   * split — without it, narrowing the redirect could silently swallow the
   * legacy URLs it exists for.
   */
  test('a legacy Hugo show URL still flattens to the show', async ({ page }) => {
    await page.goto('/shows')
    // The row's own Details link, not any `/shows/...` href: the day headings
    // are date-shaped by construction, which is exactly the set the redirect no
    // longer claims.
    const href = (await page
      .getByRole('link', { name: 'Details' })
      .first()
      .getAttribute('href')) as string
    const showSlug = href.split('/').pop() as string

    await page.goto(`/shows/2026/11/${showSlug}`)

    await expect(page).toHaveURL(new RegExp(`/shows/${showSlug}$`))
  })

  /**
   * A well-formed month with no shows is a not-found PAGE. Its status is 200
   * under `cacheComponents` — the shell has streamed by the time the histogram
   * read resolves — with the `noindex` Next injects, so the assertion is on the
   * rendered body rather than the status. A status-bearing month-existence
   * probe is the follow-up that would make this a hard 404.
   */
  test('a month with no upcoming shows renders the not-found page', async ({
    page,
  }) => {
    await page.goto('/shows/2199/01')

    await expect(page.getByRole('heading', { name: '404' })).toBeVisible({
      timeout: 15_000,
    })
    await expect(page.getByTestId('day-grouped-show-list')).toHaveCount(0)
  })
})
