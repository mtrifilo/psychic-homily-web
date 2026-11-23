import { test, expect } from '../fixtures'
import { runE2ESql } from '../e2e-db'

/**
 * Old entity URLs. A venue merged away through the admin merge, and a venue
 * whose slug a tool rewrote, each answer ONE permanent redirect to the venue's
 * current URL with the query kept, and the target renders. A venue renamed
 * through the admin edit keeps its slug, so its URL keeps answering 200.
 *
 * The rooms are this spec's own, created and removed here, and no request
 * reaches a URL before the change that makes it old: the proxy remembers a
 * live verdict for a few minutes, and a remembered one would wave the old URL
 * through instead of redirecting it.
 */

const KEPT = 'e2e-former-slugs-kept-room'
const MERGED = 'e2e-former-slugs-merged-room'
const RENAMED_FROM = 'e2e-former-slugs-renamed-from'
const RENAMED_TO = 'e2e-former-slugs-renamed-to'
const EDITED = 'e2e-former-slugs-edited-room'
const SLUGS = [KEPT, MERGED, RENAMED_FROM, RENAMED_TO, EDITED]

function quoted(values: string[]): string {
  return values.map(value => `'${value}'`).join(', ')
}

function removeRooms(): void {
  runE2ESql(`DELETE FROM entity_former_slugs WHERE entity_type = 'venue' AND former_slug IN (${quoted(SLUGS)})`)
  runE2ESql(`DELETE FROM venues WHERE slug IN (${quoted(SLUGS)})`)
}

function createRoom(slug: string): number {
  return Number(
    runE2ESql(
      `WITH inserted AS (
         INSERT INTO venues (name, address, city, state, zipcode, verified, created_at, updated_at, slug)
         VALUES ('E2E [${slug}]', '1 Former Way', 'Phoenix', 'AZ', '85001', true, NOW(), NOW(), '${slug}')
         RETURNING id
       ) SELECT id FROM inserted`
    )
  )
}

function locationOf(response: { headers(): Record<string, string> }): string {
  const location = new URL(response.headers()['location'], 'http://origin')
  return `${location.pathname}${location.search}`
}

test.describe('Former venue slugs', () => {
  test.describe.configure({ mode: 'serial' })

  test.beforeEach(() => {
    removeRooms()
  })

  test.afterAll(() => {
    removeRooms()
  })

  test('a venue merged away redirects to the room that survives it', async ({ adminPage, page, request }) => {
    const kept = createRoom(KEPT)
    const merged = createRoom(MERGED)

    const merge = await adminPage.request.post('/api/admin/venues/merge', {
      data: { canonical_venue_id: kept, merge_from_venue_id: merged },
    })
    expect(merge.status(), await merge.text()).toBe(200)

    const redirect = await request.get(`/venues/${MERGED}?utm_source=e2e`, { maxRedirects: 0 })
    expect(redirect.status(), 'the old URL must answer the permanent redirect itself').toBe(308)
    expect(locationOf(redirect)).toBe(`/venues/${KEPT}?utm_source=e2e`)

    const response = await page.goto(`/venues/${MERGED}`)
    expect(response?.status()).toBe(200)
    expect(new URL(response!.url()).pathname).toBe(`/venues/${KEPT}`)
    // One hop: the request the page answered came from the old URL, which was
    // not itself redirected.
    expect(response!.request().redirectedFrom()?.redirectedFrom() ?? null).toBeNull()
    await expect(page.getByRole('heading', { level: 1, name: `E2E [${KEPT}]` })).toBeVisible({
      timeout: 10_000,
    })
  })

  test('a venue whose slug changed redirects to its new URL', async ({ page, request }) => {
    const id = createRoom(RENAMED_FROM)
    runE2ESql(`UPDATE venues SET slug = '${RENAMED_TO}' WHERE id = ${id}`)

    const redirect = await request.get(`/venues/${RENAMED_FROM}`, { maxRedirects: 0 })
    expect(redirect.status()).toBe(308)
    expect(locationOf(redirect)).toBe(`/venues/${RENAMED_TO}`)

    const response = await page.goto(`/venues/${RENAMED_FROM}`)
    expect(response?.status()).toBe(200)
    expect(new URL(response!.url()).pathname).toBe(`/venues/${RENAMED_TO}`)
  })

  test('a venue renamed through the admin edit keeps its URL', async ({ adminPage, request }) => {
    const id = createRoom(EDITED)

    const rename = await adminPage.request.put(`/api/venues/${id}`, {
      data: { name: 'E2E [former-slugs renamed room]' },
    })
    expect(rename.status(), await rename.text()).toBe(200)

    const response = await request.get(`/venues/${EDITED}`, { maxRedirects: 0 })
    expect(response.status()).toBe(200)
  })

  test('a venue slug that never existed is still a 404', async ({ request }) => {
    const response = await request.get('/venues/e2e-former-slugs-never-existed', { maxRedirects: 0 })
    expect(response.status()).toBe(404)
  })
})

/**
 * An artist renamed through the admin edit gets a slug derived from its new
 * name, and its old URL answers one permanent redirect to the new one.
 */
test.describe('Former artist slugs', () => {
  const OLD_SLUG = 'e2e-former-slugs-artist-before'
  const NEW_NAME = 'E2E Former Slugs Artist After'
  const NEW_SLUG = 'e2e-former-slugs-artist-after'

  function removeArtists(): void {
    runE2ESql(
      `DELETE FROM entity_former_slugs WHERE entity_type = 'artist' AND former_slug IN ('${OLD_SLUG}', '${NEW_SLUG}')`
    )
    runE2ESql(`DELETE FROM artists WHERE slug IN ('${OLD_SLUG}', '${NEW_SLUG}')`)
  }

  test.beforeEach(() => removeArtists())
  test.afterAll(() => removeArtists())

  test('an artist renamed through the admin edit redirects to its new URL', async ({
    adminPage,
    page,
    request,
  }) => {
    const id = Number(
      runE2ESql(
        `WITH inserted AS (
           INSERT INTO artists (name, slug, created_at, updated_at)
           VALUES ('E2E Former Slugs Artist Before', '${OLD_SLUG}', NOW(), NOW())
           RETURNING id
         ) SELECT id FROM inserted`
      )
    )

    const rename = await adminPage.request.patch(`/api/admin/artists/${id}`, {
      data: { name: NEW_NAME },
    })
    expect(rename.status(), await rename.text()).toBe(200)

    const redirect = await request.get(`/artists/${OLD_SLUG}?utm_source=e2e`, { maxRedirects: 0 })
    expect(redirect.status()).toBe(308)
    expect(locationOf(redirect)).toBe(`/artists/${NEW_SLUG}?utm_source=e2e`)

    const response = await page.goto(`/artists/${OLD_SLUG}`)
    expect(response?.status()).toBe(200)
    expect(new URL(response!.url()).pathname).toBe(`/artists/${NEW_SLUG}`)
  })
})
