import { execFileSync } from 'child_process'
import { test, expect } from '../fixtures'
import {
  resetTestFixtures,
  lookupWorkerUserId,
} from '../fixtures/test-fixtures-reset'
import { workerAuthFile } from '../fixtures/auth'
import { runE2ESql } from '../e2e-db'

// The saved-shows calendar panel, opened from the first-save hint: Copy link
// creates and copies the private link, and a calendar app's check of that
// link (simulated with Google's importer User-Agent) turns the panel's status
// to connected when the reader comes back to the tab.

const SHOW_SLUG = 'e2e-calendar-panel-test'
const SHOW_TITLE = 'E2E [calendar-panel-test]'

/**
 * Puts the worker's user back to "never saved, never hinted, no calendar
 * link": the only state in which a save opens the hint. The link's checks
 * cascade with it.
 */
async function resetCalendarState(userId: number) {
  await resetTestFixtures(userId, ['user_bookmarks'])
  runE2ESql(
    `UPDATE user_preferences SET first_save_hint_dismissed_at = NULL, ` +
      `calendar_link_reset_at = NULL, calendar_link_reset_reason = NULL ` +
      `WHERE user_id = ${userId}`
  )
  runE2ESql(`DELETE FROM calendar_tokens WHERE user_id = ${userId}`)
}

test.describe('Saved-shows calendar panel', () => {
  test.describe.configure({ mode: 'serial' })

  let workerUserId: number

  test.beforeEach(async ({}, testInfo) => {
    workerUserId = await lookupWorkerUserId(workerAuthFile(testInfo.workerIndex))
    await resetCalendarState(workerUserId)
  })

  test.afterEach(async () => {
    await resetCalendarState(workerUserId)
  })

  test('opens from the first-save hint, copies the link, and shows a calendar check as connected', async ({
    authenticatedPage: page,
  }) => {
    await page
      .context()
      .grantPermissions(['clipboard-read', 'clipboard-write'], {
        origin: 'http://localhost:3000',
      })
    await page.goto(`/shows/${SHOW_SLUG}`)
    await expect(
      page.getByRole('navigation', { name: 'Breadcrumb' }).getByText(SHOW_TITLE)
    ).toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('button', { name: 'User menu' })).toBeVisible({
      timeout: 10_000,
    })

    const saveButton = page.getByRole('button', { name: 'Save show', exact: true })
    await Promise.all([
      page.waitForResponse(
        resp =>
          /\/saved-shows\/\d+$/.test(resp.url()) &&
          resp.request().method() === 'POST',
        { timeout: 10_000 }
      ),
      saveButton.click(),
    ])

    const hint = page.getByTestId('first-save-hint')
    await expect(hint).toBeVisible({ timeout: 10_000 })
    await hint
      .getByRole('button', { name: 'Add saved shows to your calendar' })
      .click()
    await expect(hint).toBeHidden()

    const panel = page.getByTestId('saved-shows-calendar-panel')
    await expect(
      panel.getByRole('heading', { name: 'Add your saved shows to your calendar' })
    ).toBeVisible({ timeout: 10_000 })

    // No link exists yet: Copy link creates it and copies it in one click.
    await Promise.all([
      page.waitForResponse(
        resp =>
          resp.url().endsWith('/calendar/token') &&
          resp.request().method() === 'POST',
        { timeout: 10_000 }
      ),
      panel.getByRole('button', { name: 'Copy link' }).click(),
    ])
    await expect(
      panel.getByText(
        'Copied. Paste it where your calendar app adds a calendar from a link.'
      )
    ).toBeVisible()

    const feedUrl = await page.evaluate(() => navigator.clipboard.readText())
    expect(feedUrl).toMatch(/^https?:\/\/.+/)
    // The address is copied, never shown.
    await expect(panel).not.toContainText(feedUrl)

    // A calendar app checks the link.
    execFileSync(
      'curl',
      ['-sS', '-f', '-o', '/dev/null', '-A', 'Google-Calendar-Importer', feedUrl],
      { stdio: ['ignore', 'ignore', 'pipe'] }
    )

    // The read is recorded after the response; coming back to the tab reads
    // the status again until it shows.
    const checkedLine = panel.getByText(
      /^Checked (just now|\d+m ago), since your last save\.$/
    )
    await expect(async () => {
      await page.evaluate(() =>
        document.dispatchEvent(new Event('visibilitychange', { bubbles: true }))
      )
      await expect(checkedLine).toBeVisible({ timeout: 1_000 })
    }).toPass({ timeout: 15_000 })

    await expect(
      panel.getByRole('heading', { name: 'Saved shows calendar' })
    ).toBeVisible()
    await expect(panel.getByText('Google Calendar', { exact: true })).toBeVisible()
    // Connected, so no longer an add row.
    await expect(
      panel.getByRole('button', { name: 'Google Calendar' })
    ).toHaveCount(0)

    // Closing the panel puts focus back on the Save control.
    await page.keyboard.press('Escape')
    await expect(panel).toBeHidden()
    await expect(
      page.getByRole('button', { name: /^Remove from saved shows/ })
    ).toBeFocused()
  })
})
