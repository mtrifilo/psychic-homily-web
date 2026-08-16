import { cn } from '@/lib/utils'

/**
 * The numeric unread-count badge (Figma 1132:12 State B) — one implementation
 * for every surface that shows "you have N unread things". Today that is the
 * notification surfaces (the header bell, and the mobile bottom tab bar's
 * Account tab + its sheet's Notifications row, PSY-1819), but nothing here
 * knows about notifications.
 *
 * Contract: the badge is DECORATIVE (aria-hidden). The count reaches assistive
 * tech through the host control's accessible name — build it with
 * `withUnreadLabel` so every surface announces it the same way. A surface that
 * renders this without also labelling its control leaves screen-reader users
 * with no unread affordance at all.
 *
 * Deliberately NOT a live region: the count changes on a 60s background poll,
 * so announcing it would interrupt the user at arbitrary moments to read out a
 * number they did not ask for. It is announced when the control is focused.
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
      data-testid="unread-count-badge"
      // ring-2 ring-background carves a crisp gap so the badge reads cleanly
      // over the icon it overlays. Small rounded-rect (radius sm), NOT a pill —
      // Figma 1132:12 State B.
      className={cn(
        // text-[0.625rem] (= 10px at default root), NOT text-[10px]: every
        // other dimension here is rem-based, so a px font size would leave the
        // digits fixed while the chip around them grew with the user's base
        // font size.
        'min-w-4 rounded-sm bg-primary px-1 text-center font-mono text-[0.625rem] font-bold leading-4 text-primary-foreground ring-2 ring-background',
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
 *
 * Passing the result straight through is ALWAYS safe — start there. An
 * icon-only control (the header bell) additionally REQUIRES it: guard that
 * one and it loses its accessible name entirely at zero.
 *
 * Controls with visible text may instead apply
 * `count > 0 ? withUnreadLabel(...) : undefined`, which the nav surfaces do
 * so ~25 unbadged sheet rows don't each carry an aria-label restating their
 * own text. That is tidiness, not correctness — the accessible name is the
 * same either way.
 */
export function withUnreadLabel(label: string, count: number): string {
  return count > 0 ? `${label} (${count} unread)` : label
}
