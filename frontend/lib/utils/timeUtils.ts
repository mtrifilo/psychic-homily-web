/**
 * Time utility functions for handling date/time conversions
 */

/**
 * Map of US states to their IANA timezones
 * Most shows are in Arizona which doesn't observe DST
 */
const STATE_TIMEZONES: Record<string, string> = {
  AZ: 'America/Phoenix',
  CA: 'America/Los_Angeles',
  NV: 'America/Los_Angeles',
  CO: 'America/Denver',
  NM: 'America/Denver',
  TX: 'America/Chicago',
  NY: 'America/New_York',
  // Add more as needed
}

/**
 * Get timezone for a US state (defaults to America/Phoenix for Arizona shows)
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
