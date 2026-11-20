import { execFileSync } from 'child_process'
import { test, expect } from '../fixtures'
import { lookupWorkerUserId } from '../fixtures/test-fixtures-reset'
import { workerAuthFile } from '../fixtures/auth'
import { USER_COUNT, userEmailForWorker } from '../global-setup'
import { runE2ESql } from '../e2e-db'

// The page "Email me the steps" opens, /library/calendar. Signed out, it sends
// the reader to sign in and back. Signed in on a computer with no link,
// arriving makes none; the Add to calendar click makes exactly one, the page
// then walks Google's two-step, and Google's first check of the link
// (simulated with Google's importer User-Agent) turns its status line into the
// connected block.

const HEADING = 'Add your saved shows to Google Calendar'
const WAITING = "Waiting for Google's first check."
const DONE = "You're set. New saves appear at its next check."
const GOOGLE_ADD_BY_URL = 'https://calendar.google.com/calendar/r/settings/addbyurl'

// The shared password of the seeded users (e2e/setup-db.sh).
const SEEDED_PASSWORD = 'e2e-test-password-123'

/** No calendar link and no reset marker. */
function resetCalendarLink(userId: number) {
  runE2ESql(
    `UPDATE user_preferences SET calendar_link_reset_at = NULL, ` +
      `calendar_link_reset_reason = NULL WHERE user_id = ${userId}`
  )
  runE2ESql(`DELETE FROM calendar_tokens WHERE user_id = ${userId}`)
}

test.describe('Library calendar page', () => {
  test.describe.configure({ mode: 'serial' })

  let workerUserId: number

  test.beforeEach(async ({}, testInfo) => {
    workerUserId = await lookupWorkerUserId(workerAuthFile(testInfo.workerIndex))
    resetCalendarLink(workerUserId)
  })

  test.afterEach(async () => {
    resetCalendarLink(workerUserId)
  })

  test('signed out, it leads to sign-in and returns to the page after signing in', async ({
    page,
  }, testInfo) => {
    await page.goto('/library/calendar')
    await page.waitForURL(/\/auth\?returnTo=%2Flibrary%2Fcalendar$/, {
      timeout: 10_000,
    })

    // The password form, the sign-in path the other specs use. Google and
    // magic-link sign-in carry the destination through the backend; the
    // magic-link return has its own spec (e2e/auth/magic-link.spec.ts).
    await page
      .getByLabel('Email')
      .fill(userEmailForWorker(testInfo.workerIndex % USER_COUNT))
    await page.locator('#password').fill(SEEDED_PASSWORD)
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()

    await page.waitForURL(url => url.pathname === '/library/calendar', {
      timeout: 15_000,
    })
    await expect(
      page.getByRole('heading', { level: 1, name: HEADING })
    ).toBeVisible({ timeout: 10_000 })
  })

  test("on a computer, the three steps work in order and Google's first check shows as connected", async ({
    authenticatedPage: page,
  }) => {
    const context = page.context()
    await context.grantPermissions(['clipboard-read', 'clipboard-write'], {
      origin: 'http://localhost:3000',
    })
    // Google's page is stood in for: the test checks where step 2 goes, not
    // Google itself.
    await context.route('https://calendar.google.com/**', route =>
      route.fulfill({ status: 200, contentType: 'text/html', body: '<p>Google</p>' })
    )

    const creates: string[] = []
    page.on('request', request => {
      if (
        request.method() === 'POST' &&
        new URL(request.url()).pathname.endsWith('/calendar/token')
      ) {
        creates.push(request.url())
      }
    })

    await page.goto('/library/calendar')
    await expect(
      page.getByRole('heading', { level: 1, name: HEADING })
    ).toBeVisible({ timeout: 10_000 })

    // The account has no link: arriving makes none, and the page offers one
    // button in place of the steps.
    const add = page.getByRole('button', { name: 'Add to calendar' })
    await expect(add).toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('button', { name: 'Copy link' })).toHaveCount(0)
    expect(creates).toHaveLength(0)

    // The click makes exactly one link, and the steps appear.
    await Promise.all([
      page.waitForResponse(
        resp =>
          new URL(resp.url()).pathname.endsWith('/calendar/token') &&
          resp.request().method() === 'POST',
        { timeout: 10_000 }
      ),
      add.click(),
    ])
    const copy = page.getByRole('button', { name: 'Copy link' })
    await expect(copy).toBeVisible({ timeout: 10_000 })
    expect(creates).toHaveLength(1)

    // Step 1: Copy link copies the address.
    await copy.click()
    await expect(page.getByRole('button', { name: 'Copied' })).toBeVisible()

    const feedUrl = await page.evaluate(() => navigator.clipboard.readText())
    expect(feedUrl).toMatch(/^https?:\/\/.+\.ics$/)
    // The address is copied, never shown.
    await expect(page.locator('body')).not.toContainText(feedUrl)

    // Step 2: Google's Add from URL page, in a new tab.
    const [googleTab] = await Promise.all([
      context.waitForEvent('page'),
      page.getByRole('link', { name: 'Open Google Calendar' }).click(),
    ])
    await googleTab.waitForLoadState()
    expect(googleTab.url()).toBe(GOOGLE_ADD_BY_URL)
    await googleTab.close()

    await expect(page.getByText(WAITING)).toBeVisible({ timeout: 10_000 })

    // Step 3 happens in Google; Google then checks the link.
    execFileSync(
      'curl',
      ['-sS', '-f', '-o', '/dev/null', '-A', 'Google-Calendar-Importer', feedUrl],
      { stdio: ['ignore', 'ignore', 'pipe'] }
    )

    // The read is recorded after the response; coming back to the tab reads
    // the status again (the page also reads it on an interval).
    await expect(async () => {
      await page.evaluate(() =>
        document.dispatchEvent(new Event('visibilitychange', { bubbles: true }))
      )
      await expect(page.getByText(DONE)).toBeVisible({ timeout: 1_000 })
    }).toPass({ timeout: 20_000 })

    await expect(page.getByText('Google Calendar', { exact: true })).toBeVisible()
    await expect(page.getByText(/^Checked (just now|\d+m ago)\.$/)).toBeVisible()
    await expect(page.getByText(WAITING)).toHaveCount(0)
    expect(creates).toHaveLength(1)
  })
})
