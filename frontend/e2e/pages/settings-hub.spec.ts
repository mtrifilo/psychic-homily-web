import { test, expect } from '../fixtures'
import type { Page } from '@playwright/test'

const SECTION_TITLES = [
  'Account',
  'Public profile',
  'Home page',
  'Alerts and email',
  'Appearance',
  'Feeds',
  'Privacy and data',
]

/** Pixels between a section's top and the bottom of the sticky TopBar. */
async function gapBelowTopBar(page: Page, sectionId: string) {
  return page.evaluate(id => {
    const bar = document.querySelector('header.sticky')
    const section = document.querySelector(`main section[id="${id}"]`)
    if (!bar || !section) return null
    return (
      section.getBoundingClientRect().top - bar.getBoundingClientRect().bottom
    )
  }, sectionId)
}

test.describe('/settings hub', () => {
  test('renders the rail and every section in order at 1440', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/settings')

    await expect(
      page.getByRole('heading', { level: 1, name: 'Settings' })
    ).toBeVisible({ timeout: 10_000 })

    const rail = page.getByRole('navigation', { name: 'Settings sections' })
    await expect(rail).toBeVisible()
    await expect(rail.getByRole('link')).toHaveText(
      SECTION_TITLES.map(title => new RegExp(`^${title}`))
    )
    await expect(
      page.getByRole('navigation', { name: 'On this page' })
    ).toBeHidden()
    await expect(
      page.locator('main').getByRole('heading', { level: 2 })
    ).toHaveText(
      SECTION_TITLES
    )

    await rail.getByRole('link', { name: /^Alerts and email/ }).click()
    await expect(page).toHaveURL(/\/settings#alerts$/)
    await expect(
      rail.getByRole('link', { name: /^Alerts and email/ })
    ).toHaveAttribute('aria-current', 'true')
    const gap = await gapBelowTopBar(page, 'alerts')
    expect(gap).not.toBeNull()
    expect(gap as number).toBeGreaterThanOrEqual(0)
    expect(gap as number).toBeLessThan(40)
  })

  test('the 390 jump index lands each section clear of the TopBar', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/settings')

    const index = page.getByRole('navigation', { name: 'On this page' })
    await expect(index).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByRole('navigation', { name: 'Settings sections' })
    ).toBeHidden()

    for (const [title, id] of [
      ['Public profile', 'public-profile'],
      ['Alerts and email', 'alerts'],
      ['Privacy and data', 'privacy'],
    ] as const) {
      await page.evaluate(() => window.scrollTo(0, 0))
      await index.getByRole('link', { name: new RegExp(`^${title}`) }).click()
      await expect(page).toHaveURL(new RegExp(`/settings#${id}$`))
      await expect(page.locator(`main section[id="${id}"]`)).toBeFocused()
      const gap = await gapBelowTopBar(page, id)
      expect(gap).not.toBeNull()
      expect(gap as number).toBeGreaterThanOrEqual(0)
      expect(gap as number).toBeLessThan(40)
    }
  })

  test('a cold-loaded fragment lands on its section', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/settings#appearance')

    const section = page.locator('main section[id="appearance"]')
    await expect(section).toBeFocused({ timeout: 10_000 })
    await expect
      .poll(() => gapBelowTopBar(page, 'appearance'))
      .toBeGreaterThanOrEqual(0)
    expect((await gapBelowTopBar(page, 'appearance')) as number).toBeLessThan(
      40
    )
  })

  test('a link row lands on the control where it lives today', async ({
    authenticatedPage: page,
  }) => {
    await page.goto('/settings')

    const alerts = page.getByRole('region', { name: 'Alerts and email' })
    await alerts
      .getByRole('link', { name: 'Manage in profile settings' })
      .click()
    await expect(page).toHaveURL(/\/profile\?tab=settings#alerts$/)
    await expect(page.getByRole('tab', { name: 'Settings' })).toHaveAttribute(
      'data-state',
      'active'
    )
  })
})
