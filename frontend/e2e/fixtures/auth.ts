import { test as base } from './error-detection'
import { type Page, type BrowserContext } from '@playwright/test'
import * as path from 'path'
import { USER_COUNT, userAuthFileForWorker } from '../global-setup'
import { resetTestFixtures, lookupWorkerUserId } from './test-fixtures-reset'

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
 * PSY-432: `workerCleanup` is a worker-scoped fixture whose teardown
 * calls the admin-only `/admin/test-fixtures/reset` endpoint for this
 * worker's seeded user. It fires automatically when the worker shuts
 * down — even after a test crash — so mid-test failures don't poison
 * later runs. Depending `authenticatedPage` on this fixture wires it
 * into every mutating test automatically, without `afterEach` boilerplate.
 *
 * PSY-507: `cleanBetweenRetries` is a test-scoped opt-in fixture whose
 * teardown fires the same reset between each attempt, including between
 * Playwright retries. Worker-scoped `workerCleanup` does not cover this
 * case — it only runs at worker teardown, so a test that fails partway
 * through on retry N finds leftover state from retry N-1 still in the
 * DB. See docs/runbooks/e2e-testing.md for when to opt in.
 *
 * `adminPage` remains a single shared admin — admin tests are rare and
 * low-race-risk.
 */
export const test = base.extend<
  { authenticatedPage: Page; adminPage: Page; cleanBetweenRetries: void },
  { workerCleanup: void }
>({
  // Worker-scoped (note the `{ scope: 'worker' }` option on the tuple):
  // the setup looks up the worker user's numeric ID once, then the
  // teardown calls the reset endpoint when Playwright shuts the worker
  // down. Runs whether the test passed or failed.
  //
  // If the lookup or reset fails we log and continue — we don't want a
  // cleanup hiccup to mask a real test failure.
  workerCleanup: [
    async ({}, use, workerInfo) => {
      const seededIndex = workerInfo.workerIndex % USER_COUNT
      const authFile = userAuthFileForWorker(seededIndex)

      let workerUserId: number | null = null
      try {
        workerUserId = await lookupWorkerUserId(authFile)
      } catch (err) {
        console.warn(
          `[PSY-432] worker ${workerInfo.workerIndex}: profile lookup failed; skipping cleanup (${(err as Error).message})`,
        )
      }

      await use()

      if (workerUserId !== null) {
        try {
          await resetTestFixtures(workerUserId)
        } catch (err) {
          console.warn(
            `[PSY-432] worker ${workerInfo.workerIndex}: reset failed (user_id=${workerUserId}): ${(err as Error).message}`,
          )
        }
      }
    },
    { scope: 'worker', auto: true },
  ],

  authenticatedPage: async (
    { browser, errors: _errors, workerCleanup: _cleanup },
    runFixture,
    testInfo,
  ) => {
    const seededIndex = testInfo.workerIndex % USER_COUNT
    const authFile = userAuthFileForWorker(seededIndex)
    const context: BrowserContext = await browser.newContext({
      storageState: path.join(AUTH_DIR, authFile),
    })
    const page = await context.newPage()
    await runFixture(page)
    await context.close()
  },

  // PSY-507: test-scoped cleanup. Opt-in via destructuring in the test
  // signature (`async ({ authenticatedPage, cleanBetweenRetries: _ }) => …`).
  // Fires `/admin/test-fixtures/reset` at test teardown, so retries of a
  // failing mutating test start from a clean slate instead of compounding
  // state across attempts. Reuses the PSY-432 allowlist scopes verbatim.
  cleanBetweenRetries: async ({}, use, testInfo) => {
    const seededIndex = testInfo.workerIndex % USER_COUNT
    const authFile = userAuthFileForWorker(seededIndex)

    let workerUserId: number | null = null
    try {
      workerUserId = await lookupWorkerUserId(authFile)
    } catch (err) {
      console.warn(
        `[PSY-507] cleanBetweenRetries: profile lookup failed; skipping cleanup (${(err as Error).message})`,
      )
    }

    await use()

    if (workerUserId !== null) {
      try {
        await resetTestFixtures(workerUserId)
      } catch (err) {
        console.warn(
          `[PSY-507] cleanBetweenRetries: reset failed (user_id=${workerUserId}): ${(err as Error).message}`,
        )
      }
    }
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
