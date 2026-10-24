import type { Page } from '@playwright/test'

export interface DateOfBirthInput {
  month: string
  day: string
  year: string
}

/** A date of birth well over the minimum age. */
export const ADULT_DATE_OF_BIRTH: DateOfBirthInput = { month: '6', day: '15', year: '1990' }

/**
 * Fills the sign-up date-of-birth field whose inputs carry `idPrefix`
 * (`signup-dob` on the password form, `passkey-dob` in the passkey dialog).
 */
export async function fillDateOfBirth(
  page: Page,
  idPrefix: string,
  date: DateOfBirthInput = ADULT_DATE_OF_BIRTH
): Promise<void> {
  await page.locator(`#${idPrefix}-month`).fill(date.month)
  await page.locator(`#${idPrefix}-day`).fill(date.day)
  await page.locator(`#${idPrefix}-year`).fill(date.year)
}
