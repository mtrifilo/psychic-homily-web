'use client'

/**
 * /settings/notification-filters: the Custom alerts manager.
 *
 * This is the surface previously routed from the sidebar as
 * "Notifications" at `/settings/notifications`. PSY-595 renamed the
 * sidebar label to "Notification Filters" and moved the route here so the
 * inbox at `/notifications` can own the canonical "Notifications" surface.
 * PSY-1905 renamed the label again, to "Custom alerts", once following an
 * entity became a subscription in its own right and this became the
 * criteria-based path rather than the only one.
 *
 * The PATH deliberately did not follow either rename: it is bookmarked and
 * linked from the notification inbox, the command palette and the side nav,
 * and none of that is worth breaking for a change of words.
 *
 * `/settings/notifications` NO LONGER redirects here. PSY-1905 retargeted it
 * at the account alert matrix in profile settings, because PSY-1896 made that
 * path the "manage" CTA of every artist show-alert email and what that reader
 * wants is the matrix, not this criteria builder. Reaching here is now via the
 * side nav, the command palette, the notification inbox, or the matrix's own
 * custom-alerts row (`app/settings/notifications/page.tsx`).
 */

import { useAuthRouteGuard } from '@/lib/hooks/common/useAuthRouteGuard'
import { Loader2 } from 'lucide-react'
import { FilterList } from '@/features/notifications'

export default function NotificationFiltersPage() {
  const gate = useAuthRouteGuard('redirect')

  // 'redirect' mode has already left for /auth by the time this reads
  // anything but 'loading' or 'ready'.
  if (gate !== 'ready') {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="container max-w-3xl mx-auto px-4 py-6">
      <FilterList />
    </div>
  )
}
