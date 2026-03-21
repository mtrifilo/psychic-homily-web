import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShows = vi.fn()
const mockInvalidateArtists = vi.fn()
const mockInvalidateSavedShows = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    SHOWS: {
      SUBMIT: '/shows',
    },
  },
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {},
  createInvalidateQueries: () => ({
    shows: mockInvalidateShows,
    artists: mockInvalidateArtists,
    savedShows: mockInvalidateSavedShows,
  }),
}))

// Mock showLogger to suppress console output in tests
vi.mock('@/lib/utils/showLogger', () => ({
  showLogger: {
    submitAttempt: vi.fn(),
    submitSuccess: vi.fn(),
    submitFailed: vi.fn(),
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  },
}))

// Mock errors module
vi.mock('@/lib/errors', () => ({
  ShowError: {
    fromUnknown: (error: unknown) => ({
      code: 'UNKNOWN',
      message: error instanceof Error ? error.message : String(error),
      requestId: undefined,
    }),
  },
  ShowErrorCode: {
    UNKNOWN: 'UNKNOWN',
  },
}))

// Import hooks after mocks are set up
import { useShowSubmit } from './useShowSubmit'
import type { ShowSubmission } from './useShowSubmit'


const validSubmission: ShowSubmission = {
  event_date: '2025-06-15T20:00:00Z',
  city: 'Phoenix',
  state: 'AZ',
  venues: [{ name: 'The Rebel Lounge', city: 'Phoenix', state: 'AZ' }],
  artists: [{ name: 'Test Artist' }],
}

describe('useShowSubmit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateArtists.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('submits a show with correct endpoint and method', async () => {
    const mockResponse = {
      id: 1,
      slug: 'test-artist-rebel-lounge-2025-06-15',
      title: 'Test Artist at The Rebel Lounge',
      event_date: '2025-06-15T20:00:00Z',
      status: 'pending',
      venues: [{ id: 1, name: 'The Rebel Lounge', slug: 'the-rebel-lounge', city: 'Phoenix', state: 'AZ', verified: false }],
      artists: [{ id: 1, name: 'Test Artist', slug: 'test-artist', set_type: 'performer', position: 0, socials: {} }],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-01T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(validSubmission)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows', {
      method: 'POST',
      body: JSON.stringify(validSubmission),
    })
    expect(result.current.data?.id).toBe(1)
  })

  it('invalidates shows, artists, and savedShows on success', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 2,
      slug: 'show-2',
      title: 'Show',
      event_date: '2025-06-15T20:00:00Z',
      status: 'pending',
      venues: [],
      artists: [],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-01T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
    })

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(validSubmission)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateArtists).toHaveBeenCalled()
    expect(mockInvalidateSavedShows).toHaveBeenCalled()
  })

  it('returns the show response data on success', async () => {
    const mockResponse = {
      id: 42,
      slug: 'test-show',
      title: 'Test Show',
      event_date: '2025-06-15T20:00:00Z',
      status: 'approved',
      venues: [{ id: 5, name: 'Valley Bar', slug: 'valley-bar', city: 'Phoenix', state: 'AZ', verified: true }],
      artists: [
        { id: 10, name: 'Band A', slug: 'band-a', is_headliner: true, set_type: 'headliner', position: 0, socials: {} },
        { id: 11, name: 'Band B', slug: 'band-b', set_type: 'opener', position: 1, socials: {} },
      ],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-01T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
      request_id: 'req-abc-123',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(validSubmission)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.id).toBe(42)
    expect(result.current.data?.title).toBe('Test Show')
    expect(result.current.data?.venues).toHaveLength(1)
    expect(result.current.data?.artists).toHaveLength(2)
  })

  it('handles API errors and sets error state', async () => {
    const error = new Error('Validation failed')
    Object.assign(error, { status: 422 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(validSubmission)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
    expect((result.current.error as Error).message).toBe('Validation failed')
  })

  it('does not invalidate queries on error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(validSubmission)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(mockInvalidateShows).not.toHaveBeenCalled()
    expect(mockInvalidateArtists).not.toHaveBeenCalled()
    expect(mockInvalidateSavedShows).not.toHaveBeenCalled()
  })

  it('sends optional fields when provided', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 3,
      slug: 'show-3',
      title: 'Private Show',
      event_date: '2025-06-15T20:00:00Z',
      status: 'private',
      venues: [],
      artists: [],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-01T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
    })

    const fullSubmission: ShowSubmission = {
      title: 'Private Show',
      event_date: '2025-06-15T20:00:00Z',
      city: 'Phoenix',
      state: 'AZ',
      price: 15,
      age_requirement: '21+',
      description: 'A test show',
      venues: [{ name: 'Valley Bar', city: 'Phoenix', state: 'AZ', address: '130 N Central Ave' }],
      artists: [
        { name: 'Headliner', is_headliner: true },
        { name: 'Opener', is_headliner: false, instagram_handle: '@opener' },
      ],
      is_private: true,
    }

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(fullSubmission)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const sentBody = JSON.parse(mockApiRequest.mock.calls[0][1].body)
    expect(sentBody.title).toBe('Private Show')
    expect(sentBody.price).toBe(15)
    expect(sentBody.age_requirement).toBe('21+')
    expect(sentBody.description).toBe('A test show')
    expect(sentBody.is_private).toBe(true)
    expect(sentBody.artists).toHaveLength(2)
    expect(sentBody.venues[0].address).toBe('130 N Central Ave')
  })

  it('submits with multiple venues and artists', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 4,
      slug: 'show-4',
      title: 'Multi-venue Show',
      event_date: '2025-06-15T20:00:00Z',
      status: 'pending',
      venues: [],
      artists: [],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-01T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
    })

    const multiSubmission: ShowSubmission = {
      event_date: '2025-06-15T20:00:00Z',
      city: 'Phoenix',
      state: 'AZ',
      venues: [
        { name: 'Venue A', city: 'Phoenix', state: 'AZ' },
        { name: 'Venue B', id: 5, city: 'Phoenix', state: 'AZ' },
      ],
      artists: [
        { name: 'Artist A', is_headliner: true },
        { name: 'Artist B', id: 10 },
        { name: 'Artist C' },
      ],
    }

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(multiSubmission)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const sentBody = JSON.parse(mockApiRequest.mock.calls[0][1].body)
    expect(sentBody.venues).toHaveLength(2)
    expect(sentBody.artists).toHaveLength(3)
  })

  it('handles network errors', async () => {
    mockApiRequest.mockRejectedValueOnce(new TypeError('Failed to fetch'))

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(validSubmission)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeInstanceOf(TypeError)
  })

  it('handles 401 unauthorized errors', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(validSubmission)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Unauthorized')
  })
})
