import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  // Local workers all contend against ONE Next dev server, so each added
  // worker inflates every test's wall time until the slowest tests cross the
  // 30s timeout and fail in order of duration. Measured (PSY-1565): 5 workers
  // failed 6/6/6 with 19-29s medians; 2 workers failed 0/0/2 with 2-6s
  // medians — clean AND faster in wall clock, not a speed-for-reliability
  // trade. CI keeps 3: dedicated runners, no shared-dev-server contention.
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
  },
})
