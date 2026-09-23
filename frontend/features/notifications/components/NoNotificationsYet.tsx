'use client'

/**
 * Empty state for a viewer who has never received a notification, with links
 * to the directories where Follow lives.
 *
 * Distinct from NotificationList's own empty line ("You're all caught up"),
 * which is the everything-read state. Callers pick between the two from the
 * list they already hold: no rows at all means this component.
 *
 * Every sentence names delivery the inbox receives: follow-driven show alerts
 * for artists, venues and scenes, plus comment replies and mentions.
 */

import { BracketLink } from '@/components/shared/BracketLink'

/** Directories whose entity pages carry a Follow control. */
const FOLLOW_DESTINATIONS = [
  { label: 'artists', href: '/artists' },
  { label: 'venues', href: '/venues' },
  { label: 'scenes', href: '/scenes' },
] as const

export interface NoNotificationsYetProps {
  /** `page` is the roomy dashed card; `popover` is the terse bell version. */
  variant?: 'page' | 'popover'
  /** Fired alongside navigation when a destination link is clicked. */
  onNavigate?: () => void
}

function FollowDestinationLinks({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <>
      {FOLLOW_DESTINATIONS.map(({ label, href }) => (
        <BracketLink
          key={href}
          label={label}
          href={href}
          onClick={onNavigate}
          className="font-mono text-xs"
        />
      ))}
    </>
  )
}

export function NoNotificationsYet({
  variant = 'page',
  onNavigate,
}: NoNotificationsYetProps) {
  if (variant === 'popover') {
    return (
      <div className="flex h-32 flex-col items-center justify-center gap-2 px-4 text-center">
        <p className="text-[13px] leading-[18px] text-muted-foreground">
          Nothing yet. New shows from artists, venues and scenes you follow land
          here.
        </p>
        <div className="flex items-center gap-2.5">
          <FollowDestinationLinks onNavigate={onNavigate} />
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-border/50 px-4 py-6 text-center">
      <p className="text-sm font-medium text-foreground">
        Nothing has arrived yet.
      </p>
      <p className="text-sm text-muted-foreground">
        New shows from the artists, venues and scenes you follow land here,
        along with replies to your comments and mentions.
      </p>
      <div className="flex flex-wrap items-center justify-center gap-x-2.5 gap-y-1.5">
        <span className="text-[13px] leading-[18px] text-muted-foreground">
          Find something to follow:
        </span>
        <FollowDestinationLinks onNavigate={onNavigate} />
      </div>
    </div>
  )
}
