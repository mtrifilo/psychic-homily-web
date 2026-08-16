'use client'

/**
 * Header bell + popover for the in-app notification surface (PSY-595).
 *
 * Mark-read policy (PSY-1513, reverses PSY-1018's view-clears-count):
 * opening the popover marks NOTHING read. A row is marked read when
 * clicked (scoped mark-read with that row's id), and the explicit
 * [Catch up] affordance clears everything via the mark-all endpoint.
 *
 * The unread affordance is a numeric count badge (Figma 1132:12 State B);
 * the count is also announced via the trigger's aria-label.
 */

import { useState } from 'react'
import { Bell } from 'lucide-react'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { BracketLink } from '@/components/shared/BracketLink'
import {
  UnreadCountBadge,
  withUnreadLabel,
} from '@/components/shared/UnreadCountBadge'
import { useAuthContext } from '@/lib/context/AuthContext'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import {
  NOTIFICATION_BELL_LIMIT,
  useUserNotifications,
  useMarkNotificationsRead,
} from '../hooks'
import type { NotificationLogEntry } from '../types'
import {
  EarlierDivider,
  NotificationList,
  partitionNotificationsByRead,
} from './NotificationList'

export function NotificationBell() {
  const { isAuthenticated } = useAuthContext()
  const [open, setOpen] = useState(false)

  const { data, isLoading } = useUserNotifications({
    limit: NOTIFICATION_BELL_LIMIT,
  })
  const markRead = useMarkNotificationsRead()

  const unreadCount = data?.unread_count ?? 0
  const entries = data?.notifications ?? []
  const { unread, read } = partitionNotificationsByRead(entries)

  if (!isAuthenticated) return null

  const hasUnread = unreadCount > 0

  const handleItemClick = (entry: NotificationLogEntry) => {
    if (entry.read_at == null) {
      markRead.mutate([entry.id])
    }
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          {...replayOnHydrate}
          variant="ghost"
          size="icon"
          // Visibility is parent-controlled (the TopBar account cluster is
          // hidden below sm); this trigger no longer self-hides so all three
          // cluster siblings (+ Submit, bell, avatar) share one strategy.
          className="relative cursor-pointer"
          aria-label={withUnreadLabel('Notifications', unreadCount)}
        >
          <Bell className="h-[1.2rem] w-[1.2rem]" />
          <UnreadCountBadge
            count={unreadCount}
            className="absolute right-0 top-0"
          />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={6}
        className="w-[360px] max-w-[calc(100vw-1rem)] p-0"
      >
        <div className="flex items-center justify-between border-b border-border/50 px-3 py-2.5">
          <p className="text-sm font-semibold">
            Notifications
            {hasUnread && (
              <span className="ml-1.5 font-mono text-xs font-normal text-muted-foreground">
                · {unreadCount} unread
              </span>
            )}
          </p>
          {hasUnread && (
            <BracketLink
              label="Catch up ✓"
              onClick={() => markRead.mutate(undefined)}
              disabled={markRead.isPending}
              className="font-mono text-xs"
              ariaLabel="Catch up — mark all notifications read"
            />
          )}
        </div>
        {markRead.isError && (
          <p className="border-b border-border/50 px-3 py-2 text-xs text-destructive">
            Couldn&apos;t mark read. Try again.
          </p>
        )}
        <div className="max-h-[60vh] overflow-y-auto">
          {isLoading ? (
            <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">
              Loading…
            </div>
          ) : entries.length === 0 ? (
            <NotificationList entries={[]} variant="popover" />
          ) : (
            <>
              {unread.length > 0 && (
                <NotificationList
                  entries={unread}
                  variant="popover"
                  onItemClick={handleItemClick}
                />
              )}
              {read.length > 0 && (
                <>
                  <EarlierDivider className="px-3 pb-1 pt-2" />
                  <NotificationList
                    entries={read}
                    variant="popover"
                    onItemClick={handleItemClick}
                    dimmed
                  />
                </>
              )}
            </>
          )}
        </div>
        <div className="border-t border-border/50 px-3 py-2 text-center">
          <BracketLink
            label={hasUnread ? `View all — ${unreadCount} unread` : 'View all'}
            href="/notifications"
            onClick={() => setOpen(false)}
            className="font-mono text-xs"
          />
        </div>
      </PopoverContent>
    </Popover>
  )
}
