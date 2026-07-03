'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioGuideResponse } from '../types'

/**
 * Fetch the dial-wide ON NOW / UP NEXT guide (PSY-1053): schedule-derived
 * rows for every station with scraped weekly schedules. Stations without
 * schedule data contribute nothing — the hub section renders the honesty
 * line instead of pretending to cover them.
 *
 * Refetches every minute so ON NOW flips to the next slot without a reload.
 * The interval callback must bail in the error state — a function
 * refetchInterval keeps firing on a persistently failing query otherwise
 * (the PSY-1136 infinite-poll class).
 */
export function useRadioGuide({ enabled = true }: { enabled?: boolean } = {}) {
  return useQuery({
    queryKey: radioQueryKeys.guide(),
    queryFn: () =>
      apiRequest<RadioGuideResponse>(radioEndpoints.GUIDE, { method: 'GET' }),
    enabled,
    staleTime: 60 * 1000,
    refetchInterval: query => (query.state.error ? false : 60 * 1000),
  })
}
