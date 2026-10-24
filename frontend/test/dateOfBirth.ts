import { screen } from '@testing-library/react'
import type userEvent from '@testing-library/user-event'

/** A date of birth well over the minimum age, as typed and as sent. */
export const ADULT_DATE_OF_BIRTH = {
  month: '6',
  day: '15',
  year: '1990',
  sent: '1990-06-15',
} as const

/**
 * Types ADULT_DATE_OF_BIRTH into the date-of-birth field found in `scope`
 * (the whole screen by default, or `within(dialog)` for the passkey dialog).
 */
export async function fillDateOfBirth(
  user: ReturnType<typeof userEvent.setup>,
  scope: Pick<typeof screen, 'getByLabelText'> = screen
): Promise<void> {
  await user.type(scope.getByLabelText('Month'), ADULT_DATE_OF_BIRTH.month)
  await user.type(scope.getByLabelText('Day'), ADULT_DATE_OF_BIRTH.day)
  await user.type(scope.getByLabelText('Year'), ADULT_DATE_OF_BIRTH.year)
}
