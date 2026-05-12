/**
 * Legacy redirect: `/settings/notifications` →
 * `/settings/notification-filters`. PSY-595 moved the filter-manager UI
 * to make room for the new `/notifications` inbox. Bookmarks and email
 * footers that point here continue to land on the same surface.
 */

import { redirect } from 'next/navigation'

export default function LegacyNotificationSettingsRedirect() {
  redirect('/settings/notification-filters')
}
