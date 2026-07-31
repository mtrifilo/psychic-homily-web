import { defineConfig, devices } from '@playwright/test'

/**
 * Run the E2E suite against a stack this harness does NOT own.
 *
 * `playwright.config.ts` provisions everything itself — postgres, a backend on
 * :8080, a dev server on :3000 — and `global-setup.ts` refuses to start if
 * `BACKEND_URL` points anywhere else, because the seeded database, the
 * test-fixture flags and the captured auth cookies all belong to the stack it
 * built. That guard is correct, and it is also what makes `BACKEND_URL`
 * unusable there: this config is the other half of that story.
 *
 * Without it, `BACKEND_URL` (see `e2e/backend-url.ts`) has no supported caller —
 * every value except the default is rejected by the guard, and the validation
 * in that module can never fire. That is the configuration a dispatch worktree
 * needs, so it ships here rather than being rebuilt ad hoc each time (it was,
 * twice, before this existed).
 *
 * ## Usage
 *
 *     bash scripts/dispatch/stack-up.sh "$(git rev-parse --show-toplevel)" --mode=isolated
 *     set -a; source "$(git rev-parse --show-toplevel)/dispatch-stack/.env"; set +a
 *     cd frontend && bun run test:e2e:external
 *
 * `stack-up.sh` writes `STACK_BACKEND_URL` / `STACK_FRONTEND_URL`; the two
 * fallbacks below let a hand-rolled stack work by exporting `BACKEND_URL` and
 * `E2E_BASE_URL` directly.
 *
 * ## What you give up
 *
 * No `globalSetup`, so nothing seeds the database or captures auth state. The
 * stack must already be seeded (`frontend/e2e/setup-db.sh` against its
 * `DATABASE_URL`) and `e2e/.auth/*.json` must already exist for that stack —
 * storage state captured against a different backend will authenticate as a
 * user that does not exist there. Specs whose flow depends on the harness's
 * exact topology may not pass here; `oauth-google.spec.ts` is known not to
 * (PSY-1655).
 *
 * No `webServer`, so the frontend must already be running and pointed at the
 * same backend — otherwise the browser talks to one stack and the
 * direct-to-backend fixtures talk to another, which is precisely the split this
 * whole change exists to prevent.
 */
export default defineConfig({
  testDir: '.',
  // Unit tests colocated beside fixtures are vitest's, not Playwright's.
  testIgnore: '**/*.test.ts',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  // Deliberately 2, matching the local default in `playwright.config.ts`: an
  // externally-managed stack is usually a dev server, which is exactly the
  // contention this number exists to bound (PSY-1565).
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
