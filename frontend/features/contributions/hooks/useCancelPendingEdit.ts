import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'

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
      queryClient.invalidateQueries({ queryKey: ['my-pending-edits'] })
    },
  })
}
