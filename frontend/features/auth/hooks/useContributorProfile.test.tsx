import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import {
  createWrapper,
  createWrapperWithClient,
  createTestQueryClient,
} from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateOwnContributor = vi.fn()
const mockInvalidateContributor = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    USERS: {
      PROFILE: (username: string) => `/users/${username}`,
      CONTRIBUTIONS: (username: string) => `/users/${username}/contributions`,
      ACTIVITY_HEATMAP: (username: string) => `/users/${username}/activity-heatmap`,
      RANKINGS: (username: string) => `/users/${username}/rankings`,
    },
    CONTRIBUTOR: {
      OWN_PROFILE: '/auth/profile/contributor',
      OWN_CONTRIBUTIONS: '/auth/profile/contributions',
      ADVANCEMENT: '/auth/profile/advancement',
      VISIBILITY: '/auth/profile/visibility',
      PRIVACY: '/auth/profile/privacy',
      OWN_SECTIONS: '/auth/profile/sections',
      SECTION: (sectionId: number) => `/auth/profile/sections/${sectionId}`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    contributor: {
      profile: (username: string) => ['contributor', 'profile', username],
      ownProfile: ['contributor', 'ownProfile'],
      advancement: ['contributor', 'advancement'],
      contributions: (username: string) => ['contributor', 'contributions', username],
      ownContributions: ['contributor', 'ownContributions'],
      ownSections: ['contributor', 'ownSections'],
      activityHeatmap: (username: string) => ['contributor', 'activityHeatmap', username],
      rankings: (username: string) => ['contributor', 'rankings', username],
    },
  },
  createInvalidateQueries: () => ({
    ownContributor: mockInvalidateOwnContributor,
    contributor: mockInvalidateContributor,
  }),
}))

// Import hooks after mocks are set up
import {
  usePublicProfile,
  usePublicContributions,
  useActivityHeatmap,
  usePercentileRankings,
  useOwnContributorProfile,
  useOwnContributions,
  useOwnSections,
  useAdvancementProgress,
  useUpdateVisibility,
  useUpdatePrivacy,
  useCreateSection,
  useUpdateSection,
  useDeleteSection,
} from './useContributorProfile'

// ============================================================================
// Public Profile Queries
// ============================================================================

