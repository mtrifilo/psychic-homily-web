import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import type { FieldChange, SuggestEditResponse } from '../types'

interface ShowEditPayload {
  entityId: number
  changes: FieldChange[]
  summary: string
}

/**
 * Show edit adapter for {@link EntityEditDrawer}. Mirrors `useSuggestEdit`
 * shape (mutate / isPending / isError / data.applied) but dispatches to
 * the show direct-save endpoint (PUT /shows/{id}) — NOT the suggest-edit
 * pipeline. PSY-461 / PSY-489 deliberately scoped show edits to admins
 * and submitters only; PSY-563 wired Summary persistence + RecordRevision
 * on that path so the History accordion can render meaningful entries.
 *
 * Lives in `contributions/hooks/` to avoid a circular dependency between
 * the contributions and shows feature modules — the drawer still owns
 * the dispatch shape, and we hit the same endpoint `useShowUpdate` uses.
 */
export const useShowEdit = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      entityId,
      changes,
      summary,
    }: ShowEditPayload): Promise<SuggestEditResponse> => {
      // Translate the drawer's per-field FieldChange[] into the show
      // update body shape. Only fields that changed are included.
      const body: Record<string, unknown> = {}
      for (const change of changes) {
        // The drawer normalizes blanked values to null; the show
        // endpoint accepts empty string to clear text fields and
        // null/empty for image_url (handled by utils.NilIfEmpty
        // backend-side). Send empty string when the user cleared a
        // field so the field-vs-omitted distinction stays meaningful.
        body[change.field] = change.new_value ?? ''
      }
      body.summary = summary

      await apiRequest<unknown>(`${API_BASE_URL}/shows/${entityId}`, {
        method: 'PUT',
        body: JSON.stringify(body),
      })

      // Show edits ALWAYS apply directly — there is no pending-review
      // branch (PSY-461 / PSY-489). Return a SuggestEditResponse-shaped
      // object so the drawer's UI logic doesn't need to fork.
      return {
        applied: true,
        message: 'Changes saved',
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['shows'] })
    },
  })
}
