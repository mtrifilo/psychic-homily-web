/**
 * Every day of a calendar month as the two-digit segment a day URL carries,
 * `01` through the month's real last day, 28 to 31 of them. `month` is 1-12.
 */
export function daysOfMonth(year: number, month: number): string[] {
  // Day 0 of the following month is the last day of this one.
  const length = new Date(Date.UTC(year, month, 0)).getUTCDate()
  return Array.from({ length }, (_, index) => String(index + 1).padStart(2, '0'))
}

/**
 * The earliest day of the month that `listedDays` does not contain, or
 * `undefined` when the list covers every day. `listedDays` holds two-digit day
 * segments, the same shape {@link daysOfMonth} returns.
 */
export function firstUnlistedDay(
  year: number,
  month: number,
  listedDays: ReadonlySet<string>
): string | undefined {
  return daysOfMonth(year, month).find(day => !listedDays.has(day))
}