describe('usePublicProfile', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches a public profile by username', async () => {
    const mockProfile = {
      username: 'testuser',
      bio: 'Music enthusiast',
      profile_visibility: 'public',
      user_tier: 'contributor',
      joined_at: '2024-01-15T00:00:00Z',
      stats: {
        shows_submitted: 42,
        venues_submitted: 5,
        total_contributions: 50,
      },
    }
    mockApiRequest.mockResolvedValueOnce(mockProfile)

    const { result } = renderHook(() => usePublicProfile('testuser'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/users/testuser', {
      method: 'GET',
    })
  })

  it('does not fetch when username is empty', () => {
    const { result } = renderHook(() => usePublicProfile(''), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('handles user not found error', async () => {
    const error = new Error('User not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => usePublicProfile('nonexistent'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('User not found')
  })

  it('isPending is true while the initial fetch is in flight', () => {
    // Hang the request so we can observe loading state.
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))

    const { result } = renderHook(() => usePublicProfile('testuser'), {
      wrapper: createWrapper(),
    })

    expect(result.current.isPending).toBe(true)
    expect(result.current.isSuccess).toBe(false)
  })

  it('caches profile under the username key and skips refetch on remount', async () => {
    // Profile pages link to each other — if the cache key doesn't match
    // the username, navigating user A → user B → user A would re-fetch
    // every time. Verify the queryKey is stable per username and the
    // staleTime=5min window prevents the second call.
    const queryClient = createTestQueryClient()
    const profile = {
      username: 'testuser',
      bio: 'bio',
      profile_visibility: 'public',
      user_tier: 'contributor',
      joined_at: '2024-01-01T00:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(profile)

    const { result, unmount } = renderHook(
      () => usePublicProfile('testuser'),
      { wrapper: createWrapperWithClient(queryClient) }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledTimes(1)

    unmount()

    // Remount with the same client + same username — should hit cache.
    const { result: result2 } = renderHook(
      () => usePublicProfile('testuser'),
      { wrapper: createWrapperWithClient(queryClient) }
    )

    expect(result2.current.data).toEqual(profile)
    expect(mockApiRequest).toHaveBeenCalledTimes(1)
  })

  it('issues a separate fetch for a different username', async () => {
    // Companion to the cache test above: distinct usernames must NOT share
    // a cache entry.
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValueOnce({
      username: 'a',
      bio: '',
      profile_visibility: 'public',
      user_tier: 'contributor',
      joined_at: '2024-01-01T00:00:00Z',
    })

    const { result } = renderHook(() => usePublicProfile('a'), {
      wrapper: createWrapperWithClient(queryClient),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    mockApiRequest.mockResolvedValueOnce({
      username: 'b',
      bio: '',
      profile_visibility: 'public',
      user_tier: 'contributor',
      joined_at: '2024-01-01T00:00:00Z',
    })

    const { result: result2 } = renderHook(() => usePublicProfile('b'), {
      wrapper: createWrapperWithClient(queryClient),
    })
    await waitFor(() => expect(result2.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledTimes(2)
    expect(mockApiRequest.mock.calls[0][0]).toBe('/users/a')
    expect(mockApiRequest.mock.calls[1][0]).toBe('/users/b')
  })
})

describe('useActivityHeatmap', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the activity heatmap for a username', async () => {
    const heatmap = {
      days: [
        { date: '2025-01-01', count: 3 },
        { date: '2025-01-02', count: 0 },
      ],
      total_contributions: 3,
      max_day_count: 3,
    }
    mockApiRequest.mockResolvedValueOnce(heatmap)

    const { result } = renderHook(() => useActivityHeatmap('testuser'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/users/testuser/activity-heatmap', {
      method: 'GET',
    })
    expect(result.current.data).toEqual(heatmap)
  })

  it('does not fetch when username is empty', () => {
    const { result } = renderHook(() => useActivityHeatmap(''), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('surfaces server errors instead of returning empty heatmap', async () => {
    // The contributor profile page renders a year-long grid; if errors
    // were swallowed, the user would see "0 contributions" for every day
    // when the backend was actually down.
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useActivityHeatmap('testuser'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.data).toBeUndefined()
    expect((result.current.error as Error).message).toBe('Server error')
  })
})

describe('usePercentileRankings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches percentile rankings for a username', async () => {
    const rankings = {
      shows_submitted: { value: 42, percentile: 87 },
      venues_submitted: { value: 5, percentile: 50 },
      total_contributions: { value: 50, percentile: 80 },
    }
    mockApiRequest.mockResolvedValueOnce(rankings)

    const { result } = renderHook(() => usePercentileRankings('testuser'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/users/testuser/rankings', {
      method: 'GET',
    })
    expect(result.current.data).toEqual(rankings)
  })

  it('does not fetch when username is empty', () => {
    const { result } = renderHook(() => usePercentileRankings(''), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does NOT retry on 404 (rankings unavailable)', async () => {
    // The hook deliberately disables retry to surface the "not available"
    // state fast. Verify a single attempt regardless of retry settings.
    const error = new Error('Not Found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValue(error)

    const { result } = renderHook(() => usePercentileRankings('testuser'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledTimes(1)
  })
})

describe('usePublicContributions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches contributions with default options', async () => {
    const mockResponse = {
      contributions: [
        {
          id: 1,
          action: 'created',
          entity_type: 'show',
          entity_id: 100,
          entity_name: 'Cool Show',
          created_at: '2025-03-01T00:00:00Z',
          source: 'web',
        },
      ],
      total: 1,
      limit: 20,
      offset: 0,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(
      () => usePublicContributions('testuser'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('/users/testuser/contributions')
    expect(calledUrl).toContain('limit=20')
    expect(calledUrl).toContain('offset=0')
  })

  it('passes custom limit and offset', async () => {
    mockApiRequest.mockResolvedValueOnce({
      contributions: [],
      total: 0,
      limit: 10,
      offset: 20,
    })

    const { result } = renderHook(
      () => usePublicContributions('testuser', { limit: 10, offset: 20 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('limit=10')
    expect(calledUrl).toContain('offset=20')
  })

  it('passes entity_type filter', async () => {
    mockApiRequest.mockResolvedValueOnce({
      contributions: [],
      total: 0,
      limit: 20,
      offset: 0,
    })

    const { result } = renderHook(
      () =>
        usePublicContributions('testuser', { entity_type: 'show' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('entity_type=show')
  })

  it('does not fetch when username is empty', () => {
    const { result } = renderHook(
      () => usePublicContributions(''),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

})

// ============================================================================
// Own Profile Queries
// ============================================================================

describe('useOwnContributorProfile', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the authenticated user\'s contributor profile', async () => {
    const mockProfile = {
      username: 'myuser',
      bio: 'My bio',
      profile_visibility: 'public',
      user_tier: 'trusted_contributor',
      joined_at: '2024-06-01T00:00:00Z',
      privacy_settings: {
        contributions: 'visible',
        saved_shows: 'count_only',
        following: 'hidden',
        collections: 'visible',
        last_active: 'visible',
        profile_sections: 'visible',
      },
    }
    mockApiRequest.mockResolvedValueOnce(mockProfile)

    const { result } = renderHook(() => useOwnContributorProfile(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/auth/profile/contributor', {
      method: 'GET',
    })
  })

  it('surfaces auth errors (401) to the caller', async () => {
    // The "Your profile" panel must distinguish "logged out" from "loaded
    // but empty" — silently treating 401 as undefined would render a
    // skeleton forever.
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useOwnContributorProfile(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.data).toBeUndefined()
    expect((result.current.error as Error).message).toBe('Unauthorized')
  })
})

describe('useAdvancementProgress', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches GET /auth/profile/advancement', async () => {
    const mockProgress = {
      current_tier: 'trusted_contributor',
      next_tier: 'local_ambassador',
      requirements: [
        { requirement: 'approved_edits', current: 32, threshold: 50, met: false },
      ],
    }
    mockApiRequest.mockResolvedValueOnce(mockProgress)

    const { result } = renderHook(() => useAdvancementProgress(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/auth/profile/advancement', {
      method: 'GET',
    })
    expect(result.current.data?.requirements[0].current).toBe(32)
  })

  it('skips the request when enabled=false', async () => {
    const { result } = renderHook(() => useAdvancementProgress(false), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })
})

describe('useOwnContributions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches own contributions with default options', async () => {
    const mockResponse = {
      contributions: [
        {
          id: 1,
          action: 'created',
          entity_type: 'venue',
          entity_id: 10,
          entity_name: 'New Venue',
          created_at: '2025-02-01T00:00:00Z',
          source: 'web',
        },
      ],
      total: 1,
      limit: 20,
      offset: 0,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useOwnContributions(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('/auth/profile/contributions')
    expect(calledUrl).toContain('limit=20')
    expect(calledUrl).toContain('offset=0')
  })

  it('passes entity_type filter', async () => {
    mockApiRequest.mockResolvedValueOnce({
      contributions: [],
      total: 0,
      limit: 20,
      offset: 0,
    })

    const { result } = renderHook(
      () => useOwnContributions({ entity_type: 'artist' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('entity_type=artist')
  })
})

describe('useOwnSections', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches own profile sections', async () => {
    const mockResponse = {
      sections: [
        {
          id: 1,
          title: 'About Me',
          content: 'I love live music',
          position: 0,
          is_visible: true,
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
        {
          id: 2,
          title: 'Favorite Genres',
          content: 'Punk, post-punk, shoegaze',
          position: 1,
          is_visible: true,
          created_at: '2025-01-02T00:00:00Z',
          updated_at: '2025-01-02T00:00:00Z',
        },
      ],
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useOwnSections(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/auth/profile/sections', {
      method: 'GET',
    })
  })
})

// ============================================================================
// Mutations
// ============================================================================

describe('useUpdateVisibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateOwnContributor.mockReset()
    mockInvalidateContributor.mockReset()
  })

  it('updates profile visibility and invalidates queries', async () => {
    const mockResponse = {
      username: 'testuser',
      profile_visibility: 'private',
      user_tier: 'contributor',
      joined_at: '2024-01-01T00:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useUpdateVisibility(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync({ visibility: 'private' })
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/auth/profile/visibility', {
      method: 'PATCH',
      body: JSON.stringify({ visibility: 'private' }),
    })
    expect(mockInvalidateOwnContributor).toHaveBeenCalled()
    expect(mockInvalidateContributor).toHaveBeenCalled()
  })

  it('handles update errors', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useUpdateVisibility(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({ visibility: 'public' })
      } catch (e) {
        expect((e as Error).message).toBe('Forbidden')
      }
    })

    expect(mockInvalidateOwnContributor).not.toHaveBeenCalled()
    expect(mockInvalidateContributor).not.toHaveBeenCalled()
  })
})

describe('useUpdatePrivacy', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateOwnContributor.mockReset()
    mockInvalidateContributor.mockReset()
  })

  it('updates privacy settings and invalidates queries', async () => {
    const mockResponse = {
      username: 'testuser',
      profile_visibility: 'public',
      user_tier: 'contributor',
      joined_at: '2024-01-01T00:00:00Z',
      privacy_settings: {
        contributions: 'visible',
        saved_shows: 'hidden',
        following: 'hidden',
        collections: 'visible',
        last_active: 'hidden',
        profile_sections: 'visible',
      },
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useUpdatePrivacy(), {
      wrapper: createWrapper(),
    })

    const privacyInput = {
      saved_shows: 'hidden' as const,
      last_active: 'hidden' as const,
    }

    await act(async () => {
      await result.current.mutateAsync(privacyInput)
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/auth/profile/privacy', {
      method: 'PATCH',
      body: JSON.stringify(privacyInput),
    })
    expect(mockInvalidateOwnContributor).toHaveBeenCalled()
    expect(mockInvalidateContributor).toHaveBeenCalled()
  })
})

describe('useCreateSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateOwnContributor.mockReset()
    mockInvalidateContributor.mockReset()
  })

  it('creates a new section and invalidates queries', async () => {
    const mockResponse = {
      id: 3,
      title: 'New Section',
      content: 'Some content',
      position: 2,
      is_visible: true,
      created_at: '2025-03-15T00:00:00Z',
      updated_at: '2025-03-15T00:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useCreateSection(), {
      wrapper: createWrapper(),
    })

    const input = {
      title: 'New Section',
      content: 'Some content',
      position: 2,
    }

    await act(async () => {
      await result.current.mutateAsync(input)
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/auth/profile/sections', {
      method: 'POST',
      body: JSON.stringify(input),
    })
    expect(mockInvalidateOwnContributor).toHaveBeenCalled()
    expect(mockInvalidateContributor).toHaveBeenCalled()
  })

  it('handles creation errors', async () => {
    const error = new Error('Validation failed')
    Object.assign(error, { status: 422 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCreateSection(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({
          title: '',
          content: '',
          position: 0,
        })
      } catch (e) {
        expect((e as Error).message).toBe('Validation failed')
      }
    })

    expect(mockInvalidateOwnContributor).not.toHaveBeenCalled()
  })
})

describe('useUpdateSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateOwnContributor.mockReset()
    mockInvalidateContributor.mockReset()
  })

  it('updates a section and invalidates queries', async () => {
    const mockResponse = {
      id: 1,
      title: 'Updated Title',
      content: 'Updated content',
      position: 0,
      is_visible: true,
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-03-15T00:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useUpdateSection(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync({
        sectionId: 1,
        data: { title: 'Updated Title', content: 'Updated content' },
      })
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/auth/profile/sections/1', {
      method: 'PUT',
      body: JSON.stringify({ title: 'Updated Title', content: 'Updated content' }),
    })
    expect(mockInvalidateOwnContributor).toHaveBeenCalled()
    expect(mockInvalidateContributor).toHaveBeenCalled()
  })

  it('handles section not found error', async () => {
    const error = new Error('Section not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useUpdateSection(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({
          sectionId: 999,
          data: { title: 'Test' },
        })
      } catch (e) {
        expect((e as Error).message).toBe('Section not found')
      }
    })

    expect(mockInvalidateOwnContributor).not.toHaveBeenCalled()
  })
})

describe('useDeleteSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateOwnContributor.mockReset()
    mockInvalidateContributor.mockReset()
  })

  it('deletes a section and invalidates queries', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useDeleteSection(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync(5)
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/auth/profile/sections/5', {
      method: 'DELETE',
    })
    expect(mockInvalidateOwnContributor).toHaveBeenCalled()
    expect(mockInvalidateContributor).toHaveBeenCalled()
  })

  it('handles deletion errors', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useDeleteSection(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync(999)
      } catch (e) {
        expect((e as Error).message).toBe('Forbidden')
      }
    })

    expect(mockInvalidateOwnContributor).not.toHaveBeenCalled()
  })
})
