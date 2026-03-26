import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShows = vi.fn()
const mockInvalidateArtists = vi.fn()
const mockInvalidateVenues = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

vi.mock('@/features/shows/api', () => ({
  showEndpoints: {
    UPDATE: (showId: string | number) => `/shows/${showId}`,
  },
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {},
  createInvalidateQueries: () => ({
    shows: mockInvalidateShows,
    artists: mockInvalidateArtists,
    venues: mockInvalidateVenues,
  }),
}))

// Mock showLogger
vi.mock('@/lib/utils/showLogger', () => ({
  showLogger: {
    updateAttempt: vi.fn(),
    updateSuccess: vi.fn(),
    updateFailed: vi.fn(),
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
import { useShowUpdate } from './useShowUpdate'
import type { ShowUpdate } from './useShowUpdate'


describe('useShowUpdate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateArtists.mockReset()
    mockInvalidateVenues.mockReset()
  })

  it('updates a show with correct endpoint and method', async () => {
    const mockResponse = {
      id: 1,
      slug: 'updated-show',
      title: 'Updated Title',
      event_date: '2025-06-15T20:00:00Z',
      status: 'approved',
      venues: [],
      artists: [],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-02T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const updates: ShowUpdate = { title: 'Updated Title' }

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, updates })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/1', {
      method: 'PUT',
      body: JSON.stringify(updates),
    })
    expect(result.current.data?.title).toBe('Updated Title')
  })

  it('invalidates shows, artists, and venues on success', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 1,
      slug: 'show-1',
      title: 'Show',
      event_date: '2025-06-15T20:00:00Z',
      status: 'approved',
      venues: [],
      artists: [],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-02T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
    })

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, updates: { title: 'New' } })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateArtists).toHaveBeenCalled()
    expect(mockInvalidateVenues).toHaveBeenCalled()
  })

  it('sends partial updates correctly', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 5,
      slug: 'show-5',
      title: 'Show',
      event_date: '2025-07-01T19:00:00Z',
      status: 'approved',
      venues: [],
      artists: [],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-02T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
    })

    const partialUpdate: ShowUpdate = {
      event_date: '2025-07-01T19:00:00Z',
      price: 20,
    }

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 5, updates: partialUpdate })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const sentBody = JSON.parse(mockApiRequest.mock.calls[0][1].body)
    expect(sentBody.event_date).toBe('2025-07-01T19:00:00Z')
    expect(sentBody.price).toBe(20)
    expect(sentBody.title).toBeUndefined()
  })

  it('sends venue and artist replacements', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 10,
      slug: 'show-10',
      title: 'Show',
      event_date: '2025-06-15T20:00:00Z',
      status: 'approved',
      venues: [{ id: 2, name: 'New Venue', slug: 'new-venue', city: 'Phoenix', state: 'AZ', verified: true }],
      artists: [{ id: 3, name: 'New Artist', slug: 'new-artist', set_type: 'headliner', position: 0, socials: {} }],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-02T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
    })

    const updates: ShowUpdate = {
      venues: [{ id: 2, name: 'New Venue' }],
      artists: [{ id: 3, name: 'New Artist', is_headliner: true }],
    }

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 10, updates })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const sentBody = JSON.parse(mockApiRequest.mock.calls[0][1].body)
    expect(sentBody.venues).toHaveLength(1)
    expect(sentBody.artists).toHaveLength(1)
    expect(sentBody.artists[0].is_headliner).toBe(true)
  })

  it('returns orphaned artists in response', async () => {
    const mockResponse = {
      id: 10,
      slug: 'show-10',
      title: 'Show',
      event_date: '2025-06-15T20:00:00Z',
      status: 'approved',
      venues: [],
      artists: [],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-02T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
      orphaned_artists: [
        { id: 99, name: 'Orphaned Band', slug: 'orphaned-band' },
      ],
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 10, updates: { artists: [] } })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.orphaned_artists).toHaveLength(1)
    expect(result.current.data?.orphaned_artists?.[0].name).toBe('Orphaned Band')
  })

  it('handles API errors and sets error state', async () => {
    const error = new Error('Show not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 999, updates: { title: 'New' } })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Show not found')
  })

  it('does not invalidate queries on error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, updates: { title: 'New' } })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(mockInvalidateShows).not.toHaveBeenCalled()
    expect(mockInvalidateArtists).not.toHaveBeenCalled()
    expect(mockInvalidateVenues).not.toHaveBeenCalled()
  })

  it('handles 422 validation errors', async () => {
    const error = new Error('expected required property event_date to be present')
    Object.assign(error, { status: 422 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, updates: {} })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('handles network errors', async () => {
    mockApiRequest.mockRejectedValueOnce(new TypeError('Failed to fetch'))

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, updates: { title: 'New' } })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeInstanceOf(TypeError)
  })

  it('updates all fields simultaneously', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 20,
      slug: 'show-20',
      title: 'Full Update',
      event_date: '2025-08-01T21:00:00Z',
      status: 'approved',
      venues: [],
      artists: [],
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-02T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
    })

    const fullUpdate: ShowUpdate = {
      title: 'Full Update',
      event_date: '2025-08-01T21:00:00Z',
      city: 'Tempe',
      state: 'AZ',
      price: 25,
      age_requirement: '18+',
      description: 'Updated description',
      venues: [{ id: 1 }],
      artists: [{ id: 2, is_headliner: true }],
    }

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 20, updates: fullUpdate })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const sentBody = JSON.parse(mockApiRequest.mock.calls[0][1].body)
    expect(sentBody.title).toBe('Full Update')
    expect(sentBody.event_date).toBe('2025-08-01T21:00:00Z')
    expect(sentBody.city).toBe('Tempe')
    expect(sentBody.price).toBe(25)
    expect(sentBody.age_requirement).toBe('18+')
    expect(sentBody.description).toBe('Updated description')
  })
})
