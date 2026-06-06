/**
 * Time utility functions for handling date/time conversions
 */

/**
 * Map of US state codes to their IANA timezones. This is the FALLBACK used only
 * when a venue has no resolved `timezone` (e.g. rows created before PSY-985's
 * geocoding, until the PSY-987 backfill runs). New code should prefer
 * `venue.timezone`; this map only narrows the guess for US venues.
 *
 * Kept in sync with the CLI's map in cli/src/lib/timezone.ts — both must list
 * the same states. (PSY-986 fixed the prior drift: this map had only 7 states,
 * so non-listed US states like IL/WA/MN/MA silently fell back to Arizona time.)
 */
const STATE_TIMEZONES: Record<string, string> = {
  AZ: 'America/Phoenix',
  CA: 'America/Los_Angeles',
  NV: 'America/Los_Angeles',
  CO: 'America/Denver',
  NM: 'America/Denver',
  TX: 'America/Chicago',
  NY: 'America/New_York',
  // Eastern
  CT: 'America/New_York',
  DC: 'America/New_York',
  DE: 'America/New_York',
  FL: 'America/New_York',
  GA: 'America/New_York',
  MA: 'America/New_York',
  MD: 'America/New_York',
  ME: 'America/New_York',
  MI: 'America/New_York',
  NC: 'America/New_York',
  NH: 'America/New_York',
  NJ: 'America/New_York',
  OH: 'America/New_York',
  PA: 'America/New_York',
  RI: 'America/New_York',
  SC: 'America/New_York',
  VA: 'America/New_York',
  VT: 'America/New_York',
  WV: 'America/New_York',
  // Central
  AL: 'America/Chicago',
  AR: 'America/Chicago',
  IA: 'America/Chicago',
  IL: 'America/Chicago',
  IN: 'America/Indiana/Indianapolis',
  KS: 'America/Chicago',
  KY: 'America/New_York',
  LA: 'America/Chicago',
  MN: 'America/Chicago',
  MO: 'America/Chicago',
  MS: 'America/Chicago',
  ND: 'America/Chicago',
  NE: 'America/Chicago',
  OK: 'America/Chicago',
  SD: 'America/Chicago',
  TN: 'America/Chicago',
  WI: 'America/Chicago',
  // Mountain
  ID: 'America/Boise',
  MT: 'America/Denver',
  UT: 'America/Denver',
  WY: 'America/Denver',
  // Pacific
  OR: 'America/Los_Angeles',
  WA: 'America/Los_Angeles',
  // Non-contiguous
  AK: 'America/Anchorage',
  HI: 'Pacific/Honolulu',
}

/**
 * Get the IANA timezone for a US state. Defaults to America/Phoenix (Arizona,
 * no DST) for unknown/international input — callers should prefer a venue's
 * resolved `timezone` and use this only as a fallback.
 */
export function getTimezoneForState(state: string): string {
  return STATE_TIMEZONES[state.toUpperCase()] || 'America/Phoenix'
}

/**
 * Combines a date string and time string into a UTC ISO timestamp
 * Treats the input as local time in the specified timezone
 *
 * @param dateString - Date in YYYY-MM-DD format (from date input)
 * @param timeString - Time in HH:MM format (from time input)
 * @param timezone - IANA timezone (e.g., 'America/Phoenix'). Defaults to browser timezone.
 * @returns ISO 8601 timestamp in UTC
 */
