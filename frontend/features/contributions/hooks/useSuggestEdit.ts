import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import type { EditableEntityType, SuggestEditRequest, SuggestEditResponse } from '../types'

export const useSuggestEdit = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      entityType,
      entityId,
      changes,
      summary,
    }: SuggestEditRequest & {
      entityType: EditableEntityType
      entityId: number
    }): Promise<SuggestEditResponse> => {
      const pluralType = entityType + 's'
      return apiRequest<SuggestEditResponse>(
        `${API_BASE_URL}/${pluralType}/${entityId}/suggest-edit`,
        {
          method: 'PUT',
          body: JSON.stringify({ changes, summary }),
        }
      )
    },
    onSuccess: (_data, { entityType }) => {
      const pluralType = entityType + 's'
      queryClient.invalidateQueries({ queryKey: [pluralType] })
      queryClient.invalidateQueries({ queryKey: ['my-pending-edits'] })
    },
  })
}
