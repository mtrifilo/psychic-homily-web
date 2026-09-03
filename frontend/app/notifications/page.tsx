'use client'

/**
 * /notifications — full inbox page (PSY-595).
 *
 * Sibling surface to the header bell popover (`NotificationBell`).
 * Reuses `useUserNotifications` and `NotificationList` so fetching and row
 * rendering stay identical. It does NOT share the bell's cache entry: the
 * query key is parameterized by {limit, offset} and this page deliberately
 * asks for 50 rows, so it is its own entry with its own poll. That is the
 * intended trade for a full inbox — see NOTIFICATION_LOG_SHARED_LIMIT, which
 * every surface that only wants the COUNT must stay on.
 *
 * Mark-read policy (PSY-1513, reverses PSY-1018's view-clears-count):
 * mounting this page marks NOTHING read. A row is marked read when
 * clicked (or via its [mark read] affordance); the explicit [Catch up]
 * affordance clears everything via the mark-all endpoint.
 *
 * Layout per Figma 1132:12 States A (light) / E (dark): unread-first
 * workbench with an [unread]/[all] view toggle. The default unread view
 * shows unread rows in a card followed by a dimmed "EARLIER" section of
 * already-read rows; [all] shows the interleaved history.
 */

import { useState } from 'react'
import { Loader2 } from 'lucide-react'
import { useAuthRouteGuard } from '@/lib/hooks/common/useAuthRouteGuard'
import { cn } from '@/lib/utils'
import { BracketLink } from '@/components/shared/BracketLink'
import {
  EarlierDivider,
  NotificationList,
  partitionNotificationsByRead,
  useMarkNotificationsRead,
  useUserNotifications,
} from '@/features/notifications'
import type { NotificationLogEntry } from '@/features/notifications'
import { InlineErrorBanner } from '@/components/shared/InlineErrorBanner'

type InboxView = 'unread' | 'all'

export default function NotificationInboxPage() {
  const gate = useAuthRouteGuard('redirect')
  const { data, isLoading, isError, error } = useUserNotifications({ limit: 50 })
  const markRead = useMarkNotificationsRead()
  const [view, setView] = useState<InboxView>('unread')

  const unreadCount = data?.unread_count ?? 0
  const entries = data?.notifications ?? []
  const { unread, read } = partitionNotificationsByRead(entries)

  // Only 'loading' and 'ready' reach here: 'redirect' mode throws.
  if (gate !== 'ready') {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  const handleMarkRowRead = (entry: NotificationLogEntry) => {
    if (entry.read_at == null) {
      markRead.mutate([entry.id])
    }
  }

  return (
    <div className="container mx-auto max-w-3xl px-4 py-6">
      <div className="mb-4 flex items-end justify-between gap-2">
        <div>
          <h1 className="text-2xl font-semibold">
            Notifications
            {unreadCount > 0 && (
              <span className="ml-2 align-middle font-mono text-xs font-normal text-primary">
                {unreadCount} unread
              </span>
            )}
          </h1>
          {/* Names what actually lands here. PSY-1896 put artist show alerts
              in this inbox, so an inventory that lists only custom alerts now
              omits the follow-driven alert type that works. */}
          <p className="mt-1 text-sm text-muted-foreground">
            Comment replies, mentions, shows from artists and scenes you follow,
            and matches from your custom alerts.
          </p>
        </div>
        <BracketLink
          label="Custom alerts"
          href="/settings/notification-filters"
          className="font-mono text-xs"
        />
      </div>

      <div className="mb-4 flex items-center justify-between gap-2 border-y border-border/50 py-2">
        <div className="flex items-center gap-2">
          <BracketLink
            label="unread"
            onClick={() => setView('unread')}
            active={view === 'unread'}
            className={cn('font-mono text-xs', view === 'unread' && 'text-primary')}
            ariaLabel="Show unread notifications first"
          />
          <BracketLink
            label="all"
            onClick={() => setView('all')}
            active={view === 'all'}
            className={cn('font-mono text-xs', view === 'all' && 'text-primary')}
            ariaLabel="Show all notifications interleaved"
          />
        </div>
        {unreadCount > 0 && (
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
        <div className="mb-4">
          <InlineErrorBanner>
            Couldn&apos;t mark notifications read.{' '}
            {markRead.error instanceof Error ? markRead.error.message : ''}
          </InlineErrorBanner>
        </div>
      )}

      {isError ? (
        <InlineErrorBanner variant="queryFallback">
          Couldn&apos;t load your notifications.{' '}
          {error instanceof Error ? error.message : ''}
        </InlineErrorBanner>
      ) : isLoading ? (
        <div className="flex h-40 items-center justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      ) : view === 'all' ? (
        <div className="overflow-hidden rounded-lg border border-border/50 bg-card">
          <NotificationList
            entries={entries}
            variant="page"
            onItemClick={handleMarkRowRead}
            onMarkRead={handleMarkRowRead}
          />
        </div>
      ) : (
        <>
          {unread.length > 0 ? (
            <div className="overflow-hidden rounded-lg border border-border/50 bg-card">
              <NotificationList
                entries={unread}
                variant="page"
                onItemClick={handleMarkRowRead}
                onMarkRead={handleMarkRowRead}
              />
            </div>
          ) : (
            // The page-variant empty state carries its own dashed framing —
            // render it outside the card so it isn't double-bordered.
            <NotificationList entries={[]} variant="page" />
          )}
          {read.length > 0 && (
            <>
              <EarlierDivider className="mb-2 mt-6" />
              <div className="overflow-hidden rounded-lg border border-border/50 bg-card">
                <NotificationList
                  entries={read}
                  variant="page"
                  onItemClick={handleMarkRowRead}
                  dimmed
                />
              </div>
            </>
          )}
        </>
      )}
    </div>
  )
}
