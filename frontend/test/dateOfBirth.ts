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
  // A click and a paste per part: the fewest events that still go through
  // the inputs' change handlers.
  for (const [label, value] of [
    ['Month', ADULT_DATE_OF_BIRTH.month],
    ['Day', ADULT_DATE_OF_BIRTH.day],
    ['Year', ADULT_DATE_OF_BIRTH.year],
  ] as const) {
    await user.click(scope.getByLabelText(label))
    await user.paste(value)
  }
}
