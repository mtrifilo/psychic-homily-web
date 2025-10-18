import { useQuery } from '@tanstack/react-query'
import { ArtistSearchResponse, ArtistSearchParams } from '@/lib/types/artist'
import { queryKeys } from '../queryClient'
import { API_ENDPOINTS, apiRequest } from '@/lib/api'

export const useArtistSearch = (params: ArtistSearchParams) => {
    return useQuery({
        queryKey: queryKeys.artists.search(params),
        queryFn: async (): Promise<ArtistSearchResponse> => {
            return apiRequest(`${API_ENDPOINTS.ARTISTS.SEARCH}?query=${encodeURIComponent(params.query)}`)
        },
        enabled: params.query.length > 0, // only search when a query is present
        staleTime: 5 * 60 * 1000, // 5 minutes - artist data rarely changes
        gcTime: 30 * 60 * 1000, // 30 minutes - keep in memory longer
    })
}
