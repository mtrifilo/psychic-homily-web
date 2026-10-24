import { useState } from 'react'
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  DATE_OF_BIRTH_INVALID_MESSAGE,
  DATE_OF_BIRTH_MISSING_MESSAGE,
  DateOfBirthField,
  EMPTY_DATE_OF_BIRTH,
  dateOfBirthFromParts,
  dateOfBirthProblem,
  firstInvalidDateOfBirthPartId,
  type DateOfBirthParts,
} from './date-of-birth-field'

// 2026-10-24 at 12:00 UTC: the latest calendar date anywhere is 2026-10-25.
const NOW = new Date('2026-10-24T12:00:00Z')

const parts = (month: string, day: string, year: string): DateOfBirthParts => ({ month, day, year })

describe('dateOfBirthFromParts', () => {
  it.each([
    ['4', '27', '1998', '1998-04-27'],
    ['04', '07', '2000', '2000-04-07'],
    ['2', '29', '2000', '2000-02-29'],
    // The latest date anywhere at NOW, the backend's "today".
    ['10', '25', '2026', '2026-10-25'],
    // An age of 150 is still plausible; 151 is not.
    ['10', '26', '1875', '1875-10-26'],
  ])('reads %s/%s/%s as %s', (m, d, y, want) => {
    expect(dateOfBirthFromParts(parts(m, d, y), NOW)).toBe(want)
  })

  it.each([
    ['2', '29', '1999'],
    ['2', '30', '2000'],
    ['13', '1', '2000'],
    ['0', '1', '2000'],
    ['1', '0', '2000'],
    ['1', '1', '98'],
    ['', '1', '2000'],
    ['1', '', '2000'],
    ['1', '1', ''],
    ['10', '26', '2026'],
    // 151 years before the latest date anywhere at NOW.
    ['10', '25', '1875'],
  ])('refuses %s/%s/%s', (m, d, y) => {
    expect(dateOfBirthFromParts(parts(m, d, y), NOW)).toBeNull()
  })
})

describe('dateOfBirthProblem', () => {
  it('names every empty part, with the missing sentence', () => {
    expect(dateOfBirthProblem(parts('', '7', ''), NOW)).toEqual({
      message: DATE_OF_BIRTH_MISSING_MESSAGE,
      parts: ['month', 'year'],
    })
  })

  it.each([
    [parts('2', '30', '2004'), ['day']],
    [parts('13', '30', '2004'), ['month']],
    [parts('13', '40', '04'), ['month', 'day', 'year']],
    [parts('1', '1', '2027'), ['year']],
    [parts('12', '1', '2026'), ['month']],
    [parts('10', '30', '2026'), ['day']],
    [parts('1', '1', '1800'), ['year']],
  ])('puts only the failing parts in the error state for %o', (value, want) => {
    expect(dateOfBirthProblem(value, NOW)).toEqual({
      message: DATE_OF_BIRTH_INVALID_MESSAGE,
      parts: want,
    })
  })

  it('finds nothing wrong with a plausible date', () => {
    expect(dateOfBirthProblem(parts('6', '15', '1990'), NOW)).toBeNull()
  })
})

describe('firstInvalidDateOfBirthPartId', () => {
  it('points at the first part in the error state, else the month', () => {
    expect(firstInvalidDateOfBirthPartId('dob', parts('4', '', ''), NOW)).toBe('dob-day')
    expect(firstInvalidDateOfBirthPartId('dob', parts('6', '15', '1990'), NOW)).toBe('dob-month')
  })
})

function Harness({ showError }: { showError: boolean }) {
  const [value, setValue] = useState(EMPTY_DATE_OF_BIRTH)
  return (
    <>
      <DateOfBirthField idPrefix="dob" value={value} onChange={setValue} showError={showError} />
      <output data-testid="value">{dateOfBirthFromParts(value) ?? 'null'}</output>
    </>
  )
}

describe('DateOfBirthField', () => {
  it('renders three empty inputs under a legend, with the profile promise and no error', () => {
    render(<Harness showError={false} />)

    expect(screen.getByRole('group', { name: 'Date of birth' })).toHaveAccessibleDescription(
      'Never shown on your profile.'
    )
    for (const label of ['Month', 'Day', 'Year']) {
      const input = screen.getByLabelText(label)
      expect(input).toHaveValue('')
      expect(input).toHaveAttribute('aria-invalid', 'false')
    }
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('keeps digits only and exposes the date once it is whole', async () => {
    const user = userEvent.setup()
    render(<Harness showError={false} />)

    await user.type(screen.getByLabelText('Month'), 'a6')
    await user.type(screen.getByLabelText('Day'), '15')
    expect(screen.getByTestId('value')).toHaveTextContent('null')
    await user.type(screen.getByLabelText('Year'), '19x90')

    expect(screen.getByLabelText('Month')).toHaveValue('6')
    expect(screen.getByTestId('value')).toHaveTextContent('1990-06-15')
  })

  it('shows the problem once errors are on, and updates it as the parts change', async () => {
    const user = userEvent.setup()
    render(<Harness showError />)

    expect(screen.getByRole('alert')).toHaveTextContent(DATE_OF_BIRTH_MISSING_MESSAGE)
    expect(screen.getByRole('group', { name: 'Date of birth' })).toHaveAccessibleDescription(
      `Never shown on your profile. ${DATE_OF_BIRTH_MISSING_MESSAGE}`
    )

    await user.type(screen.getByLabelText('Month'), '2')
    await user.type(screen.getByLabelText('Day'), '30')
    await user.type(screen.getByLabelText('Year'), '2004')
    expect(screen.getByRole('alert')).toHaveTextContent(DATE_OF_BIRTH_INVALID_MESSAGE)
    expect(screen.getByLabelText('Day')).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByLabelText('Month')).toHaveAttribute('aria-invalid', 'false')

    await user.clear(screen.getByLabelText('Day'))
    await user.type(screen.getByLabelText('Day'), '28')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
