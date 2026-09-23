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

/**
 * The section's top settles just below the TopBar: not under it, and not
 * left further down the page. Polled, because the landing scroll is smooth.
 */
async function expectLandedBelowTopBar(page: Page, sectionId: string) {
  await expect
    .poll(async () => {
      const gap = await gapBelowTopBar(page, sectionId)
      return gap !== null && gap >= 0 && gap < 40
    })
    .toBe(true)
}

test.describe('/settings hub', () => {
  test('renders the rail and every section in order at 1440', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/settings')

    await expect(
      page.getByRole('heading', { level: 1, name: 'Settings', exact: true })
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
    ).toHaveText(SECTION_TITLES)

    await rail.getByRole('link', { name: /^Alerts and email/ }).click()
    await expect(page).toHaveURL(/\/settings#alerts$/)
    await expect(
      rail.getByRole('link', { name: /^Alerts and email/ })
    ).toHaveAttribute('aria-current', 'true')
    await expectLandedBelowTopBar(page, 'alerts')
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
      await expectLandedBelowTopBar(page, id)
    }
  })

  test('a cold-loaded fragment lands on its section', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/settings#appearance')

    const section = page.locator('main section[id="appearance"]')
    await expect(section).toBeFocused({ timeout: 10_000 })
    await expectLandedBelowTopBar(page, 'appearance')
  })

  test('the rail follows a scroll that no click started', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/settings')
    const rail = page.getByRole('navigation', { name: 'Settings sections' })
    await expect(rail).toBeVisible({ timeout: 10_000 })

    await page.evaluate(() => {
      document
        .querySelector('main section[id="appearance"]')
        ?.scrollIntoView({ block: 'start' })
    })
    await expect(
      rail.getByRole('link', { name: /^Appearance/ })
    ).toHaveAttribute('aria-current', 'true')
    await expect(rail.locator('a[aria-current="true"]')).toHaveCount(1)
  })

  test('Back from a linked page returns to the hub after a section jump', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/settings')
    const rail = page.getByRole('navigation', { name: 'Settings sections' })
    await rail.getByRole('link', { name: /^Feeds/ }).click({ timeout: 10_000 })
    await expect(page).toHaveURL(/\/settings#feeds$/)

    await page
      .getByRole('region', { name: 'Feeds' })
      .getByRole('link', { name: 'Manage in profile settings' })
      .click()
    await expect(page).toHaveURL(/\/profile\?tab=settings$/)
    await expect(
      page.getByRole('heading', { level: 1, name: /edit profile & settings/i })
    ).toBeVisible()

    await page.goBack()
    await expect(page).toHaveURL(/\/settings#feeds$/)
    await expect(
      page.getByRole('heading', { level: 1, name: 'Settings', exact: true })
    ).toBeVisible()
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
