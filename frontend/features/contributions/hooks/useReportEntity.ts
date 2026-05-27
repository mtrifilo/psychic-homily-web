import { useMutation } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import type { ReportableEntityType } from '../types'

interface ReportEntityInput {
  entityType: ReportableEntityType
  entityId: number
  reportType: string
  details?: string
}

interface ReportEntityResponse {
  success: boolean
  report?: {
    id: number
    entity_type: string
    entity_id: number
    report_type: string
    status: string
  }
}

/**
 * Explicit singular → URL plural map for the report endpoint. Mirrors the
 * canonical pattern from `useSuggestEdit.ts` (PSY-726): an exhaustive
 * `Record<ReportableEntityType, string>` makes adding a new entity — or one
 * with an irregular plural — a compile error here, not a silent 404 at
 * runtime. The previous `entityType + 's'` concatenation only worked by
 * coincidence (every current entity has a regular plural) and was the
 * subject of audit PSY-766.
 */
const REPORT_PLURAL: Record<ReportableEntityType, string> = {
  artist: 'artists',
  venue: 'venues',
  festival: 'festivals',
  show: 'shows',
  comment: 'comments',
  collection: 'collections',
}

/**
 * Per-type URL suffix for the report endpoint. Shows route to
 * `/shows/{id}/entity-report` (the generic entity-report queue introduced
 * after the original `/shows/{id}/report` handler); every other type uses
 * `/report`. Exhaustive map so a new entity must declare its suffix
 * explicitly instead of inheriting the default and silently breaking.
 */
const REPORT_SUFFIX: Record<ReportableEntityType, string> = {
  artist: 'report',
  venue: 'report',
  festival: 'report',
  show: 'entity-report',
  comment: 'report',
  collection: 'report',
}

export const useReportEntity = () => {
  return useMutation({
    mutationFn: async ({
      entityType,
      entityId,
      reportType,
      details,
    }: ReportEntityInput): Promise<ReportEntityResponse> => {
      const pluralType = REPORT_PLURAL[entityType]
      const suffix = REPORT_SUFFIX[entityType]

      return apiRequest<ReportEntityResponse>(
        `${API_BASE_URL}/${pluralType}/${entityId}/${suffix}`,
        {
          method: 'POST',
          body: JSON.stringify({
            report_type: reportType,
            ...(details ? { details } : {}),
          }),
        }
      )
    },
  })
}
