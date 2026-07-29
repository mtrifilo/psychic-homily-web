import { defineConfig, devices } from '@playwright/test'

/**
 * Standalone config for the pre-hydration replay proof harness.
 *
 * Deliberately does NOT reuse the main config: no `globalSetup` (which pins the
 * backend to :8080 and would collide with a running dev stack), and no
 * `webServer` (the harness needs a production `next build` + `next start`, not
 * the dev server, because dev-server timings are not representative).
 *
 * Bring the stack up by hand — see the header of `prehydration-replay.spec.ts`.
 */
export default defineConfig({
  testDir: '.',
  // Trials mutate one shared row set (`user_bookmarks`) and share one backend,
  // so they must not overlap.
  workers: 1,
  fullyParallel: false,
  // Each trial deliberately waits out a multi-second hydration window under
  // heavy throttling.
  timeout: 120_000,
  reporter: 'list',
  use: {
    baseURL: process.env.HYDRATION_BASE_URL ?? 'http://localhost:3099',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
