/**
 * Ensures a date string is treated as UTC.
 *
 * The backend formats timestamps with a literal "Z" suffix (e.g.
 * "2026-03-30T10:00:00Z"), which `new Date()` correctly interprets
 * as UTC.  However, if a timestamp ever arrives without a timezone
 * indicator, `new Date()` treats it as *local* time, which introduces
 * an offset equal to the user's UTC difference (e.g. 7 hours in
 * Arizona / MST).
 *
 * This helper appends "Z" when the string lacks any timezone suffix
 * so the value is always parsed as UTC.
 */
function ensureUTC(dateStr: string): Date {
  // Already has a timezone indicator: ends with "Z", or has a +/-HH:MM / +/-HHMM offset
  if (/Z$|[+-]\d{2}:\d{2}$|[+-]\d{4}$/.test(dateStr)) {
    return new Date(dateStr)
  }
  return new Date(dateStr + 'Z')
}

/**
 * Format a UTC timestamp string into a human-friendly relative time.
 *
 * Handles both short-form ("2m ago") and long-form ("2 minutes ago")
 * output via the `short` option (default: false).
 */
export function formatRelativeTime(
  dateStr: string,
  options?: { short?: boolean }
): string {
  const date = ensureUTC(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSec = Math.floor(diffMs / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffHr = Math.floor(diffMin / 60)
  const diffDays = Math.floor(diffHr / 24)

  if (options?.short) {
    if (diffMin < 1) return 'just now'
    if (diffMin < 60) return `${diffMin}m ago`
    if (diffHr < 24) return `${diffHr}h ago`
    if (diffDays < 7) return `${diffDays}d ago`
    if (diffDays < 30) return `${Math.floor(diffDays / 7)}w ago`

    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: date.getFullYear() !== now.getFullYear() ? 'numeric' : undefined,
    })
  }

  if (diffSec < 60) return 'just now'
  if (diffMin < 60) return `${diffMin} minute${diffMin === 1 ? '' : 's'} ago`
  if (diffHr < 24) return `${diffHr} hour${diffHr === 1 ? '' : 's'} ago`
  if (diffDays < 30) return `${diffDays} day${diffDays === 1 ? '' : 's'} ago`

  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
