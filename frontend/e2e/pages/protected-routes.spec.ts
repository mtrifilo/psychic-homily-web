import { test } from '../fixtures/error-detection'
import { expect } from '@playwright/test'

test.describe('Protected route redirects', () => {
  test('unauthenticated user is redirected from /collection to /auth', async ({
    page,
  }) => {
    await page.goto('/collection')

    // Should redirect to auth page
    await page.waitForURL(/\/auth/, { timeout: 10_000 })

    // Auth page content should be visible
    await expect(
      page.getByText('Sign in to your account')
    ).toBeVisible({ timeout: 5_000 })
  })

  test('unauthenticated user is redirected from /submissions to /auth', async ({
    page,
  }) => {
    await page.goto('/submissions')

    // Should redirect to auth page
    await page.waitForURL(/\/auth/, { timeout: 10_000 })

    // Auth page content should be visible
    await expect(
      page.getByText('Sign in to your account')
    ).toBeVisible({ timeout: 5_000 })
  })
})
