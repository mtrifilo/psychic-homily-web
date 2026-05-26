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
 * so the value is always parsed as UTC. Originally introduced as a
 * private helper for `formatRelativeTime` (PSY-255); extracted (PSY-780)
 * so the same rule applies to every relative-time formatter in the app.
 */
export function ensureUTC(dateStr: string): Date {
  // Already has a timezone indicator: ends with "Z", or has a +/-HH:MM / +/-HHMM offset
  if (/Z$|[+-]\d{2}:\d{2}$|[+-]\d{4}$/.test(dateStr)) {
    return new Date(dateStr)
  }
  return new Date(dateStr + 'Z')
}
