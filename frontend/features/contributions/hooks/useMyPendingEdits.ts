import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import type { PendingEditResponse } from '../types'

interface MyPendingEditsResponse {
  edits: PendingEditResponse[]
  total: number
}

export const useMyPendingEdits = (limit = 20, offset = 0) => {
  return useQuery({
    queryKey: ['my-pending-edits', limit, offset],
    queryFn: () =>
      apiRequest<MyPendingEditsResponse>(
        `${API_BASE_URL}/my/pending-edits?limit=${limit}&offset=${offset}`
      ),
  })
}
