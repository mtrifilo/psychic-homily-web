import { test, expect } from '../fixtures'
import { workerStorageState } from '../fixtures/auth'
import { SEEDED_DETAIL_PATHS, warmRoutes } from '../helpers/navigation'

// The pointer on interactive controls comes from one base rule in
// app/globals.css, which a cursor utility on the element overrides. A unit test
// pins the rule in the compiled CSS; these read the cursor the browser computes
// from the stylesheet it was served: on one rendered control per primitive (the
// Button primitive, BracketLink's button branch, a cmdk command item, a Radix
// dropdown-menu item that is not a link, a Radix select trigger and item), and
// on probe elements for every ARIA role the rule lists.

// Mirrors the role list of the base rule (and EXPECTED_SELECTORS in
// app/pointerCursor.test.ts); a role added there is added here.
const POINTER_ROLES = [
  'button',
  'link',
  'tab',
  'menuitem',
  'menuitemcheckbox',
  'menuitemradio',
  'option',
  'radio',
  'checkbox',
  'switch',
  'combobox',
]

test.describe('Pointer cursor on interactive controls', () => {
  test.beforeAll(async ({ browser }, testInfo) => {
    await warmRoutes(browser, testInfo, ['/shows', SEEDED_DETAIL_PATHS.show])
    await warmRoutes(browser, testInfo, ['/shows/submit'], {
      storageState: workerStorageState(testInfo.workerIndex),
    })
  })

  test(
    'Button, command item and every listed ARIA role on /shows',
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

      await page.getByTestId('city-filter-combobox').click()
      const cityOption = page.getByRole('option').first()
      await expect(cityOption).toBeVisible()
      await expect(cityOption).toHaveCSS('cursor', 'pointer')
      await page.keyboard.press('Escape')

      // Bare elements carrying only a role, so no component class or native
      // UA cursor can supply the pointer; an aria-disabled twin and a text
      // input with role=combobox show what the rule leaves alone.
      const cursors = await page.evaluate(roles => {
        const host = document.createElement('div')
        document.body.append(host)
        const read = (html: string) => {
          host.innerHTML = html
          return getComputedStyle(host.firstElementChild!).cursor
        }
        const result = {
          enabled: Object.fromEntries(
            roles.map(role => [role, read(`<span role="${role}">x</span>`)])
          ),
          ariaDisabled: Object.fromEntries(
            roles.map(role => [
              role,
              read(`<span role="${role}" aria-disabled="true">x</span>`),
            ])
          ),
          comboboxInput: read('<input type="text" role="combobox">'),
        }
        host.remove()
        return result
      }, POINTER_ROLES)

      for (const role of POINTER_ROLES) {
        expect(cursors.enabled[role], `role=${role}`).toBe('pointer')
        expect(cursors.ariaDisabled[role], `aria-disabled role=${role}`).not.toBe(
          'pointer'
        )
      }
      expect(cursors.comboboxInput).toBe('text')
    }
  )

  test('BracketLink button on a show page', { tag: '@smoke' }, async ({ page }) => {
    await page.goto(SEEDED_DETAIL_PATHS.show)
    const bracket = page.getByRole('button', { name: 'Add to calendar' })
    await expect(bracket).toBeVisible({ timeout: 10_000 })
    await expect(bracket).toHaveCSS('cursor', 'pointer')
  })

  test(
    'dropdown-menu item and select item, signed in',
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
      await authenticatedPage.keyboard.press('Escape')

      // "Sign out" is a DropdownMenuItem with no asChild link, so its pointer
      // can only come from the menuitem role. Read, never clicked.
      await authenticatedPage.getByRole('button', { name: 'User menu' }).click()
      const signOut = authenticatedPage.getByRole('menuitem', { name: 'Sign out' })
      await expect(signOut).toBeVisible()
      await expect(signOut).not.toHaveAttribute('href')
      await expect(signOut).toHaveCSS('cursor', 'pointer')
      await authenticatedPage.keyboard.press('Escape')
    }
  )
})
