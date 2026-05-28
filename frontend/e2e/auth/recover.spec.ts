import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'
import { createAccountRecoveryToken } from '../helpers/jwt'
import { execSync } from 'child_process'

// PSY-719: end-to-end coverage for soft-deleted account recovery
// (frontend/app/auth/recover/page.tsx + backend
// RequestAccountRecoveryHandler / ConfirmAccountRecoveryHandler).
//
// Two distinct stages, mirrored from the page's two render branches:
//   1. Request: the user submits their email on /auth/recover. No real email
//      is delivered in the E2E env — and the backend has no email provider
//      configured here, so the request surfaces a config error inline before
//      the enumeration-safe "sent" confirmation (see the test comment for the
//      harness limitation and the email-mock follow-up).
//   2. Completion: the recovery link (`?token=`) auto-fires
//      ConfirmAccountRecoveryHandler, which restores the soft-deleted account
//      and logs the user in. This is the substantive, fully-exercised leg.
//
// The completion target is the dedicated soft-deleted-and-recoverable fixture
// user seeded by setup-db.sh (is_active=false, deleted_at=NOW() → inside the
// 30-day grace window). It is NOT a per-worker login user, so restoring it
// mid-suite can't disturb parallel workers' auth state.

const RECOVERY_USER_EMAIL = 'e2e-recovery@test.local'
const ACTIVE_USER_EMAIL = 'e2e-user@test.local'
const E2E_DB_URL =
  'postgres://e2euser:e2epassword@localhost:5433/e2edb?sslmode=disable'

/** Look up a user's ID directly from the DB (avoids rate-limited auth endpoints). */
function getUserId(email: string): number {
  const result = execSync(
    `psql "${E2E_DB_URL}" -tAc "SELECT id FROM users WHERE email = '${email}'"`,
    { encoding: 'utf-8' }
  ).trim()
  return parseInt(result, 10)
}

test.describe('Account Recovery', () => {
  test('request form renders and submits the recovery email', async ({ page }) => {
    await page.goto('/auth/recover')

    // The request form renders (not the token-confirmation branch).
    await expect(
      page.getByRole('heading', { name: /recover your account/i })
    ).toBeVisible()
    await expect(page.getByLabel('Email')).toBeVisible()

    // Submit the email of the recoverable account.
    await page.getByLabel('Email').fill(RECOVERY_USER_EMAIL)
    await page
      .getByRole('button', { name: /send recovery email/i })
      .click()

    // HARNESS LIMITATION: the E2E backend configures no email provider
    // (no Resend key / FromEmail in global-setup.ts), so EmailService.
    // IsConfigured() is false and RequestAccountRecoveryHandler returns the
    // pre-lookup SERVICE_UNAVAILABLE error *before* the enumeration-safe
    // "if eligible, sent" path. The page surfaces that inline and stays on
    // the form. We therefore assert the reachable behavior — the form is
    // wired end-to-end and renders the backend's response inline — rather
    // than the "sent" confirmation, which a real email provider would gate.
    // Covering the "sent" happy path needs an e2e email-service mock (same
    // class of gap as the OAuth IdP mock — see oauth-google.spec.ts).
    await expect(
      page.getByText(/email service is not configured/i)
    ).toBeVisible({ timeout: 10_000 })
  })

  test('completion via recovery link restores the account and logs in', async ({
    page,
  }) => {
    const userId = getUserId(RECOVERY_USER_EMAIL)

    // Mint the recovery token the email link would carry. The token, not the
    // email, is what ConfirmAccountRecoveryHandler consumes.
    const token = await createAccountRecoveryToken(userId, RECOVERY_USER_EMAIL)

    // Following the recovery link auto-fires confirmation on mount.
    await page.goto(`/auth/recover?token=${token}`)

    // On success the page pushes to "/" and the user is logged in. Assert the
    // post-flow logged-in marker positively (avatar dropdown), mirroring the
    // login spec, rather than just the absence of an error.
    await page.waitForURL('/', { timeout: 15_000 })
    await expect(
      page.getByRole('button', { name: 'User menu' })
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByRole('link', { name: /login/i })
    ).not.toBeVisible()
  })

  test('shows error when recovery link points at an already-active account', async ({
    page,
  }) => {
    // A worker login user is active, so ConfirmAccountRecoveryHandler returns
    // ACCOUNT_ACTIVE (HTTP 200 + error body) rather than restoring/logging in.
    // The page surfaces the message and offers a fresh-link CTA.
    const userId = getUserId(ACTIVE_USER_EMAIL)
    const token = await createAccountRecoveryToken(userId, ACTIVE_USER_EMAIL)

    await page.goto(`/auth/recover?token=${token}`)

    await expect(
      page.getByText(/this account is already active/i)
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByRole('link', { name: /request a new recovery link/i })
    ).toBeVisible()
  })

  test('shows error for an invalid recovery token', async ({ page }) => {
    await page.goto('/auth/recover?token=invalid-garbage-token')

    await expect(
      page.getByText(/invalid or expired recovery token/i)
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      page.getByRole('link', { name: /request a new recovery link/i })
    ).toBeVisible()
  })
})
