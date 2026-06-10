'use client'

/**
 * Contributor Profile Hooks
 *
 * TanStack Query hooks for contributor profile operations:
 * public profiles, own profile, contributions, privacy settings,
 * and custom profile sections.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys, createInvalidateQueries } from '@/lib/queryClient'
import type {
  PublicProfileResponse,
  ContributionsResponse,
  ProfileSectionsResponse,
  ProfileSectionResponse,
  CreateSectionInput,
  UpdateSectionInput,
  UpdateVisibilityInput,
  UpdatePrivacyInput,
  ActivityHeatmapResponse,
  PercentileRankings,
  UserFollowingResponse,
  AttendedShowsResponse,
  UserFieldNotesResponse,
} from '../types'

// ============================================================================
// Public Profile Queries
// ============================================================================

/**
 * Hook to fetch a public contributor profile by username
 */
export function usePublicProfile(username: string) {
  return useQuery({
    queryKey: queryKeys.contributor.profile(username),
    queryFn: async (): Promise<PublicProfileResponse> => {
      return apiRequest<PublicProfileResponse>(
        API_ENDPOINTS.USERS.PROFILE(username),
        { method: 'GET' }
      )
    },
    enabled: Boolean(username),
    staleTime: 5 * 60 * 1000,
  })
}

interface UsePublicContributionsOptions {
  limit?: number
  offset?: number
  entity_type?: string
}

/**
 * Hook to fetch a user's public contribution history
 */
export function usePublicContributions(
  username: string,
  options: UsePublicContributionsOptions = {}
) {
  const { limit = 20, offset = 0, entity_type } = options

  const params = new URLSearchParams()
  params.set('limit', String(limit))
  params.set('offset', String(offset))
  if (entity_type) params.set('entity_type', entity_type)

  const endpoint = `${API_ENDPOINTS.USERS.CONTRIBUTIONS(username)}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.contributor.contributions(username),
    queryFn: async (): Promise<ContributionsResponse> => {
      return apiRequest<ContributionsResponse>(endpoint, { method: 'GET' })
    },
    enabled: Boolean(username),
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to fetch a user's activity heatmap (daily contribution counts for last 365 days)
 */
export function useActivityHeatmap(username: string) {
  return useQuery({
    queryKey: queryKeys.contributor.activityHeatmap(username),
    queryFn: async (): Promise<ActivityHeatmapResponse> => {
      return apiRequest<ActivityHeatmapResponse>(
        API_ENDPOINTS.USERS.ACTIVITY_HEATMAP(username),
        { method: 'GET' }
      )
    },
    enabled: Boolean(username),
    staleTime: 10 * 60 * 1000, // 10 minutes — heatmap data doesn't change often
  })
}

/**
 * Hook to fetch a user's percentile rankings by username
 */
export function usePercentileRankings(username: string) {
  return useQuery({
    queryKey: queryKeys.contributor.rankings(username),
    queryFn: async (): Promise<PercentileRankings> => {
      return apiRequest<PercentileRankings>(
        API_ENDPOINTS.USERS.RANKINGS(username),
        { method: 'GET' }
      )
    },
    enabled: Boolean(username),
    staleTime: 5 * 60 * 1000,
    retry: false, // Don't retry on 404 (rankings not available)
  })
}

// ============================================================================
// Public Profile List Queries (PSY-1046 endpoints, consumed by PSY-1045)
//
// All three are privacy-gated server-side: `hidden` → 404 (retry disabled so
// the section can render its gated state immediately), `count_only` → total
// with an empty list, owner always gets the full list.
// ============================================================================

interface UseUserFollowingOptions {
  type?: 'all' | 'artist' | 'venue' | 'label' | 'festival'
  limit?: number
  offset?: number
}

/**
 * Hook to fetch the entities a user follows (artists / venues / labels /
 * festivals), name+slug enriched.
 */
export function useUserFollowing(
  username: string,
  options: UseUserFollowingOptions = {}
) {
  const { type = 'all', limit = 20, offset = 0 } = options

  const params = new URLSearchParams()
  params.set('type', type)
  params.set('limit', String(limit))
  params.set('offset', String(offset))

  return useQuery({
    queryKey: queryKeys.contributor.following(username, type),
    queryFn: async (): Promise<UserFollowingResponse> => {
      return apiRequest<UserFollowingResponse>(
        `${API_ENDPOINTS.USERS.FOLLOWING(username)}?${params.toString()}`,
        { method: 'GET' }
      )
    },
    enabled: Boolean(username),
    staleTime: 5 * 60 * 1000,
    retry: false, // 404 = hidden by privacy settings — don't hammer it
  })
}

interface UsePaginationOptions {
  limit?: number
  offset?: number
}

/**
 * Hook to fetch a user's concert diary: past approved shows they marked
 * "going", most recent first.
 */
export function useUserAttendedShows(
  username: string,
  options: UsePaginationOptions = {}
) {
  const { limit = 20, offset = 0 } = options

  const params = new URLSearchParams()
  params.set('limit', String(limit))
  params.set('offset', String(offset))

  return useQuery({
    queryKey: queryKeys.contributor.attendedShows(username),
    queryFn: async (): Promise<AttendedShowsResponse> => {
      return apiRequest<AttendedShowsResponse>(
        `${API_ENDPOINTS.USERS.ATTENDED_SHOWS(username)}?${params.toString()}`,
        { method: 'GET' }
      )
    },
    enabled: Boolean(username),
    staleTime: 5 * 60 * 1000,
    retry: false, // 404 = hidden by privacy settings — don't hammer it
  })
}

/**
 * Hook to fetch the visible field notes a user has written (show
 * title/slug enriched), newest first.
 */
export function useUserFieldNotes(
  username: string,
  options: UsePaginationOptions = {}
) {
  const { limit = 20, offset = 0 } = options

  const params = new URLSearchParams()
  params.set('limit', String(limit))
  params.set('offset', String(offset))

  return useQuery({
    queryKey: queryKeys.contributor.fieldNotes(username),
    queryFn: async (): Promise<UserFieldNotesResponse> => {
      return apiRequest<UserFieldNotesResponse>(
        `${API_ENDPOINTS.USERS.FIELD_NOTES(username)}?${params.toString()}`,
        { method: 'GET' }
      )
    },
    enabled: Boolean(username),
    staleTime: 5 * 60 * 1000,
    retry: false,
  })
}

// ============================================================================
// Own Profile Queries
// ============================================================================

/**
 * Hook to fetch the authenticated user's contributor profile
 */
export function useOwnContributorProfile() {
  return useQuery({
    queryKey: queryKeys.contributor.ownProfile,
    queryFn: async (): Promise<PublicProfileResponse> => {
      return apiRequest<PublicProfileResponse>(
        API_ENDPOINTS.CONTRIBUTOR.OWN_PROFILE,
        { method: 'GET' }
      )
    },
    staleTime: 5 * 60 * 1000,
  })
}

interface UseOwnContributionsOptions {
  limit?: number
  offset?: number
  entity_type?: string
}

/**
 * Hook to fetch the authenticated user's contribution history
 */
export function useOwnContributions(
  options: UseOwnContributionsOptions = {}
) {
  const { limit = 20, offset = 0, entity_type } = options

  const params = new URLSearchParams()
  params.set('limit', String(limit))
  params.set('offset', String(offset))
  if (entity_type) params.set('entity_type', entity_type)

  const endpoint = `${API_ENDPOINTS.CONTRIBUTOR.OWN_CONTRIBUTIONS}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.contributor.ownContributions,
    queryFn: async (): Promise<ContributionsResponse> => {
      return apiRequest<ContributionsResponse>(endpoint, { method: 'GET' })
    },
    staleTime: 5 * 60 * 1000,
  })
}

