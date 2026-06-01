'use client'

import { useUnverifiedVenues } from './useAdminVenues'
import { usePendingReports } from './useAdminReports'
import { usePendingArtistReports } from './useAdminArtistReports'
import { usePendingShows } from './useAdminShows'
import { useAdminPendingEdits } from './useAdminPendingEdits'
import { useAdminEntityReports } from './useAdminEntityReports'
import { useAdminPendingComments } from './useAdminComments'

/**
 * The four attention counts the admin navigation surfaces as badges.
 * `moderation` aggregates the three moderation-queue sources (pending edits +
 * entity reports + pending comments) — the same formula the retired admin tab
 * bar used for its moderation badge.
 */
export interface AdminNavCounts {
  moderation: number
  pendingShows: number
  unverifiedVenues: number
  reports: number
}

/**
 * Bundles the queue queries that back the admin-nav badge counts so the global
 * Sidebar (and mobile drawer) can show them without each consumer re-deriving
 * the aggregation. Every underlying query is gated by `enabled` because the
 * Sidebar mounts on every page: pass `enabled: isAdmin && inAdmin` so these
 * admin-only endpoints never fire for non-admins or on public routes (they'd
 * 403 / waste requests otherwise). React Query dedupes by query key, so the
 * desktop Sidebar and mobile drawer share one in-flight fetch per count; an
 * admin page already running the same (unfiltered) query reuses it too. (A page
 * that filters — e.g. the moderation queue by entity_type — uses a different key
 * and runs its own query; the nav badge intentionally keeps the global total.)
 */
export function useAdminNavCounts({ enabled }: { enabled: boolean }): AdminNavCounts {
  const { data: pendingShowsData } = usePendingShows({ enabled })
  const { data: unverifiedVenuesData } = useUnverifiedVenues({ enabled })
  const { data: reportsData } = usePendingReports({ enabled })
  const { data: artistReportsData } = usePendingArtistReports({ enabled })
  const { data: pendingEditsData } = useAdminPendingEdits({ status: 'pending', enabled })
  const { data: entityReportsData } = useAdminEntityReports({ status: 'pending', enabled })
  const { data: pendingCommentsData } = useAdminPendingComments(25, 0, { enabled })

  return {
    moderation:
      (pendingEditsData?.total || 0) +
      (entityReportsData?.total || 0) +
      (pendingCommentsData?.total || 0),
    pendingShows: pendingShowsData?.total || 0,
    unverifiedVenues: unverifiedVenuesData?.total || 0,
    reports: (reportsData?.total || 0) + (artistReportsData?.total || 0),
  }
}
