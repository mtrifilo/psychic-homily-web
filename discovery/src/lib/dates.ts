/**
 * Returns today's date as YYYY-MM-DD in the local timezone.
 * Avoids the UTC shift bug in `new Date().toISOString().split('T')[0]`
 * which can return yesterday's date after midnight in western timezones.
 */
export function getLocalDateString(date: Date = new Date()): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}
