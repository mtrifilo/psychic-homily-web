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
