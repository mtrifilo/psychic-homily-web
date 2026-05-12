'use client'

/**
 * /settings/notification-filters — the show-alert filter manager.
 *
 * This is the surface previously routed from the sidebar as
 * "Notifications" at `/settings/notifications`. PSY-595 renamed the
 * sidebar label to "Notification Filters" and moved the route here so the
 * inbox at `/notifications` can own the canonical "Notifications" surface.
 *
 * `/settings/notifications` continues to redirect here so any bookmarks
 * or external links stay live (`app/settings/notifications/page.tsx`).
 */

import { useAuthContext } from '@/lib/context/AuthContext'
import { redirect } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { FilterList } from '@/features/notifications'

export default function NotificationFiltersPage() {
  const { isAuthenticated, isLoading } = useAuthContext()

  if (isLoading) {
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
    <div className="container max-w-3xl mx-auto px-4 py-6">
      <FilterList />
    </div>
  )
}
