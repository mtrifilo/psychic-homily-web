'use client'

import { Input } from '@/components/ui/input'
import { MAX_PLAUSIBLE_AGE } from '@/lib/legal'

/**
 * The date-of-birth errors: a part left empty, and a date that does not exist
 * or falls outside the plausible range (dateOfBirthMissingMessage and
 * dateOfBirthInvalidMessage in backend/internal/api/handlers/auth/signup_age.go;
 * a backend test fails when the two differ).
 */
export const DATE_OF_BIRTH_MISSING_MESSAGE = 'Enter your date of birth.'
export const DATE_OF_BIRTH_INVALID_MESSAGE = 'Enter a real date of birth.'

export type DateOfBirthPart = 'month' | 'day' | 'year'

/** The three parts as typed: digits only, possibly empty. */
export interface DateOfBirthParts {
  month: string
  day: string
  year: string
}

export const EMPTY_DATE_OF_BIRTH: DateOfBirthParts = { month: '', day: '', year: '' }

/** What is wrong with a date of birth, and which parts take the error state. */
export interface DateOfBirthProblem {
  message: string
  parts: DateOfBirthPart[]
}

const PART_ORDER: readonly DateOfBirthPart[] = ['month', 'day', 'year']

// The latest calendar date anywhere on Earth is the date at UTC+14; the
// backend's legal.AgeOn and legal.ParseDateOfBirth count against it.
const LATEST_UTC_OFFSET_MS = 14 * 60 * 60 * 1000

interface CalendarDate {
  year: number
  month: number
  day: number
}

function latestCalendarDate(now: Date): CalendarDate {
  const there = new Date(now.getTime() + LATEST_UTC_OFFSET_MS)
  return {
    year: there.getUTCFullYear(),
    month: there.getUTCMonth() + 1,
    day: there.getUTCDate(),
  }
}

function daysInMonth(year: number, month: number): number {
  return new Date(Date.UTC(year, month, 0)).getUTCDate()
}

function compareDates(a: CalendarDate, b: CalendarDate): number {
  return a.year - b.year || a.month - b.month || a.day - b.day
}

/** Whole years from birth to today, counted the way legal.AgeOn counts them. */
function ageOn(birth: CalendarDate, today: CalendarDate): number {
  const birthdayPassed =
    today.month > birth.month || (today.month === birth.month && today.day >= birth.day)
  return today.year - birth.year - (birthdayPassed ? 0 : 1)
}

/**
 * Why the parts do not name a plausible date of birth, or null when they do.
 * Plausible mirrors the backend: a calendar date that exists, no later than
 * the latest date anywhere at `now`, and giving an age of at most
 * MAX_PLAUSIBLE_AGE then. The backend stays the authority; this keeps a date
 * it would refuse from leaving the page.
 */
export function dateOfBirthProblem(
  parts: DateOfBirthParts,
  now: Date = new Date()
): DateOfBirthProblem | null {
  const empty = PART_ORDER.filter(part => parts[part] === '')
  if (empty.length > 0) {
    return { message: DATE_OF_BIRTH_MISSING_MESSAGE, parts: empty }
  }

  const invalid = (...wrong: DateOfBirthPart[]): DateOfBirthProblem => ({
    message: DATE_OF_BIRTH_INVALID_MESSAGE,
    parts: wrong,
  })
  const month = /^\d{1,2}$/.test(parts.month) ? Number(parts.month) : NaN
  const day = /^\d{1,2}$/.test(parts.day) ? Number(parts.day) : NaN
  const year = /^\d{4}$/.test(parts.year) ? Number(parts.year) : NaN
  const monthOk = month >= 1 && month <= 12
  const yearOk = !Number.isNaN(year)
  const dayLimit = monthOk && yearOk ? daysInMonth(year, month) : 31
  const dayOk = day >= 1 && day <= dayLimit
  const wrong = PART_ORDER.filter(
    part => !{ month: monthOk, day: dayOk, year: yearOk }[part]
  )
  if (wrong.length > 0) {
    return invalid(...wrong)
  }

  const birth = { year, month, day }
  const today = latestCalendarDate(now)
  if (compareDates(birth, today) > 0) {
    if (year > today.year) return invalid('year')
    if (month > today.month) return invalid('month')
    return invalid('day')
  }
  if (ageOn(birth, today) > MAX_PLAUSIBLE_AGE) {
    return invalid('year')
  }
  return null
}

