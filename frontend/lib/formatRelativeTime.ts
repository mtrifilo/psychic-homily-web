import { ensureUTC } from './ensureUTC'

/**
 * Format a UTC timestamp string into a human-friendly relative time.
 *
 * Handles both short-form ("2m ago") and long-form ("2 minutes ago")
 * output via the `short` option (default: false). The long form jumps
 * from "N days ago" straight to an absolute date past 30 days — callers
 * that want weeks/months granularity should use `formatTimeAgo` instead.
 */
export function formatRelativeTime(
  dateStr: string,
  options?: {
    short?: boolean
    /**
     * The instant to measure from. Defaults to the clock. A server-rendered
     * caller passes a value the hydrating client sees too (a query's
     * `dataUpdatedAt`), so both renders produce the same text.
     */
    now?: Date
    /** Zone the short form's absolute-date fallback reads in. Defaults to
     *  the runtime's, which differs between a server and a browser. */
    timeZone?: string
  }
): string {
  const date = ensureUTC(dateStr)
  const now = options?.now ?? new Date()
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

    const timeZone = options?.timeZone
    const year = (d: Date) => d.toLocaleDateString('en-US', { year: 'numeric', timeZone })
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: year(date) !== year(now) ? 'numeric' : undefined,
      timeZone,
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
