import { ensureUTC } from './ensureUTC'

/**
 * Format a UTC timestamp string as a long-form relative time descriptor.
 *
 * Granularity goes: just now → minutes → hours → days → weeks → months →
 * absolute date (past ~12 months). The week branch caps at 5 weeks before
 * handing off to months, so "1 month ago" appears starting at 35 days.
 *
 * Sibling of `formatRelativeTime`:
 *  - `formatRelativeTime` (default) skips weeks/months and jumps from days
 *    straight to an absolute date past 30 days — fine for compact bylines.
 *  - `formatTimeAgo` is the right pick when you want the richer
 *    weeks/months phrasing (e.g. notification feeds, request lists).
 *
 * Consolidated PSY-780: previously duplicated in `features/notifications/types.ts`
 * (no month branch) and `features/requests/types.ts` (with months). The drift
 * was accidental — every surface that wants relative time should get the same
 * weeks/months breakdown.
 */
export function formatTimeAgo(dateString: string): string {
  const date = ensureUTC(dateString)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSeconds = Math.floor(diffMs / 1000)
  const diffMinutes = Math.floor(diffSeconds / 60)
  const diffHours = Math.floor(diffMinutes / 60)
  const diffDays = Math.floor(diffHours / 24)
  const diffWeeks = Math.floor(diffDays / 7)
  const diffMonths = Math.floor(diffDays / 30)

  if (diffSeconds < 60) return 'just now'
  if (diffMinutes === 1) return '1 minute ago'
  if (diffMinutes < 60) return `${diffMinutes} minutes ago`
  if (diffHours === 1) return '1 hour ago'
  if (diffHours < 24) return `${diffHours} hours ago`
  if (diffDays === 1) return '1 day ago'
  if (diffDays < 7) return `${diffDays} days ago`
  if (diffWeeks === 1) return '1 week ago'
  if (diffWeeks < 5) return `${diffWeeks} weeks ago`
  if (diffMonths === 1) return '1 month ago'
  if (diffMonths < 12) return `${diffMonths} months ago`
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}
