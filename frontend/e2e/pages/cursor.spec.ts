import { test, expect } from '../fixtures'
import { workerStorageState } from '../fixtures/auth'
import { SEEDED_DETAIL_PATHS, warmRoutes } from '../helpers/navigation'

// The pointer on interactive controls comes from one base rule in
// app/globals.css, which a cursor utility on the element overrides. A unit test
// pins the rule in the compiled CSS; these read the cursor the browser actually
// computes on one control per primitive that relies on it: the Button
// primitive, BracketLink's button branch, a Radix dropdown-menu item, a cmdk
// command item and a Radix select item.

test.describe('Pointer cursor on interactive controls', () => {
  test.beforeAll(async ({ browser }, testInfo) => {
    await warmRoutes(browser, testInfo, ['/shows', SEEDED_DETAIL_PATHS.show])
    await warmRoutes(browser, testInfo, ['/shows/submit'], {
      storageState: workerStorageState(testInfo.workerIndex),
    })
  })

  test(
    'Button, dropdown-menu item and command item on /shows',
    { tag: '@smoke' },
    async ({ page }) => {
      await page.goto('/shows')
      await expect(page.locator('article').first()).toBeVisible({
        timeout: 10_000,
      })

      const saveButton = page
        .getByRole('button', { name: /^Sign in to save/ })
        .first()
      await expect(saveButton).toHaveAttribute('data-slot', 'button')
      await expect(saveButton).toHaveCSS('cursor', 'pointer')

      const primaryNav = page.getByRole('navigation', { name: 'Primary' })
      await primaryNav.getByRole('button', { name: 'Browse the catalog' }).click()
      const menuItem = page.getByRole('menuitem', { name: 'Venues' })
      await expect(menuItem).toBeVisible()
      await expect(menuItem).toHaveCSS('cursor', 'pointer')
      await page.keyboard.press('Escape')

      await page.getByTestId('city-filter-combobox').click()
      const cityOption = page.getByRole('option').first()
      await expect(cityOption).toBeVisible()
      await expect(cityOption).toHaveCSS('cursor', 'pointer')
    }
  )

  test('BracketLink button on a show page', { tag: '@smoke' }, async ({ page }) => {
    await page.goto(SEEDED_DETAIL_PATHS.show)
    const bracket = page.getByRole('button', { name: 'Add to calendar' })
    await expect(bracket).toBeVisible({ timeout: 10_000 })
    await expect(bracket).toHaveCSS('cursor', 'pointer')
  })

  test(
    'select item in the show submission form',
    { tag: '@smoke' },
    async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/shows/submit')
      const billRole = authenticatedPage.getByRole('combobox', {
        name: 'Bill role 1',
      })
      await expect(billRole).toBeVisible({ timeout: 10_000 })
      await expect(billRole).toHaveCSS('cursor', 'pointer')

      await billRole.click()
      const selectItem = authenticatedPage.getByRole('option').first()
      await expect(selectItem).toBeVisible()
      await expect(selectItem).toHaveCSS('cursor', 'pointer')
    }
  )
})
