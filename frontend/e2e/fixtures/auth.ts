import { test as base } from './error-detection'
import { type Page, type BrowserContext } from '@playwright/test'
import * as path from 'path'

const AUTH_DIR = path.resolve(__dirname, '../.auth')

/**
 * Auth fixtures that provide pre-authenticated pages using storageState
 * captured during global setup.
 */
export const test = base.extend<{
  authenticatedPage: Page
  adminPage: Page
}>({
  authenticatedPage: async ({ browser, errors: _errors }, use) => {
    const context: BrowserContext = await browser.newContext({
      storageState: path.join(AUTH_DIR, 'user.json'),
    })
    const page = await context.newPage()
    await use(page)
    await context.close()
  },

  adminPage: async ({ browser, errors: _errors }, use) => {
    const context: BrowserContext = await browser.newContext({
      storageState: path.join(AUTH_DIR, 'admin.json'),
    })
    const page = await context.newPage()
    await use(page)
    await context.close()
  },
})

export { expect } from '@playwright/test'
