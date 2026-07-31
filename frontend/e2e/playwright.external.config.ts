import { defineConfig, devices } from '@playwright/test'

/**
 * Run the E2E suite against a stack that is ALREADY RUNNING.
 *
 * PSY-1645 made `BACKEND_URL` decide which backend the suite addresses, but in
 * `playwright.config.ts` that means *"provision the harness's own backend at
 * this port"*: `global-setup.ts` starts postgres, seeds it, starts a backend,
 * captures auth state, and refuses when the port is already in use. That is
 * right for its model, and it is why it cannot serve the other case — a stack
 * someone else already brought up, which the run should *use* rather than
 * duplicate.
 *
 * This config is that second mode: no `globalSetup`, no `globalTeardown`, no
 * `webServer`. It owns nothing.
 *
 * ## What it requires of the stack
 *
 * Because nothing is provisioned here, the stack must already satisfy
 * everything `global-setup.ts` normally arranges:
 *
 * 1. **Seeded database** — `frontend/e2e/setup-db.sh` against its `DATABASE_URL`.
 * 2. **Test-fixture endpoints** — the backend needs `ENABLE_TEST_FIXTURES=1`
 *    and `ENVIRONMENT=test` (see `backend/internal/api/routes/test_fixtures.go`),
 *    or every worker teardown 404s on `/admin/test-fixtures/reset`.
 *    `DISABLE_AUTH_RATE_LIMITS=1` too, or parallel logins trip the limiter.
 * 3. **`e2e/.auth/*.json` captured against THAT backend**, which means it must
 *    also sign with the same `JWT_SECRET_KEY` those cookies were minted under.
 *    A mismatch is not a soft failure: `lookupWorkerUserId` runs in worker
 *    fixture *setup*, so every worker aborts with `HTTP 401` before the first
 *    test — and the error blames `BACKEND_URL`, which is the one thing that
 *    was right.
 *
 * ## Known NOT to work: `scripts/dispatch/stack-up.sh`
 *
 * Stated plainly because the natural assumption is that it does. As of PSY-1659
 * a dispatch stack cannot satisfy (2) or (3):
 *
 * - `stack-up.sh` pins `JWT_SECRET_KEY=dispatch-jwt-secret-key-for-testing-only`
 *   while `global-setup.ts` captures `e2e/.auth` under
 *   `e2e-jwt-secret-key-for-testing-only`, so harness-captured auth state is
 *   rejected — and nothing in `stack-up.sh` captures its own.
 * - It does not set `ENABLE_OAUTH_TEST_PROVIDER=1`, which `global-setup.ts`
 *   does. That is a candidate cause of PSY-1655 (`oauth-google` failing on a
 *   non-default topology) — noted as a lead, not a diagnosis; that ticket owns it.
 *
 * Closing those is PSY-1662. Until then this config is for a stack
 * you configured yourself — which is how it was verified.
 *
 * ## Usage (the verified path)
 *
 * Bring up postgres/backend/frontend yourself with the same env `global-setup`
 * uses — critically the SAME `JWT_SECRET_KEY` — seed the DB, capture auth
 * state against the running frontend, then:
 *
 *     BACKEND_URL=http://localhost:8095 \
 *     E2E_BASE_URL=http://localhost:3095 \
 *       bun run test:e2e:external
 *
 * Both variables are REQUIRED and are validated below. There is deliberately no
 * default: falling back to `localhost:3000`/`:8080` would silently drive a
 * developer's real dev stack and point a row-deleting fixture reset at their
 * database. `global-setup`'s port-in-use check is what normally makes that
 * impossible, and this config removes it.
 */
function required(name: string, value: string | undefined): string {
  if (!value) {
    throw new Error(
      `${name} is required by e2e/playwright.external.config.ts. This config ` +
        `never provisions a stack — it only talks to one that is already ` +
        `running, so it cannot guess where that is. Set BACKEND_URL and ` +
        `E2E_BASE_URL (or STACK_FRONTEND_URL) to the SAME stack. They are ` +
        `separate variables with no cross-check: pointing them at different ` +
        `stacks runs the browser against one and deletes rows in the other.`
    )
  }
  return value
}

const baseURL = required(
  'E2E_BASE_URL (or STACK_FRONTEND_URL)',
  process.env.E2E_BASE_URL ?? process.env.STACK_FRONTEND_URL
)
const backendURL = required('BACKEND_URL', process.env.BACKEND_URL)

// Print the resolved pair. The failure this guards against is a run that goes
// green against the wrong stack, which is invisible unless the targets are
// stated up front.
console.log(`[e2e:external] frontend=${baseURL} backend=${backendURL}`)

export default defineConfig({
  testDir: '.',
  // Unit tests colocated beside fixtures belong to vitest, not Playwright.
  testIgnore: '**/*.test.ts',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  // Explicit rather than inherited-by-default: `trace: 'on-first-retry'` below
  // is dead config at 0, and an external stack is exactly where a trace is
  // most useful (no HTML report, often headless on someone else's machine).
  retries: Number(process.env.E2E_RETRIES ?? 1),
  // Matches the local default in `playwright.config.ts`. An externally-managed
  // stack is usually a dev server, which is the contention this bound exists
  // for (PSY-1565); raise it with E2E_WORKERS when the stack can take it.
  workers: Number(process.env.E2E_WORKERS ?? 2),
  reporter: 'list',

  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },

  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
