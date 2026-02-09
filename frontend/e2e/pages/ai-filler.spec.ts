import * as path from 'path'
import { test, expect } from '../fixtures'

const MOCK_EXTRACTION_RESPONSE = {
  success: true,
  data: {
    artists: [
      {
        name: 'The National',
        is_headliner: true,
        matched_id: 1,
        matched_name: 'The National',
        matched_slug: 'the-national',
      },
    ],
    venue: {
      name: 'Valley Bar',
      city: 'Phoenix',
      state: 'AZ',
      matched_id: 1,
      matched_name: 'Valley Bar',
      matched_slug: 'valley-bar-phoenix-az',
    },
    date: '2026-03-15',
    time: '20:00',
    cost: '$35',
    ages: '21+',
  },
  warnings: [],
}

test.describe('AI Form Filler', () => {
  test('extracts show info from pasted text', async ({
    authenticatedPage,
  }) => {
    // Mock the extraction API at the browser level
    await authenticatedPage.route('**/api/ai/extract-show', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_EXTRACTION_RESPONSE),
      })
    )

    await authenticatedPage.goto('/submissions')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Submit a Show' })
    ).toBeVisible({ timeout: 10_000 })

    // Expand the AI card
    await authenticatedPage.getByText('AI Form Filler-Outer').click()

    // Type flyer text into the AI textarea (disambiguate from ShowForm description textarea)
    await authenticatedPage
      .getByPlaceholder('Paste show details')
      .fill(
        'The National at Valley Bar\nPhoenix AZ\nMarch 15, 2026\n$35 / 21+'
      )

    // Click extract
    await authenticatedPage
      .getByRole('button', { name: 'Extract Show Info' })
      .click()

    // Assert extraction complete
    await expect(
      authenticatedPage.getByText('Extraction Complete')
    ).toBeVisible({ timeout: 10_000 })

    // Assert extracted artist badge visible
    await expect(
      authenticatedPage.getByText('The National').first()
    ).toBeVisible()

    // Assert extracted venue badge visible
    await expect(
      authenticatedPage.getByText('Valley Bar').first()
    ).toBeVisible()

    // Verify form auto-populated: venue city
    await expect(
      authenticatedPage.locator('[id="venue.city"]')
    ).toHaveValue('Phoenix', { timeout: 5_000 })

    // Verify date field
    await expect(authenticatedPage.locator('#date')).toHaveValue('2026-03-15')
  })

  test('extracts show info from uploaded image', async ({
    authenticatedPage,
  }) => {
    // Mock the extraction API
    await authenticatedPage.route('**/api/ai/extract-show', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_EXTRACTION_RESPONSE),
      })
    )

    await authenticatedPage.goto('/submissions')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Submit a Show' })
    ).toBeVisible({ timeout: 10_000 })

    // Expand the AI card
    await authenticatedPage.getByText('AI Form Filler-Outer').click()

    // Upload test image via the hidden file input
    const testImagePath = path.resolve(
      __dirname,
      '../fixtures/test-flyer.png'
    )
    await authenticatedPage
      .locator('input[type="file"]')
      .setInputFiles(testImagePath)

    // Assert image preview appears
    await expect(
      authenticatedPage.locator('img[alt="Uploaded flyer"]')
    ).toBeVisible({ timeout: 5_000 })

    // Click extract
    await authenticatedPage
      .getByRole('button', { name: 'Extract Show Info' })
      .click()

    // Assert extraction complete
    await expect(
      authenticatedPage.getByText('Extraction Complete')
    ).toBeVisible({ timeout: 10_000 })

    // Verify form auto-populated
    await expect(
      authenticatedPage.locator('[id="venue.city"]')
    ).toHaveValue('Phoenix', { timeout: 5_000 })

    await expect(authenticatedPage.locator('#date')).toHaveValue('2026-03-15')
  })

  test('shows error when extraction fails', async ({
    authenticatedPage,
  }) => {
    // Mock the extraction API to return an error (use 200 + success:false
    // to avoid triggering the error-detection fixture's 5xx check)
    await authenticatedPage.route('**/api/ai/extract-show', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: false,
          error: 'AI service is temporarily unavailable',
        }),
      })
    )

    await authenticatedPage.goto('/submissions')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Submit a Show' })
    ).toBeVisible({ timeout: 10_000 })

    // Expand the AI card
    await authenticatedPage.getByText('AI Form Filler-Outer').click()

    // Type text and extract
    await authenticatedPage
      .getByPlaceholder('Paste show details')
      .fill('Some show details')
    await authenticatedPage
      .getByRole('button', { name: 'Extract Show Info' })
      .click()

    // Assert error alert appears
    await expect(
      authenticatedPage.getByText('AI service is temporarily unavailable')
    ).toBeVisible({ timeout: 10_000 })
  })
})
