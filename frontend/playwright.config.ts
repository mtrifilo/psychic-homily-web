import { defineConfig, devices } from '@playwright/test'
import { BACKEND_BASE_URL } from './e2e/backend-url'

export default defineConfig({
  testDir: './e2e',
  // Every browser test here is a `.spec.ts`; `.test.ts` is Vitest's extension
  // repo-wide. Playwright's default match would claim both, so a unit test
  // sitting next to the fixture it covers would be collected by each runner
  // and fail under this one.
  testIgnore: '**/*.test.ts',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  // Local workers all contend against ONE Next dev server, so each added
  // worker inflates every test's wall time until the slowest tests cross the
  // 30s timeout and fail in order of duration. Measured across repeat runs at
  // identical load: 5 workers failed 6/6/6 with 19-29s medians; 2 workers
  // failed 0/0/2 with 2-6s medians. Two is also faster end-to-end (full suite
  // 92s vs 241s), so this is not a speed-for-reliability trade. Raising this
  // number is the thing that reintroduces the "random" slow-test flakiness.
  // CI keeps 3: dedicated runners, no shared-dev-server contention.
  // The 5 seeded users (USER_COUNT in e2e/global-setup.ts) are a ceiling, not
  // a target; using only 2 of them is fine.
  workers: process.env.CI ? 3 : 2,
  // Blob reporter in CI so sharded runs can be merged via `playwright
  // merge-reports` (see `.github/workflows/ci.yml`, PSY-418). HTML locally
  // for dev ergonomics.
  reporter: process.env.CI ? 'blob' : 'html',

  globalSetup: './e2e/global-setup.ts',
  globalTeardown: './e2e/global-teardown.ts',

  use: {
    baseURL: 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],

  webServer: {
    command: 'bun run dev',
    url: 'http://localhost:3000',
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
    // PSY-1649: the OAuth button leaves the app entirely — a full-page
    // redirect to the backend's /auth/login/google, which does not survive
    // the same-origin /api proxy the data path deliberately uses. So it reads
    // its own variable, and the harness has to point that variable at the
    // backend `global-setup.ts` actually started. Without this, moving the
    // backend off :8080 with BACKEND_URL fixed the data specs (they go
    // through the proxy, which follows BACKEND_URL) while oauth-google.spec
    // kept redirecting to a stale :8080 — no single value of
    // NEXT_PUBLIC_API_URL satisfied both (measured in PSY-1645).
    //
    // Merged over process.env by Playwright, and BACKEND_BASE_URL is
    // http://localhost:8080 when BACKEND_URL is unset, so a default run is
    // unchanged. Note `reuseExistingServer` above: locally this only takes
    // effect when Playwright is the one starting the dev server.
    env: {
      NEXT_PUBLIC_OAUTH_BACKEND_URL: BACKEND_BASE_URL,
    },
  },
})