// ============================================================================
// Own Sections Queries
// ============================================================================

/**
 * Hook to fetch the authenticated user's profile sections
 */
export function useOwnSections() {
  return useQuery({
    queryKey: queryKeys.contributor.ownSections,
    queryFn: async (): Promise<ProfileSectionsResponse> => {
      return apiRequest<ProfileSectionsResponse>(
        API_ENDPOINTS.CONTRIBUTOR.OWN_SECTIONS,
        { method: 'GET' }
      )
    },
    staleTime: 5 * 60 * 1000,
  })
}

// ============================================================================
// Mutations
// ============================================================================

/**
 * Hook to update profile visibility (public/private)
 */
export function useUpdateVisibility() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (input: UpdateVisibilityInput): Promise<PublicProfileResponse> => {
      return apiRequest<PublicProfileResponse>(
        API_ENDPOINTS.CONTRIBUTOR.VISIBILITY,
        {
          method: 'PATCH',
          body: JSON.stringify(input),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.ownContributor()
      invalidateQueries.contributor()
    },
  })
}

/**
 * Hook to update granular privacy settings
 */
export function useUpdatePrivacy() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (input: UpdatePrivacyInput): Promise<PublicProfileResponse> => {
      return apiRequest<PublicProfileResponse>(
        API_ENDPOINTS.CONTRIBUTOR.PRIVACY,
        {
          method: 'PATCH',
          body: JSON.stringify(input),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.ownContributor()
      invalidateQueries.contributor()
    },
  })
}

/**
 * Hook to create a new profile section
 */
export function useCreateSection() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (input: CreateSectionInput): Promise<ProfileSectionResponse> => {
      return apiRequest<ProfileSectionResponse>(
        API_ENDPOINTS.CONTRIBUTOR.OWN_SECTIONS,
        {
          method: 'POST',
          body: JSON.stringify(input),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.ownContributor()
      invalidateQueries.contributor()
    },
  })
}

/**
 * Hook to update an existing profile section
 */
export function useUpdateSection() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      sectionId,
      data,
    }: {
      sectionId: number
      data: UpdateSectionInput
    }): Promise<ProfileSectionResponse> => {
      return apiRequest<ProfileSectionResponse>(
        API_ENDPOINTS.CONTRIBUTOR.SECTION(sectionId),
        {
          method: 'PUT',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.ownContributor()
      invalidateQueries.contributor()
    },
  })
}

/**
 * Hook to delete a profile section
 */
export function useDeleteSection() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (sectionId: number): Promise<void> => {
      return apiRequest<void>(
        API_ENDPOINTS.CONTRIBUTOR.SECTION(sectionId),
        { method: 'DELETE' }
      )
    },
    onSuccess: () => {
      invalidateQueries.ownContributor()
      invalidateQueries.contributor()
    },
  })
}
