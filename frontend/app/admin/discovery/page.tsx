'use client'

import { DiscoveryTriage } from '@/features/admin/components/DiscoveryTriage'

/**
 * /admin/discovery — admin triage queue for the bulk-backfill music-link
 * suggestions (PSY-1207). Sits under the admin layout, which delegates auth
 * + IsAdmin gating to `<AdminGuard>` so this page only renders for
 * authenticated admins.
 *
 * Implementation lives at
 * frontend/features/admin/components/DiscoveryTriage.
 */
export default function AdminDiscoveryPage() {
  return <DiscoveryTriage />
}
