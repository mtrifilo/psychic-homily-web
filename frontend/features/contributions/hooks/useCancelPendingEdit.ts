import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import { refetchAfterMutation } from '@/lib/refetchAfterMutation'

export const useCancelPendingEdit = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (editId: number): Promise<{ success: boolean }> => {
      return apiRequest<{ success: boolean }>(
        `${API_BASE_URL}/my/pending-edits/${editId}`,
        { method: 'DELETE' }
      )
    },
    onSuccess: () => {
      void refetchAfterMutation(queryClient, ['my-pending-edits'])
    },
  })
}
