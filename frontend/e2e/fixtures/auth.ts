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
  // PSY-1645: both calls used to be wrapped in try/catch + console.warn, on the
  // reasoning that a cleanup hiccup shouldn't mask a real test failure. That
  // traded one loud, precise failure for several quiet, misleading ones: a
  // misconfigured backend URL made the lookup 401, cleanup was skipped
  // silently, and the next tests failed on dirty fixture state in ways that
  // read as product bugs. Neither call is a "hiccup" — the lookup failing means
  // we are not authenticated against the backend under test, and the reset
  // failing means the database is now dirty for every later test and run. Both
  // must abort the run. Playwright reports a fixture error separately from test
  // results, so a teardown throw does not overwrite the failure that preceded
  // it. There is deliberately no opt-out flag: an env var that downgrades this
  // to a warning is exactly how the silent mode would come back.
  workerCleanup: [
    async ({}, use, workerInfo) => {
      const seededIndex = workerInfo.workerIndex % USER_COUNT
      const authFile = userAuthFileForWorker(seededIndex)

      const workerUserId = await lookupWorkerUserId(authFile)

      await use()

      await resetTestFixtures(workerUserId)
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

    // PSY-1645: throws rather than warning-and-skipping, for the reasons on
    // `workerCleanup` above. This fixture exists precisely so a retry starts
    // clean, so silently not cleaning defeats its only purpose.
    const workerUserId = await lookupWorkerUserId(authFile)

    // `use` here is Playwright's fixture callback (the 2nd arg of a fixture
    // function), not React's `use` hook. The rule's name heuristic flags it
    // because this named-property fixture reads as a plain function; the
    // anonymous array-fixture above (`workerCleanup`) isn't flagged for the
    // same call. False positive — see PSY-953.
    // eslint-disable-next-line react-hooks/rules-of-hooks
    await use()

    await resetTestFixtures(workerUserId)
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
