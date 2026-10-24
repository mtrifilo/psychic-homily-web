import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'
import { fillDateOfBirth } from '../helpers/date-of-birth'

const REGISTER_USER = {
  email: 'e2e-register@test.local',
  password: 'Xq9!mzPh2wLk_e2e',
}


test.describe('Registration', () => {
  test('registers a new account and lands on the check-your-inbox interstitial', { tag: '@smoke' }, async ({ page }) => {
    await page.goto('/auth')

    // Switch to signup tab
    await page.getByRole('tab', { name: 'Create account' }).click()

    // Wait for signup form to render
    await expect(page.locator('#signup-email')).toBeVisible({ timeout: 5_000 })

    // Fill registration form
    await page.locator('#signup-email').fill(REGISTER_USER.email)
    await page.locator('#signup-password').fill(REGISTER_USER.password)
    await page.locator('#terms').check()
    await fillDateOfBirth(page, 'signup-dob')

    // Submit
    await page.getByRole('button', { name: 'Create account' }).click()

    // PSY-1900: signup no longer navigates on success. The card is replaced
    // in place by the check-your-inbox interstitial, which names the address
    // the verification link went to.
    await expect(
      page.getByRole('heading', { name: 'Check your inbox.' })
    ).toBeVisible({ timeout: 15_000 })
    // `exact` keeps this off the surrounding sentence, which also contains the
    // address, so strict mode sees one node rather than two.
    await expect(
      page.getByText(REGISTER_USER.email, { exact: true })
    ).toBeVisible()
    await expect(page.getByText(/It expires in 24 hours/)).toBeVisible()
    await expect(
      page.getByRole('link', { name: 'Browse upcoming shows' })
    ).toBeVisible()

    // The session is live even though the user has not verified yet.
    await expect(
      page.getByRole('button', { name: 'User menu' })
    ).toBeVisible({ timeout: 5_000 })
  })

  test('shows password strength requirements', async ({ page }) => {
    await page.goto('/auth')
    await page.getByRole('tab', { name: 'Create account' }).click()
    await expect(page.locator('#signup-password')).toBeVisible({
      timeout: 5_000,
    })

    const registerRequests: string[] = []
    page.on('request', request => {
      if (new URL(request.url()).pathname.endsWith('/auth/register')) {
        registerRequests.push(request.url())
      }
    })

    // Every field valid but the password, so Password is the first invalid one
    await page.locator('#signup-email').fill('short-password@test.local')
    await page.locator('#signup-password').fill('short')
    await page.locator('#terms').check()
    await page.locator('#age-confirmation').check()

    // Password strength meter should show unmet requirements
    await expect(
      page.getByText('At least 12 characters', { exact: true })
    ).toBeVisible({ timeout: 5_000 })

    // The submit button stays enabled; pressing it shows the field's error
    // and moves focus to the first invalid field instead of submitting.
    const submit = page.getByRole('button', { name: 'Create account' })
    await expect(submit).toBeEnabled()
    await submit.click()

    await expect(
      page.getByText('Password must be at least 12 characters')
    ).toBeVisible({ timeout: 5_000 })
    await expect(page.locator('#signup-password')).toBeFocused()
    await expect(page.locator('#signup-password')).toHaveAttribute('aria-invalid', 'true')
    await expect(submit).toBeEnabled()

    // Still on /auth, and nothing was sent
    expect(page.url()).toContain('/auth')
    expect(registerRequests).toHaveLength(0)
  })

  test('shows error for breached password', async ({ page }) => {
    await page.goto('/auth')
    await page.getByRole('tab', { name: 'Create account' }).click()
    await expect(page.locator('#signup-email')).toBeVisible({ timeout: 5_000 })

    // Fill form with a commonly breached password
    await page.locator('#signup-email').fill('breach-test@test.local')
    await page.locator('#signup-password').fill('TestPassword123!')
    await page.locator('#terms').check()
    await fillDateOfBirth(page, 'signup-dob')

    // Submit — server will reject the breached password
    await page.getByRole('button', { name: 'Create account' }).click()

    // PSY-474: can't use `page.getByRole('alert')` unscoped — Next.js's
    // RouteAnnouncer renders a permanent empty `role="alert"` live region
    // at the page root for route narration, so any form-level alert makes
    // the selector match 2 elements and trip strict-mode. The text check
    // below is content-specific and sufficient on its own.
    await expect(
      page.getByText(/password has been exposed in a data breach/i)
    ).toBeVisible({ timeout: 10_000 })

    // Still on /auth
    expect(page.url()).toContain('/auth')
  })
})

