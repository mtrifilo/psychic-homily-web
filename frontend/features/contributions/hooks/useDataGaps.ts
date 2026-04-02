import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import type { EditableEntityType } from '../types'

export interface DataGap {
  field: string
  label: string
  priority: number
}

interface DataGapsResponse {
  gaps: DataGap[]
}

/**
 * Fetches data gaps (missing fields) for an entity.
 * Only enabled when the user is authenticated.
 */
export function useDataGaps(
  entityType: EditableEntityType,
  entitySlug: string,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: ['entities', entityType, entitySlug, 'data-gaps'],
    queryFn: () =>
      apiRequest<DataGapsResponse>(
        `${API_BASE_URL}/entities/${entityType}/${entitySlug}/data-gaps`
      ),
    enabled: options?.enabled !== false && !!entitySlug,
    staleTime: 10 * 60 * 1000, // 10 minutes — gaps don't change often
  })
}
