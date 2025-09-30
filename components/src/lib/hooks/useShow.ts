import { useMutation, useQueryClient } from '@tanstack/react-query'
import { API_ENDPOINTS, apiRequest } from '../api'

interface Artist {
    name: string
    id?: string
    is_headliner?: boolean
}

interface Venue {
    name: string
    id?: string
    city: string
    state: string
    address?: string
}

interface ShowSubmission {
    title?: string // Title is now optional
    event_date: string // UTC timestamp in ISO 8601 format
    city: string
    state: string
    price?: number
    age_requirement?: string
    description?: string // Description is also optional
    venues: Venue[]
    artists: Artist[]
}

export const useShow = () => {
    const queryClient = useQueryClient()

    return useMutation({
        mutationFn: async (showSubmission: ShowSubmission) => {
            return apiRequest(API_ENDPOINTS.SHOWS.SUBMIT, {
                method: 'POST',
                body: JSON.stringify(showSubmission),
            })
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['shows'] })
        },
        onError: (error) => {
            console.error('Error creating show', error)
            queryClient.invalidateQueries({ queryKey: ['shows'] })
        },
    })
}
