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

export const useReportEntity = () => {
  return useMutation({
    mutationFn: async ({
      entityType,
      entityId,
      reportType,
      details,
    }: ReportEntityInput): Promise<ReportEntityResponse> => {
      // Comments use /comments/{id}/report
      if (entityType === 'comment') {
        return apiRequest<ReportEntityResponse>(
          `${API_BASE_URL}/comments/${entityId}/report`,
          {
            method: 'POST',
            body: JSON.stringify({
              report_type: reportType,
              ...(details ? { details } : {}),
            }),
          }
        )
      }

      // Shows use /entity-report instead of /report
      const reportPath = entityType === 'show' ? 'entity-report' : 'report'
      const pluralType = entityType + 's'

      return apiRequest<ReportEntityResponse>(
        `${API_BASE_URL}/${pluralType}/${entityId}/${reportPath}`,
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
