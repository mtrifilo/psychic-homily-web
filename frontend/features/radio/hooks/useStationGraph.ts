'use client'

/**
 * useStationGraph (PSY-1299)
 *
 * Fetches the within-station co-occurrence subgraph for the station page.
 * Server owns all shaping: top-N node cap, backbone edge filtering
 * (PSY-1295), show-based clusters, and derived is_isolate /
 * is_cross_cluster flags. Defaults-only per the ticket — the endpoint's
 * `window` / `limit` params are not exposed in the UI yet.
 */

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioStationGraphResponse } from '../types'

interface UseStationGraphOptions {
  slug: string
  enabled?: boolean
}

export function useStationGraph({ slug, enabled = true }: UseStationGraphOptions) {
  return useQuery({
    queryKey: radioQueryKeys.stationGraph(slug),
    queryFn: () =>
      apiRequest<RadioStationGraphResponse>(radioEndpoints.STATION_GRAPH(slug), {
        method: 'GET',
      }),
    enabled: enabled && Boolean(slug),
    staleTime: 5 * 60 * 1000, // 5 minutes — matches useSceneGraph
  })
}
