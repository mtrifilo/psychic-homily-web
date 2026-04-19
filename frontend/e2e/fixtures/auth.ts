import { test as base } from './error-detection'
import { type Page, type BrowserContext } from '@playwright/test'
import * as path from 'path'
import { USER_COUNT, userAuthFileForWorker } from '../global-setup'

const AUTH_DIR = path.resolve(__dirname, '../.auth')

/**
 * Auth fixtures that provide pre-authenticated pages using storageState
 * captured during global setup.
 *
 * PSY-431: `authenticatedPage` is per-worker — each worker gets its own
 * seeded user so parallel mutating tests don't race on shared state.
 * Worker 0 uses the legacy `user.json` / `e2e-user@test.local`; workers
 * 1-4 get `user-N.json` / `e2e-user-N@test.local`.
 *
 * PSY-462: Playwright retries can spawn workers whose `workerIndex`
 * exceeds the seeded pool (retry #2 on a 5-worker run yielded
 * workerIndex=5). Modulo the index over USER_COUNT so retry workers
 * fall back to an already-seeded auth file instead of ENOENT-ing on
 * `user-5.json`. This is race-free in practice: the original worker's
 * test has already finished by the time Playwright spins up a retry
 * worker, so no two live workers share a user at the same instant.
 *
 * `adminPage` remains a single shared admin — admin tests are rare and
 * low-race-risk.
 */
export const test = base.extend<{
  authenticatedPage: Page
  adminPage: Page
}>({
  authenticatedPage: async ({ browser, errors: _errors }, runFixture, testInfo) => {
    const seededIndex = testInfo.workerIndex % USER_COUNT
    const authFile = userAuthFileForWorker(seededIndex)
    const context: BrowserContext = await browser.newContext({
      storageState: path.join(AUTH_DIR, authFile),
    })
    const page = await context.newPage()
    await runFixture(page)
    await context.close()
  },

  adminPage: async ({ browser, errors: _errors }, runFixture) => {
    const context: BrowserContext = await browser.newContext({
      storageState: path.join(AUTH_DIR, 'admin.json'),
    })
    const page = await context.newPage()
    await runFixture(page)
    await context.close()
  },
})

export { expect } from '@playwright/test'
