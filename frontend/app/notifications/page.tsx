'use client'

/**
 * /notifications — full inbox page (PSY-595).
 *
 * Sibling surface to the header bell popover (`NotificationBell`).
 * Reuses `useUserNotifications` so the bell + page share one cache entry,
 * and reuses `NotificationList` so row rendering stays identical.
 *
 * Mark-read policy: page mounts → mark all unread read on first paint.
 * That matches the bell (view-clears-count) so once a user has SEEN their
 * notifications on either surface, the badge clears.
 */

import { useEffect } from 'react'
import { redirect } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import {
  NotificationList,
  useMarkNotificationsRead,
  useUserNotifications,
} from '@/features/notifications'
import { InlineErrorBanner } from '@/components/shared/InlineErrorBanner'

export default function NotificationInboxPage() {
  const { isAuthenticated, isLoading: authLoading } = useAuthContext()
  const { data, isLoading, isError, error } = useUserNotifications({ limit: 50 })
  const markRead = useMarkNotificationsRead()

  const unreadCount = data?.unread_count ?? 0
  const entries = data?.notifications ?? []

  // Auto-mark-read once the inbox is open. Same view-clears-count policy
  // as the bell popover. Only fires while there's something to clear so
  // re-mounts on cached data don't re-issue the mutation.
  useEffect(() => {
    if (authLoading || !isAuthenticated) return
    if (unreadCount === 0) return
    markRead.mutate(undefined)
  }, [authLoading, isAuthenticated, unreadCount, markRead])

  if (authLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!isAuthenticated) {
    redirect('/auth')
  }

  return (
    <div className="container mx-auto max-w-3xl px-4 py-6">
      <div className="mb-4 flex items-end justify-between gap-2">
        <div>
          <h1 className="text-2xl font-semibold">Notifications</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Comment replies, mentions, and matches from your alert filters.
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <a href="/settings/notification-filters">Manage filters</a>
        </Button>
      </div>

      {isError ? (
        <InlineErrorBanner variant="queryFallback">
          Couldn&apos;t load your notifications.{' '}
          {error instanceof Error ? error.message : ''}
        </InlineErrorBanner>
      ) : isLoading ? (
        <div className="flex h-40 items-center justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-border/50 bg-card">
          <NotificationList entries={entries} variant="page" />
        </div>
      )}
    </div>
  )
}
