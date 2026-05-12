'use client'

/**
 * NotificationBell — header bell icon with unread-count badge + popover.
 * Lives in the desktop TopBar between the search input and the user-menu
 * avatar. PSY-595.
 *
 * Behaviour:
 *   - Only renders when the user is authenticated.
 *   - Polls /me/notifications every 60s via useUserNotifications.
 *   - Badge shows unread_count, capped at "9+".
 *   - Clicking the bell opens a Radix popover with the most recent rows.
 *   - The popover marks all unread notifications read when it opens (after
 *     a small delay so the user has a moment to register the count) so
 *     the badge clears.
 *   - Each row navigates to its deep link; the popover closes via the
 *     PopoverPrimitive close-on-outside-click behavior.
 *
 * Mark-read policy: matches the inbox page — view-clears-count. A future
 * ticket could move to "click marks that row" if surfacing "still unread"
 * after popover dismiss proves useful.
 */

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Bell } from 'lucide-react'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useUserNotifications, useMarkNotificationsRead } from '../hooks'
import { NotificationList } from './NotificationList'

export function NotificationBell() {
  const { isAuthenticated } = useAuthContext()
  const [open, setOpen] = useState(false)

  const { data, isLoading } = useUserNotifications({ limit: 10 })
  const markRead = useMarkNotificationsRead()

  const unreadCount = data?.unread_count ?? 0
  const entries = data?.notifications ?? []

  // Mark unread rows read once the popover opens. Run async so the user
  // can briefly see the unread badge before it clears.
  useEffect(() => {
    if (!open) return
    if (unreadCount === 0) return
    const id = window.setTimeout(() => {
      markRead.mutate(undefined) // no IDs = mark all
    }, 500)
    return () => window.clearTimeout(id)
  }, [open, unreadCount, markRead])

  if (!isAuthenticated) return null

  const badge =
    unreadCount > 9 ? '9+' : unreadCount > 0 ? String(unreadCount) : null

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="relative hidden cursor-pointer sm:flex"
          aria-label={
            unreadCount > 0
              ? `Notifications (${unreadCount} unread)`
              : 'Notifications'
          }
        >
          <Bell className="h-[1.2rem] w-[1.2rem]" />
          {badge != null && (
            <span
              className="absolute right-1 top-1 flex h-4 min-w-[1rem] items-center justify-center rounded-full bg-primary px-1 text-[10px] font-semibold leading-none text-primary-foreground"
              aria-hidden
            >
              {badge}
            </span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={6}
        className="w-[360px] max-w-[calc(100vw-1rem)] p-0"
      >
        <div className="flex items-center justify-between border-b border-border/50 px-3 py-2.5">
          <p className="text-sm font-semibold">Notifications</p>
          <Link
            href="/notifications"
            onClick={() => setOpen(false)}
            className="text-xs text-muted-foreground transition-colors hover:text-foreground"
          >
            View all
          </Link>
        </div>
        <div className="max-h-[60vh] overflow-y-auto">
          {isLoading ? (
            <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">
              Loading…
            </div>
          ) : (
            <NotificationList
              entries={entries}
              variant="popover"
              onItemClick={() => setOpen(false)}
            />
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
