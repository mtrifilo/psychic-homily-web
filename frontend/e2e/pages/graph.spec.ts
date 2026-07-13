import { test, expect } from '../fixtures'

test.describe('Graph Observatory', () => {
  test('search → graph → recenter → artist page', async ({ page }) => {
    await page.goto('/graph')
    await expect(page.getByRole('heading', { name: 'Follow the threads.' })).toBeVisible()

    const search = page.getByPlaceholder('Search an artist to begin…')
    await search.fill('Playboy Manbaby')
    await page.getByRole('button', { name: 'Playboy Manbaby', exact: true }).click()

    await expect(page.getByText('Centered on Playboy Manbaby')).toBeVisible()
    await page.getByText('Browse connections as a list').click()

    const list = page.getByRole('list', { name: 'Artists connected to Playboy Manbaby' })
    const choices = list.getByRole('button')
    await expect(choices.nth(1)).toBeVisible()
    const nextArtist = (await choices.nth(1).locator('span').first().textContent())?.trim()
    expect(nextArtist).toBeTruthy()
    await choices.nth(1).click()

    const panel = page.getByRole('region', { name: `About ${nextArtist}` })
    await expect(panel).toBeVisible()
    await panel.getByRole('button', { name: /Center here/i }).click()

    await expect(page.getByText(`Centered on ${nextArtist}`)).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Graph traversal history' })).toContainText(
      'Playboy Manbaby',
    )

    await page.getByText('Browse connections as a list').click()
    await page
      .getByRole('list', { name: `Artists connected to ${nextArtist}` })
      .getByRole('button')
      .first()
      .click()
    await page.getByRole('region', { name: `About ${nextArtist}` }).getByRole('link', { name: /Open page/i }).click()
    await expect(page).toHaveURL(/\/artists\//)
  })

  test('/explore hands off to the Observatory', async ({ page }) => {
    await page.goto('/explore')
    await expect(page).toHaveURL(/\/graph$/)
    await expect(page.getByRole('heading', { name: 'Follow the threads.' })).toBeVisible()
  })
})
