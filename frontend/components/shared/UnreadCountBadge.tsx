import { cn } from '@/lib/utils'

/**
 * The numeric unread-count badge (Figma 1132:12 State B) — one implementation
 * for every surface that shows "you have N unread notifications": the header
 * bell (NotificationBell) and the mobile bottom tab bar's Account tab + its
 * sheet's Notifications row (PSY-1819).
 *
 * Contract: the badge is DECORATIVE (aria-hidden). The count reaches assistive
 * tech through the host control's accessible name — build it with
 * `withUnreadLabel` so every surface announces it the same way. A surface that
 * renders this without also labelling its control leaves screen-reader users
 * with no unread affordance at all.
 *
 * Positioning belongs to the host, not here: the bell and the tab overlay it on
 * an icon (`absolute`), the sheet row trails it inline (`ml-auto`). Pass that
 * in via `className`.
 */
export interface UnreadCountBadgeProps {
  /** Unread items. Zero or negative renders nothing — callers need no guard. */
  count: number
  /** Host-supplied placement (absolute offsets, inline alignment). */
  className?: string
}

export function UnreadCountBadge({ count, className }: UnreadCountBadgeProps) {
  if (count <= 0) return null

  return (
    <span
      data-testid="notification-unread-badge"
      // ring-2 ring-background carves a crisp gap so the badge reads cleanly
      // over the icon it overlays. Small rounded-rect (radius sm), NOT a pill —
      // Figma 1132:12 State B.
      className={cn(
        'min-w-4 rounded-sm bg-primary px-1 text-center font-mono text-[10px] font-bold leading-4 text-primary-foreground ring-2 ring-background',
        className
      )}
      aria-hidden
    >
      {count}
    </span>
  )
}

/**
 * The accessible name for a control carrying an UnreadCountBadge. Keeps the
 * base label as an exact prefix at zero unread, so name-based queries (tests,
 * e2e locators) that predate the badge keep matching.
 */
export function withUnreadLabel(label: string, count: number): string {
  return count > 0 ? `${label} (${count} unread)` : label
}
