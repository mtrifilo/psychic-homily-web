import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import type { EditableEntityType, SuggestEditRequest, SuggestEditResponse } from '../types'

/**
 * Explicit singular → URL plural map for the suggest-edit endpoint and the
 * react-query cache key. `Record<EditableEntityType, string>` makes the map
 * exhaustive: adding a new editable entity (or an entity with an irregular
 * plural) is a compile error here, not a silent 404 at runtime. Show is
 * present for type-completeness even though `EntityEditDrawer` routes show
 * edits to `useShowEdit` instead — see EditableEntityType doc.
 */
const ENTITY_PLURAL: Record<EditableEntityType, string> = {
  artist: 'artists',
  venue: 'venues',
  festival: 'festivals',
  release: 'releases',
  label: 'labels',
  show: 'shows',
}

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
      const pluralType = ENTITY_PLURAL[entityType]
      return apiRequest<SuggestEditResponse>(
        `${API_BASE_URL}/${pluralType}/${entityId}/suggest-edit`,
        {
          method: 'PUT',
          body: JSON.stringify({ changes, summary }),
        }
      )
    },
    onSuccess: (_data, { entityType }) => {
      const pluralType = ENTITY_PLURAL[entityType]
      queryClient.invalidateQueries({ queryKey: [pluralType] })
      queryClient.invalidateQueries({ queryKey: ['my-pending-edits'] })
    },
  })
}
