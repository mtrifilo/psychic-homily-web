'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { artistEndpoints } from '@/features/artists/api'
import type { MyArtistReportResponse } from '../types'

/**
 * The caller's own PENDING report for an artist, or `{ report: null }` when
 * they have none.
 *
 * Reads `entity_reports` — the same table `useReportEntity` writes to. Those
 * two used to disagree: submissions went to `entity_reports` while this read
 * searched `artist_reports`, so a user who had just reported an artist was
 * told they had not (PSY-1633).
 *
 * Pending-only, matching the backend's duplicate rule: a resolved or dismissed
 * report no longer blocks a new submission, so surfacing one would disable an
 * action the API would accept.
 *
 * Submitting is NOT part of this hook — that is `useReportEntity`, which posts
 * to the same entity pipeline every other entity type uses. The two are not
 * wired together, so a surface that both submits and renders this state must
 * invalidate `queryKeys.artistReports.myReport` itself; otherwise the 30s
 * staleTime shows the pre-submit answer.
 */
export const useMyArtistReport = (artistId: number | string | null) => {
  return useQuery({
    queryKey: queryKeys.artistReports.myReport(String(artistId)),
    queryFn: async (): Promise<MyArtistReportResponse> => {
      return apiRequest<MyArtistReportResponse>(
        artistEndpoints.MY_REPORT(artistId!),
        {
          method: 'GET',
        }
      )
    },
    enabled: Boolean(artistId),
    staleTime: 30 * 1000, // 30 seconds
  })
}