export function combineDateTimeToUTC(
  dateString: string,
  timeString: string,
  timezone?: string
): string {
  // Parse date and time parts manually to avoid JS Date timezone quirks
  const [year, month, day] = dateString.split('-').map(Number)
  const [hours, minutes] = timeString.split(':').map(Number)

  if (!timezone) {
    // No timezone specified — use browser-local behavior (backward compatible)
    const date = new Date(year, month - 1, day, hours, minutes, 0, 0)
    return date.toISOString().replace(/\.\d{3}Z$/, 'Z')
  }

  // Timezone-aware: interpret the date/time as being in the target timezone.
  // 1. Create a UTC date with the desired wall-clock values
  const utcGuess = Date.UTC(year, month - 1, day, hours, minutes, 0, 0)

  // 2. Probe the target timezone's UTC offset at that instant
  const formatter = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
  const parts = formatter.formatToParts(new Date(utcGuess))
  const p = (type: string) => Number(parts.find(x => x.type === type)?.value ?? 0)
  const tzYear = p('year')
  const tzMonth = p('month')
  const tzDay = p('day')
  let tzHour = p('hour')
  if (tzHour === 24) tzHour = 0 // Intl may return 24 for midnight
  const tzMinute = p('minute')

  // 3. The offset (in ms) is how much the timezone's wall clock differs from our UTC guess
  const localAsUtc = Date.UTC(tzYear, tzMonth - 1, tzDay, tzHour, tzMinute, 0, 0)
  const offsetMs = localAsUtc - utcGuess

  // 4. Subtract the offset to get the correct UTC time
  const corrected = new Date(utcGuess - offsetMs)

  // Convert to UTC and return as RFC3339 format (without milliseconds)
  // Go's time.Time parser expects this format
  return corrected.toISOString().replace(/\.\d{3}Z$/, 'Z')
}

/**
 * Format a UTC date string for display in a specific timezone
 *
 * @param utcDateString - ISO 8601 date string in UTC
 * @param timezone - IANA timezone (e.g., 'America/Phoenix')
 * @param options - Intl.DateTimeFormat options
 */
export function formatInTimezone(
  utcDateString: string,
  timezone: string,
  options: Intl.DateTimeFormatOptions
): string {
  const date = new Date(utcDateString)
  return date.toLocaleString('en-US', { ...options, timeZone: timezone })
}

/**
 * Format date as "Mon, Dec 1" in specified timezone
 */
export function formatDateInTimezone(
  utcDateString: string,
  timezone: string
): string {
  return formatInTimezone(utcDateString, timezone, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  })
}

/**
 * Format date as "Sat Apr 5, 2025" in specified timezone (includes year)
 */
export function formatDateWithYearInTimezone(
  utcDateString: string,
  timezone: string
): string {
  const date = new Date(utcDateString)
  const formatter = new Intl.DateTimeFormat('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    timeZone: timezone,
  })

  const parts = formatter.formatToParts(date)
  const weekday = parts.find(p => p.type === 'weekday')?.value || ''
  const month = parts.find(p => p.type === 'month')?.value || ''
  const day = parts.find(p => p.type === 'day')?.value || ''
  const year = parts.find(p => p.type === 'year')?.value || ''

  return `${weekday} ${month} ${day}, ${year}`
}

/**
 * Format time as "7:30 PM" in specified timezone
 */
export function formatTimeInTimezone(
  utcDateString: string,
  timezone: string
): string {
  return formatInTimezone(utcDateString, timezone, {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

/**
 * Parse an ISO date string into separate date and time strings for form inputs
 * Returns date in YYYY-MM-DD format and time in HH:MM format
 *
 * @param isoDateString - ISO 8601 date string
 * @returns Object with date and time strings
 */
export function parseISOToDateAndTime(
  isoDateString: string,
  timezone?: string
): {
  date: string
  time: string
} {
  const dateObj = new Date(isoDateString)

  if (!timezone) {
    // No timezone — use browser-local behavior (backward compatible)
    const year = dateObj.getFullYear()
    const month = String(dateObj.getMonth() + 1).padStart(2, '0')
    const day = String(dateObj.getDate()).padStart(2, '0')
    const date = `${year}-${month}-${day}`

    const hours = String(dateObj.getHours()).padStart(2, '0')
    const minutes = String(dateObj.getMinutes()).padStart(2, '0')
    const time = `${hours}:${minutes}`

    return { date, time }
  }

  // Timezone-aware: extract date/time parts in the target timezone
  const formatter = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
  const parts = formatter.formatToParts(dateObj)
  const p = (type: string) => parts.find(x => x.type === type)?.value ?? '00'

  let hourVal = p('hour')
  if (hourVal === '24') hourVal = '00' // Intl may return 24 for midnight

  const date = `${p('year')}-${p('month')}-${p('day')}`
  const time = `${hourVal}:${p('minute')}`

  return { date, time }
}
