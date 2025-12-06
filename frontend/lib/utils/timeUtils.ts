/**
 * Time utility functions for handling date/time conversions
 */

/**
 * Combines a date string and time string into a UTC ISO timestamp
 * @param dateString - Date in YYYY-MM-DD format (from date input)
 * @param timeString - Time in HH:MM format (from time input)
 * @returns ISO 8601 timestamp in UTC
 */
export function combineDateTimeToUTC(
  dateString: string,
  timeString: string
): string {
  // Create a date object from the date string (assumes local timezone)
  const date = new Date(dateString)

  // Parse the time string (HH:MM format)
  const [hours, minutes] = timeString.split(':').map(Number)

  // Set the time on the date object
  date.setHours(hours, minutes, 0, 0)

  // Convert to UTC and return as ISO string
  return date.toISOString()
}

