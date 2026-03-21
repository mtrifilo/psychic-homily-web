import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

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
    },
    CONTRIBUTOR: {
      OWN_PROFILE: '/auth/profile/contributor',
      OWN_CONTRIBUTIONS: '/auth/profile/contributions',
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
      contributions: (username: string) => ['contributor', 'contributions', username],
      ownContributions: ['contributor', 'ownContributions'],
      ownSections: ['contributor', 'ownSections'],
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
  useOwnContributorProfile,
  useOwnContributions,
  useOwnSections,
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
    expect(result.current.data?.username).toBe('testuser')
    expect(result.current.data?.stats?.shows_submitted).toBe(42)
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
    expect(result.current.data?.contributions).toHaveLength(1)
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

  it('handles API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(
      () => usePublicContributions('testuser'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isError).toBe(true))
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
        attendance: 'visible',
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
    expect(result.current.data?.username).toBe('myuser')
    expect(result.current.data?.privacy_settings?.saved_shows).toBe('count_only')
  })

  it('handles unauthorized error', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useOwnContributorProfile(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
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
    expect(result.current.data?.sections).toHaveLength(2)
    expect(result.current.data?.sections[0].title).toBe('About Me')
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
      const data = await result.current.mutateAsync({ visibility: 'private' })
      expect(data.profile_visibility).toBe('private')
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
        attendance: 'count_only',
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
      attendance: 'count_only' as const,
      last_active: 'hidden' as const,
    }

    await act(async () => {
      const data = await result.current.mutateAsync(privacyInput)
      expect(data.privacy_settings?.saved_shows).toBe('hidden')
      expect(data.privacy_settings?.last_active).toBe('hidden')
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
      const data = await result.current.mutateAsync(input)
      expect(data.id).toBe(3)
      expect(data.title).toBe('New Section')
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
      const data = await result.current.mutateAsync({
        sectionId: 1,
        data: { title: 'Updated Title', content: 'Updated content' },
      })
      expect(data.title).toBe('Updated Title')
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
