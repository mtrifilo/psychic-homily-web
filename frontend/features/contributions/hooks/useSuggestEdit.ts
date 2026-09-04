import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL, isConflictError } from '@/lib/api'
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
    // A 409 means the server refused because the entity is not in the state the
    // submission described: the field moved since the form was loaded, or this
    // submitter already has an edit queued on it. Both are answered by showing
    // the submitter the CURRENT entity, so the refetch belongs here rather than
    // in each caller, which would otherwise leave the form asserting a previous
    // value the server has already rejected.
    //
    // The DETAIL prefix, not the whole entity namespace onSuccess invalidates:
    // the form reads its previous values from the one entity, and the more
    // common 409 is a duplicate pending edit, on which nothing about the entity
    // changed at all.
    //
    // The prefix and not the exact key, because a detail query is keyed by
    // whatever the page routed on — a slug on every entity page, the numeric id
    // elsewhere — and this hook holds only the id. Matching on the id alone
    // would refresh nothing on exactly the surface the drawer opens from.
    onError: (error, { entityType }) => {
      if (!isConflictError(error)) return
      queryClient.invalidateQueries({ queryKey: [ENTITY_PLURAL[entityType], 'detail'] })
    },
  })
}
