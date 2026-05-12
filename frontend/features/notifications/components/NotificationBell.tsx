'use client'

/**
 * Header bell + popover for the in-app notification surface (PSY-595).
 * Mark-read policy matches /notifications: view-clears-count. Opening the
 * popover fires mark-all-read after a 500ms delay so the user has time to
 * register the badge before it clears.
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
