'use client'

/**
 * Community radio-play match suggestions.
 *
 * Authenticated users suggest an existing artist for an unmatched play.
 * Creates a pending queue row only — never mutates radio_plays until an
 * admin accepts via the admin match-suggestions endpoints.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'

export interface RadioPlayMatchSuggestion {
  id: number
  play_id: number
  play_artist_name: string
  play_match_state: string
  suggested_artist_id: number
  suggested_artist_name: string
  suggested_artist_slug?: string | null
  submitted_by: number
  submitter_username?: string | null
  note?: string | null
  status: string
  reviewed_by?: number | null
  reviewed_at?: string | null
  rejection_reason?: string | null
  created_at: string
}

export interface CreatePlayMatchSuggestionInput {
  playId: number
  artistId: number
  note?: string
}

const MATCH_SUGGESTION_ENDPOINTS = {
  CREATE: (playId: number) =>
    `${API_BASE_URL}/radio/plays/${playId}/match-suggestions`,
  MINE: (playId: number) =>
    `${API_BASE_URL}/radio/plays/${playId}/match-suggestions/mine`,
} as const

export const playMatchSuggestionQueryKeys = {
  mine: (playId: number) =>
    ['radio', 'plays', playId, 'match-suggestions', 'mine'] as const,
}

/**
 * Caller's pending suggestion for a play, if any. Disabled when the user is
 * not authenticated or the play is already matched (caller gates `enabled`).
 */
export function useOwnPlayMatchSuggestion(playId: number, enabled: boolean) {
  return useQuery({
    queryKey: playMatchSuggestionQueryKeys.mine(playId),
    queryFn: async (): Promise<RadioPlayMatchSuggestion | null> => {
      const body = await apiRequest<{ suggestion: RadioPlayMatchSuggestion | null }>(
        MATCH_SUGGESTION_ENDPOINTS.MINE(playId)
      )
      return body.suggestion ?? null
    },
    enabled: enabled && playId > 0,
    staleTime: 30_000,
  })
}

/**
 * Submit a community match suggestion. Seeds the caller's pending cache so
 * the playlist row flips to "suggestion pending" without claiming a match.
 */
export function useCreatePlayMatchSuggestion() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      playId,
      artistId,
      note,
    }: CreatePlayMatchSuggestionInput): Promise<RadioPlayMatchSuggestion> => {
      const trimmed = note?.trim()
      return apiRequest<RadioPlayMatchSuggestion>(
        MATCH_SUGGESTION_ENDPOINTS.CREATE(playId),
        {
          method: 'POST',
          body: JSON.stringify({
            artist_id: artistId,
            ...(trimmed ? { note: trimmed } : {}),
          }),
        }
      )
    },
    onSuccess: (entry) => {
      // Seed the mine cache so the row flips to "suggestion pending"
      // immediately without waiting on a refetch.
      queryClient.setQueryData(
        playMatchSuggestionQueryKeys.mine(entry.play_id),
        entry
      )
    },
  })
}