/**
 * The parts as the YYYY-MM-DD date the backend reads, or null when they do
 * not name a plausible date of birth.
 */
export function dateOfBirthFromParts(
  parts: DateOfBirthParts,
  now: Date = new Date()
): string | null {
  if (dateOfBirthProblem(parts, now) !== null) {
    return null
  }
  return `${parts.year}-${parts.month.padStart(2, '0')}-${parts.day.padStart(2, '0')}`
}

/** The id of one part's input, under the field's idPrefix. */
export function dateOfBirthPartId(idPrefix: string, part: DateOfBirthPart): string {
  return `${idPrefix}-${part}`
}

/**
 * The id of the input a refused submit focuses: the first part in the error
 * state, or the month when the parts are fine.
 */
export function firstInvalidDateOfBirthPartId(
  idPrefix: string,
  parts: DateOfBirthParts,
  now: Date = new Date()
): string {
  const first = dateOfBirthProblem(parts, now)?.parts[0] ?? 'month'
  return dateOfBirthPartId(idPrefix, first)
}

const PART_INPUTS: readonly {
  part: DateOfBirthPart
  label: string
  placeholder: string
  autoComplete: string
  maxLength: number
  className: string
}[] = [
  { part: 'month', label: 'Month', placeholder: 'MM', autoComplete: 'bday-month', maxLength: 2, className: 'w-16' },
  { part: 'day', label: 'Day', placeholder: 'DD', autoComplete: 'bday-day', maxLength: 2, className: 'w-16' },
  { part: 'year', label: 'Year', placeholder: 'YYYY', autoComplete: 'bday-year', maxLength: 4, className: 'w-[88px]' },
]

interface DateOfBirthFieldProps {
  /** Prefixes every id the field renders; unique on the page. */
  idPrefix: string
  value: DateOfBirthParts
  onChange: (value: DateOfBirthParts) => void
  onBlur?: () => void
  /**
   * Whether a submit has been attempted. Until then the field shows no error;
   * after, it shows the problem with the current value, if any.
   */
  showError: boolean
  disabled?: boolean
}

/**
 * The date of birth as three inputs, Month / Day / Year, with no default and
 * the promise that it is never shown on the profile. Controlled: the caller
 * holds the parts and reads the date with dateOfBirthFromParts.
 */
export function DateOfBirthField({
  idPrefix,
  value,
  onChange,
  onBlur,
  showError,
  disabled = false,
}: DateOfBirthFieldProps) {
  const problem = showError ? dateOfBirthProblem(value) : null
  const hintId = `${idPrefix}-hint`
  const errorId = `${idPrefix}-error`

  return (
    <fieldset
      className="flex flex-col gap-2"
      aria-describedby={problem ? `${hintId} ${errorId}` : hintId}
    >
      <legend className="mb-2 text-sm font-medium text-foreground">Date of birth</legend>
      <p id={hintId} className="text-xs text-muted-foreground">
        Never shown on your profile.
      </p>
      <div className="flex items-start gap-3">
        {PART_INPUTS.map(({ part, label, placeholder, autoComplete, maxLength, className }) => {
          const id = dateOfBirthPartId(idPrefix, part)
          return (
            <div key={part} className="flex flex-col gap-1.5">
              <label htmlFor={id} className="text-xs text-muted-foreground">
                {label}
              </label>
              <Input
                id={id}
                inputMode="numeric"
                autoComplete={autoComplete}
                placeholder={placeholder}
                maxLength={maxLength}
                className={className}
                value={value[part]}
                disabled={disabled}
                aria-invalid={problem?.parts.includes(part) ?? false}
                onBlur={onBlur}
                onChange={event =>
                  onChange({ ...value, [part]: event.target.value.replace(/\D/g, '') })
                }
              />
            </div>
          )
        })}
      </div>
      {problem && (
        <p id={errorId} role="alert" className="text-sm text-destructive">
          {problem.message}
        </p>
      )}
    </fieldset>
  )
}
