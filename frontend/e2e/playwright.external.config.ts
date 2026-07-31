import { defineConfig, devices } from '@playwright/test'

/**
 * Run the E2E suite against a stack that is ALREADY RUNNING.
 *
 * PSY-1645 made `BACKEND_URL` decide which backend the suite addresses, but in
 * `playwright.config.ts` that means *"provision the harness's own backend at
 * this port"* — `global-setup.ts` starts postgres, seeds it, starts a backend
 * and captures auth state, and it refuses outright when the port is already in
 * use. That is the right behaviour for its model, and it is precisely why it
 * cannot serve the other case:
 *
 *     scripts/dispatch/stack-up.sh --mode=isolated
 *
 * has already started postgres, a backend and a frontend on free ports. There
 * the run should *use* that stack, not build a second one beside it. Under the
 * default config global-setup would reject the port and, if it got past that,
 * re-seed a database the stack already owns.
 *
 * This config is that second mode: no `globalSetup`, no `globalTeardown`, no
 * `webServer`. It owns nothing and assumes the stack is up.
 *
 * ## Usage
 *
 *     bash scripts/dispatch/stack-up.sh "$(git rev-parse --show-toplevel)" --mode=isolated
 *     set -a; source "$(git rev-parse --show-toplevel)/dispatch-stack/.env"; set +a
 *     cd frontend && bun run test:e2e:external
 *
 * `stack-up.sh` exports `BACKEND_URL` (which `e2e/backend-url.ts` reads for the
 * direct-to-backend calls) and `STACK_FRONTEND_URL` (which becomes `baseURL`
 * below). A hand-rolled stack works by exporting `BACKEND_URL` and
 * `E2E_BASE_URL` directly.
 *
 * ## What you take on by using it
 *
 * Nothing seeds the database and nothing captures auth state, because there is
 * no global-setup to do it. Both are now your job:
 *
 * - The stack's database must be seeded — `frontend/e2e/setup-db.sh` against
 *   its `DATABASE_URL`.
 * - `e2e/.auth/*.json` must correspond to THAT backend. Storage state captured
 *   against a different one authenticates as a user that does not exist there,
 *   which surfaces as a 401 rather than as the mismatch it is.
 *
 * `oauth-google.spec.ts` is known not to pass under this topology — its faux
 * provider redirect never fires when the stack is not on the default ports
 * (PSY-1655). That is pre-existing and unrelated to this config.
 *
 * The frontend must already point at the same backend. If it does not, the
 * browser talks to one stack while the direct-to-backend fixtures talk to
 * another — exactly the split PSY-1645 exists to prevent.
 */
export default defineConfig({
  testDir: '.',
  // Unit tests colocated beside fixtures belong to vitest, not Playwright.
  testIgnore: '**/*.test.ts',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  // Matches the local default in `playwright.config.ts`. An externally-managed
  // stack is usually a dev server, which is the contention this bound exists
  // for (PSY-1565); override with E2E_WORKERS when the stack can take it.
  workers: Number(process.env.E2E_WORKERS ?? 2),
  reporter: 'list',

  use: {
    baseURL:
      process.env.E2E_BASE_URL ??
      process.env.STACK_FRONTEND_URL ??
      'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },

  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
