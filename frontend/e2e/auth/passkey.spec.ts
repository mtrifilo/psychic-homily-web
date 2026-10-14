import { test } from '../fixtures/error-detection'
import { expect, type Page } from '@playwright/test'

// Every passkey ceremony runs against Chromium's virtual authenticator, a
// platform (internal) authenticator with resident keys and user verification
// that answers prompts without interaction. Each test signs up its own
// account, so nothing here touches the seeded users.

async function addPlatformAuthenticator(page: Page): Promise<() => Promise<number>> {
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('WebAuthn.enable')
  const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2',
      transport: 'internal',
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  })
  return async () => {
    const { credentials } = await cdp.send('WebAuthn.getCredentials', { authenticatorId })
    return credentials.length
  }
}

function uniqueEmail(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@test.local`
}

async function signInWithPasskey(page: Page) {
  await page.context().clearCookies()
  await page.goto('/auth')
  await page.getByRole('button', { name: /sign in with passkey/i }).click()
  await page.waitForURL((url) => !url.pathname.startsWith('/auth'), { timeout: 15_000 })
  await expect(page.getByRole('button', { name: 'User menu' })).toBeVisible({ timeout: 10_000 })
}

test.describe('Passkeys', () => {
  test('signs up with a passkey, then signs in with it', async ({ page }) => {
    const storedCredentials = await addPlatformAuthenticator(page)

    await page.goto('/auth')
    await page.getByRole('tab', { name: 'Create account' }).click()
    await page.getByRole('button', { name: /sign up with passkey/i }).click()
    await page.locator('#passkey-email').fill(uniqueEmail('e2e-passkey-signup'))
    await page.locator('#passkey-terms').check()
    await page.locator('#passkey-age-confirmation').check()
    await page.getByRole('button', { name: /continue with passkey/i }).click()

    // The account exists and the session is live.
    await expect(page.getByRole('button', { name: 'User menu' })).toBeVisible({ timeout: 15_000 })
    expect(await storedCredentials()).toBe(1)

    await signInWithPasskey(page)
  })

  test('adds a passkey from settings, then signs in with it', async ({ page }) => {
    const storedCredentials = await addPlatformAuthenticator(page)

    // A fresh password account: adding a passkey needs a recent sign-in.
    await page.goto('/auth')
    await page.getByRole('tab', { name: 'Create account' }).click()
    await page.locator('#signup-email').fill(uniqueEmail('e2e-passkey-settings'))
    await page.locator('#signup-password').fill('Xq9!mzPh2wLk_e2e')
    await page.locator('#terms').check()
    await page.locator('#age-confirmation').check()
    await page.getByRole('button', { name: 'Create account' }).click()
    await expect(page.getByRole('button', { name: 'User menu' })).toBeVisible({ timeout: 15_000 })

    await page.goto('/profile?tab=settings')
    await page.getByRole('button', { name: 'Add passkey' }).click()
    await page.locator('#passkey-name').fill('Virtual authenticator')
    await page.getByRole('button', { name: /register passkey/i }).click()

    await expect(page.getByText('Virtual authenticator')).toBeVisible({ timeout: 15_000 })
    expect(await storedCredentials()).toBe(1)

    await signInWithPasskey(page)
  })
})
