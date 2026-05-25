'use client'

import { StreamingWorklist } from '@/features/admin/components/StreamingWorklist'

/**
 * /admin/streaming-worklist — admin triage surface for streaming-link
 * discovery. Sits under the admin layout, which delegates auth + IsAdmin
 * gating to `<AdminGuard>` so this page only renders for authenticated
 * admins.
 *
 * Implementation lives at
 * frontend/features/admin/components/StreamingWorklist.
 */
export default function AdminStreamingWorklistPage() {
  return <StreamingWorklist />
}
