import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'

/**
 * The directory is a CITY's rooms. An anonymous visitor in this harness has no
 * favourites and no IP geo (the `/api/geo` route answers `{geo: null}` with no
 * Vercel headers in front of it), so a bare `/venues` here is the no-derivable-
 * city state, and a city page is reached by its own address.
 */
const PHOENIX = '/venues?cities=Phoenix%2CAZ'

async function tableIsUp(page: Page) {
  await expect(page.getByRole('table')).toBeVisible({ timeout: 10_000 })
}

/** The widest thing the document lays out, measured after the rows arrive. */
function documentOverflow(page: Page) {
  return page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }))
}

test.describe('Venues directory', () => {
  test('a city page lists its rooms in a dense table', { tag: '@smoke' }, async ({
    page,
  }) => {
    await page.goto(PHOENIX)

    await expect(
      page.getByRole('heading', { level: 1, name: 'Venues in Phoenix, AZ' })
    ).toBeVisible()

    await tableIsUp(page)

    // The row anatomy: the room links to its own page, and the count column
    // carries its unit in the header rather than in every cell.
    const rooms = page.getByRole('table').locator('tbody tr')
    expect(await rooms.count()).toBeGreaterThanOrEqual(3)
    await expect(
      page.getByRole('columnheader', { name: /upcoming shows/i })
    ).toBeVisible()
    // The place column names the field it carries.
    await expect(
      page.getByRole('columnheader', { name: /^address$/i })
    ).toBeVisible()
    await expect(
      page.getByRole('link', { name: 'Crescent Ballroom' })
    ).toHaveAttribute('href', '/venues/crescent-ballroom-phoenix-az')

    // Every row is on the page; nothing is behind a press.
    await expect(page.getByRole('button', { name: /load more/i })).toHaveCount(0)
  })

  test('a sortable header writes ?sort= and the default writes nothing', async ({
    page,
  }) => {
    await page.goto(PHOENIX)
    await tableIsUp(page)

    await page.getByTestId('venue-sort-header-name').click()
    await expect(page).toHaveURL(/[?&]sort=name(?:&|$)/)
    await expect(
      page.getByRole('columnheader', { name: /^room/i })
    ).toHaveAttribute('aria-sort', 'ascending')

    // The city survives the sort write: the header builds on the params that
    // are already there.
    await expect(page).toHaveURL(/cities=Phoenix/)

    // Back to the default order, which is the one the URL never names.
    await page.getByTestId('venue-sort-header-upcoming').click()
    await expect(page).not.toHaveURL(/sort=/)
    await expect(page).toHaveURL(/cities=Phoenix/)
  })

  test('the pager links page 2 as a real URL that asks for the next slice', async ({
    page,
  }) => {
    // This catalogue has far fewer than 50 rooms in any one city, so the pager
    // is exercised against a synthesized page of the real response shape.
    const PAGE_SIZE = 50
    const TOTAL = 120
    const requested: string[] = []
    // A URL PREDICATE, not a glob: `?` is not a wildcard in Playwright's glob,
    // and the list request is the one whose path ENDS at `/venues`, which is
    // what keeps `/venues/cities` on the real backend.
    //
    // The PAGE is at that path too, so the document navigation has to fall
    // through: fulfilling it hands the browser the JSON to render, which is a
    // blank page and no table.
    await page.route(
      url => url.pathname.endsWith('/venues'),
      async route => {
        if (route.request().resourceType() === 'document') {
          return route.fallback()
        }
        const url = new URL(route.request().url())
        requested.push(url.search)
        const offset = Number(url.searchParams.get('offset') ?? 0)
        const venues = Array.from(
          { length: Math.min(PAGE_SIZE, TOTAL - offset) },
          (_, i) => ({
            id: offset + i + 1,
            slug: `seeded-room-${offset + i + 1}`,
            name: `Seeded Room ${offset + i + 1}`,
            address: '1 Test St',
            city: 'Phoenix',
            state: 'AZ',
            timezone: 'America/Phoenix',
            verified: true,
            upcoming_show_count: TOTAL - offset - i,
            shows_this_week: 0,
            social: {},
            next_show: {
              event_date: '2026-12-01T03:00:00Z',
              slug: `show-${offset + i + 1}`,
              title: '',
            },
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          })
        )
        await route.fulfill({
          json: { venues, total: TOTAL, limit: PAGE_SIZE, offset },
        })
      }
    )

    await page.goto(PHOENIX)
    await tableIsUp(page)

    const pagers = page.getByTestId('pagination')
    await expect(pagers).toHaveCount(2)

    const pageTwo = pagers.first().getByRole('link', { name: 'Page 2' })
    await expect(pageTwo).toHaveAttribute('href', /[?&]page=2(?:&|$)/)
    await expect(pageTwo).toHaveAttribute('href', /cities=Phoenix/)

    await pageTwo.click()
    await expect(page).toHaveURL(/[?&]page=2(?:&|$)/)
    await expect
      .poll(() => requested.some(s => s.includes(`offset=${PAGE_SIZE}`)))
      .toBe(true)
    await expect(page.getByText(`Seeded Room ${PAGE_SIZE + 1}`)).toBeVisible()
  })

  test('with no derivable city it offers the busiest cities instead of a table', async ({
    page,
  }) => {
    await page.goto('/venues')

    const chooser = page.getByTestId('venues-city-chooser')
    await expect(chooser).toBeVisible({ timeout: 10_000 })
    await expect(chooser).toContainText('Choose a city to see its rooms.')
    await expect(page.getByRole('table')).toHaveCount(0)

    const phoenixChip = chooser.getByTestId('venues-busiest-phoenix-az')
    await expect(phoenixChip).toHaveAttribute('href', PHOENIX)

    await phoenixChip.click()
    await expect(page).toHaveURL(/cities=Phoenix/)
    await tableIsUp(page)
  })

  /**
   * The directory's indexing posture. Asserted in a browser rather than only in
   * `venuesPageMetadata.test.ts` because the metadata is DYNAMIC: it is
   * resolved per request against the city facet, so a unit test cannot show
   * that the facet actually reached it.
   */
  test.describe('indexing', () => {
    // SCOPED TO `head`, which is the assertion and not a detail. This route's
    // metadata is resolved from `searchParams`, so the server streams these
    // tags into the BODY of the document; React hoists them into the head as it
    // renders. A selector over the whole document would pass either way, and a
    // canonical or a robots directive outside the head is ignored, so the
    // thing worth pinning is where they END UP, in the DOM a crawler reads.
    const canonical = (page: Page) =>
      page.locator('head > link[rel="canonical"]').first()

    test('a city page names itself in the title and the canonical', async ({
      page,
    }) => {
      await page.goto(PHOENIX)
      await tableIsUp(page)

      await expect(page).toHaveTitle('Venues in Phoenix, AZ | Psychic Homily')
      await expect(
        page.locator('head > meta[name="description"]').first()
      ).toHaveAttribute('content', /Live-music rooms in Phoenix, AZ/)
      await expect(canonical(page)).toHaveAttribute(
        'href',
        'https://psychichomily.com/venues?cities=Phoenix%2CAZ'
      )
    })

    test('the bare directory keeps the generic title and the root canonical', async ({
      page,
    }) => {
      await page.goto('/venues')

      await expect(page).toHaveTitle('Venues | Psychic Homily')
      await expect(canonical(page)).toHaveAttribute(
        'href',
        'https://psychichomily.com/venues'
      )
    })

    /**
     * The other half of the owner-locked pagination exception: a page of a
     * city is its own document, but only a page the city HAS. The
     * seed holds well under one page of Phoenix rooms, so `?page=2` is past the
     * end here and canonicalizes to the city itself, which is also what the
     * body of such a page tells the reader to go back to.
     *
     * The self-canonical for a page that DOES exist needs a city spanning two
     * pages, which this seed has no way to express; `venuesPageMetadata.test.ts`
     * drives it against a facet sized to two pages instead.
     */
    test('a page past the end of a city canonicalizes to the city', async ({
      page,
    }) => {
      await page.goto(`${PHOENIX}&page=2`)

      await expect(canonical(page)).toHaveAttribute(
        'href',
        'https://psychichomily.com/venues?cities=Phoenix%2CAZ'
      )
    })

    test('a city with no rooms is reachable and asks not to be indexed', async ({
      page,
    }) => {
      const response = await page.goto('/venues?cities=Nowhereville%2CZZ')

      expect(response?.status()).toBe(200)
      await expect(page.getByTestId('venues-city-chooser')).toBeVisible()
      await expect(
        page.locator('head > meta[name="robots"]').first()
      ).toHaveAttribute('content', /noindex/)
      // No canonical at all: a noindex beside a canonical pointing elsewhere
      // invites the noindex to be consolidated onto the target, which here
      // would be the directory root.
      await expect(page.locator('head > link[rel="canonical"]')).toHaveCount(0)
    })

    // The guard for the title and the canonical: a hand-crafted city value
    // must not reach either, whatever it contains.
    test('never echoes an unrecognised city value into the head', async ({
      page,
    }) => {
      await page.goto('/venues?cities=%3Cb%3Epwned%3C%2Fb%3E%2CZZ')

      await expect(page).toHaveTitle('Venues | Psychic Homily')
      await expect(page.locator('head')).not.toContainText('pwned')
    })
  })

  test.describe('narrow viewports', () => {
    for (const width of [320, 390]) {
      test(`does not scroll sideways at ${width}px`, async ({ page }) => {
        await page.setViewportSize({ width, height: 800 })
        await page.goto(PHOENIX)
        await tableIsUp(page)

        const { scrollWidth, clientWidth } = await documentOverflow(page)
        expect(
          scrollWidth,
          `the directory overflows a ${width}px viewport`
        ).toBeLessThanOrEqual(clientWidth)

        // The column headers are dropped at this width, so the count's unit
        // moves onto the row that carries the number and the sort control
        // becomes the chip.
        await expect(
          page.getByRole('columnheader', { name: /upcoming shows/i })
        ).toBeHidden()
        await expect(page.getByText(/\d+ upcoming/).first()).toBeVisible()
        await expect(page.getByTestId('venue-sort-chip')).toBeVisible()
      })
    }
  })
})