test.describe('Date of birth', () => {
  test('refuses an impossible date on the form, with the field focused and the button enabled', async ({ page }) => {
    await page.goto('/auth')
    await page.getByRole('tab', { name: 'Create account' }).click()
    await expect(page.locator('#signup-email')).toBeVisible({ timeout: 5_000 })

    await page.locator('#signup-email').fill('e2e-dob-impossible@test.local')
    await page.locator('#signup-password').fill(REGISTER_USER.password)
    await page.locator('#terms').check()
    await fillDateOfBirth(page, 'signup-dob', { month: '2', day: '30', year: '2004' })
    const submit = page.getByRole('button', { name: 'Create account' })
    await submit.click()

    await expect(page.getByText('Enter a real date of birth.')).toBeVisible()
    await expect(page.locator('#signup-dob-day')).toBeFocused()
    await expect(submit).toBeEnabled()
  })

  test('an under-16 date ends on the refusal, which Create account keeps showing after a reload', async ({ page }) => {
    await page.goto('/auth')
    await page.getByRole('tab', { name: 'Create account' }).click()
    await expect(page.locator('#signup-email')).toBeVisible({ timeout: 5_000 })

    const now = new Date()
    await page.locator('#signup-email').fill('e2e-dob-under-age@test.local')
    await page.locator('#signup-password').fill(REGISTER_USER.password)
    await page.locator('#terms').check()
    await fillDateOfBirth(page, 'signup-dob', {
      month: String(now.getUTCMonth() + 1),
      day: '1',
      year: String(now.getUTCFullYear() - 10),
    })
    await page.getByRole('button', { name: 'Create account' }).click()

    const heading = page.getByRole('heading', { name: "We can't create an account for you." })
    await expect(heading).toBeVisible({ timeout: 10_000 })
    await expect(heading).toBeFocused()
    await expect(page.getByRole('tab', { name: 'Sign in' })).toHaveCount(0)

    await page.reload()
    await page.getByRole('tab', { name: 'Create account' }).click()
    await expect(heading).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('#signup-email')).toHaveCount(0)

    await page.getByRole('tab', { name: 'Sign in' }).click()
    await expect(page.locator('#email')).toBeVisible()
  })
})

// A gated control names the action it refused; that arrival opens on Create
// account. A returnTo alone (the header link, the mobile Account sheet) and an
// unlisted intent both open on Sign in.
test.describe('Initial tab', () => {
  test('a gated arrival opens on Create account with its account ledger', async ({ page }) => {
    await page.goto('/auth?returnTo=%2Fshows&intent=save')

    await expect(
      page.getByRole('tab', { name: 'Create account' })
    ).toHaveAttribute('aria-selected', 'true')
    await expect(
      page.getByRole('heading', { name: 'Never miss a show.' })
    ).toBeVisible()
    await expect(page.locator('#signup-email')).toBeVisible()
  })

  for (const [label, query] of [
    ['a returnTo that names no intent', 'returnTo=%2Fshows'],
    ['an unlisted intent', 'returnTo=%2Fshows&intent=bogus'],
  ] as const) {
    test(`${label} opens on Sign in`, async ({ page }) => {
      await page.goto(`/auth?${query}`)

      await expect(
        page.getByRole('tab', { name: 'Sign in' })
      ).toHaveAttribute('aria-selected', 'true')
      await expect(page.getByText('Sign in to your account')).toBeVisible()
    })
  }
})
