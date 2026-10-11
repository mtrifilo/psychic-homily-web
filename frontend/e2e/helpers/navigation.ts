import { existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import type { Browser, TestInfo } from '@playwright/test'

/**
 * Load budget for one route while warming: about twice the slowest cold
 * route load seen on a loaded CI shard (13.9 s).
 */
const WARM_ROUTE_TIMEOUT_MS = 30_000

/**
 * The routes already warmed in a run: one empty marker file per route under
 * `dir`, written only after the route loaded. Every worker process of the run
 * reads the same directory, including a worker Playwright starts after a
 * failure.
 */
export function warmLedger(dir: string) {
  const markerFor = (path: string) => join(dir, encodeURIComponent(path))
  return {
    isWarm: (path: string) => existsSync(markerFor(path)),
    markWarm: (path: string) => {
      mkdirSync(dir, { recursive: true })
      writeFileSync(markerFor(path), '')
    },
  }
}

/**
 * The run's ledger lives in the project output dir, which Playwright clears
 * at the start of every run, so a new run warms again.
 */
const ledgerFor = (testInfo: TestInfo) =>
  warmLedger(join(testInfo.project.outputDir, '.warmed-routes'))

/**
 * Loads each path once in a throwaway page, so the dev server has compiled
 * the route before a test reaches it. Call it from `test.beforeAll` with the
 * routes a spec visits, so the compile runs under the hook's own timeout
 * instead of inside a navigation wait or the test's budget.
 *
 * The dev server compiles a route on its first request, and an App Router
 * client navigation commits its URL only after the destination's RSC payload
 * and client chunks arrive, so an unwarmed route's compile lands inside
 * whatever waits on the URL. The client chunks compile when a browser first
 * requests them, which is why this loads a page rather than fetching the
 * HTML. A dynamic route compiles once for every slug, so any seeded slug
 * warms it.
 *
 * A run loads each path at most once across all its workers (warmLedger):
 * the compile is shared by the whole server, and every extra page load is
 * contention for the tests running beside it.
 *
 * Pass `storageState` to load the pages as the signed-in viewer the test
 * uses. It is required for a page that sends a signed-out viewer to /auth,
 * because that navigation can abort the next path's load.
 */
export async function warmRoutes(
  browser: Browser,
  testInfo: TestInfo,
  paths: readonly string[],
  { storageState }: { storageState?: string } = {}
): Promise<void> {
  const ledger = ledgerFor(testInfo)
  const cold = paths.filter(path => !ledger.isWarm(path))
  if (cold.length === 0) return
  testInfo.setTimeout(testInfo.timeout + cold.length * WARM_ROUTE_TIMEOUT_MS)
  const context = await browser.newContext({
    baseURL: testInfo.project.use.baseURL,
    storageState,
  })
  try {
    const page = await context.newPage()
    for (const path of cold) {
      const response = await page.goto(path, {
        timeout: WARM_ROUTE_TIMEOUT_MS,
      })
      if (!response?.ok()) {
        throw new Error(
          `warming ${path} returned HTTP ${response?.status() ?? 'no response'}`
        )
      }
      ledger.markWarm(path)
    }
  } finally {
    await context.close()
  }
}

/**
 * Budget for a client navigation to a warmed route to commit its URL. The
 * commit waits for the RSC payload and the destination's client chunks; this
 * is about three times the slowest such commit seen on a loaded CI shard
 * (4.7 s, against Playwright's 5 s expect default).
 */
export const CLIENT_NAVIGATION_TIMEOUT_MS = 15_000

/** One seeded detail page per dynamic route the specs warm (e2e/setup-db.sh). */
export const SEEDED_DETAIL_PATHS = {
  show: '/shows/e2e-attendance-test',
  artist: '/artists/calexico',
  venue: '/venues/e2e-reserved-venue',
} as const
