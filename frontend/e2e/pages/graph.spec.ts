import { test, expect } from '../fixtures'

test.describe('Graph Observatory', () => {
  test('search → graph → context → recenter → reset', async ({ page }) => {
    await page.goto('/graph')
    await expect(page.getByRole('heading', { name: 'Follow the threads.' })).toBeVisible()

    const rootArtist = 'Playboy Manbaby'
    const search = page.getByPlaceholder('Search an artist to begin…')
    await search.fill(rootArtist)
    await page.getByRole('button', { name: rootArtist, exact: true }).click()

    await expect(page.getByText('Centered on Playboy Manbaby')).toBeVisible()
    await page.getByText('Browse connections as a list').click()

    const list = page.getByRole('list', { name: 'Artists connected to Playboy Manbaby' })
    const choices = list.getByRole('button')
    await expect(choices.first()).toBeVisible()
    const nextArtist = (await choices.first().locator('span').first().textContent())?.trim()
    expect(nextArtist).toBeTruthy()
    await choices.first().click()

    const panel = page.getByRole('region', { name: `About ${nextArtist}` })
    await expect(panel).toBeVisible()
    await expect(panel.getByRole('link', { name: /Open page/i })).toHaveAttribute(
      'href',
      /\/artists\//,
    )
    await panel.getByRole('button', { name: /Center here/i }).click()

    await expect(page.getByText(`Centered on ${nextArtist}`)).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Graph traversal history' })).toContainText(
      rootArtist,
    )

    await page.getByRole('button', { name: 'Reset' }).click()
    await expect(page.getByRole('heading', { name: 'Pick a name. See what it touches.' })).toBeVisible()
    await expect(page.getByText(/Centered on/)).toHaveCount(0)
    await expect(page.getByRole('navigation', { name: 'Graph traversal history' })).toHaveCount(0)
  })

  test('/explore hands off to the Observatory', async ({ page }) => {
    await page.goto('/explore')
    await expect(page).toHaveURL(/\/graph$/)
    await expect(page.getByRole('heading', { name: 'Follow the threads.' })).toBeVisible()
  })
})
