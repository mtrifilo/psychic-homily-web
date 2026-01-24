import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'
import { ShowErrorCode } from '../errors'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShows = vi.fn()
const mockInvalidateArtists = vi.fn()
const mockInvalidateVenues = vi.fn()
const mockInvalidateSavedShows = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    SHOWS: {
      SUBMIT: '/shows',
      UPDATE: (id: number) => `/shows/${id}`,
      DELETE: (id: number) => `/shows/${id}`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the show logger
vi.mock('../utils/showLogger', () => ({
  showLogger: {
    submitAttempt: vi.fn(),
    submitSuccess: vi.fn(),
    submitFailed: vi.fn(),
    updateAttempt: vi.fn(),
    updateSuccess: vi.fn(),
    updateFailed: vi.fn(),
    deleteAttempt: vi.fn(),
    deleteSuccess: vi.fn(),
    deleteFailed: vi.fn(),
  },
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  createInvalidateQueries: () => ({
    shows: mockInvalidateShows,
    artists: mockInvalidateArtists,
    venues: mockInvalidateVenues,
    savedShows: mockInvalidateSavedShows,
  }),
}))

// Import hooks after mocks are set up
import { useShowSubmit } from './useShowSubmit'
import { useShowUpdate } from './useShowUpdate'
import { useShowDelete } from './useShowDelete'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useShowSubmit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateArtists.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('submits a show with correct payload', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 123,
      title: 'Test Show',
      event_date: '2025-03-15T20:00:00Z',
    })

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    const submission = {
      event_date: '2025-03-15T20:00:00Z',
      city: 'Phoenix',
      state: 'AZ',
      venues: [{ name: 'The Rebel Lounge', city: 'Phoenix', state: 'AZ' }],
      artists: [{ name: 'Test Artist', is_headliner: true }],
    }

    await act(async () => {
      result.current.mutate(submission)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify(submission),
      })
    )
  })

  it('returns show data on successful submission', async () => {
    const responseData = {
      id: 456,
      title: 'New Show',
      event_date: '2025-04-01T19:00:00Z',
      request_id: 'req-123',
    }
    mockApiRequest.mockResolvedValueOnce(responseData)

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        event_date: '2025-04-01T19:00:00Z',
        city: 'Tempe',
        state: 'AZ',
        venues: [{ name: 'Yucca Tap Room', city: 'Tempe', state: 'AZ' }],
        artists: [{ name: 'Band Name' }],
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.id).toBe(456)
  })

  it('invalidates queries on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 789 })

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({
        event_date: '2025-05-01T20:00:00Z',
        city: 'Mesa',
        state: 'AZ',
        venues: [{ name: 'Nile Theater', city: 'Mesa', state: 'AZ' }],
        artists: [{ name: 'Artist' }],
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateArtists).toHaveBeenCalled()
    expect(mockInvalidateSavedShows).toHaveBeenCalled()
  })

  it('handles submission errors', async () => {
    const error = new Error('Validation failed')
    Object.assign(error, { status: 400 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        event_date: '2025-03-15T20:00:00Z',
        city: 'Phoenix',
        state: 'AZ',
        venues: [],
        artists: [],
      })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('supports private show submission', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 100, is_private: true })

    const { result } = renderHook(() => useShowSubmit(), {
      wrapper: createWrapper(),
    })

    const submission = {
      event_date: '2025-03-15T20:00:00Z',
      city: 'Phoenix',
      state: 'AZ',
      venues: [{ name: 'House Show', city: 'Phoenix', state: 'AZ' }],
      artists: [{ name: 'Local Band' }],
      is_private: true,
    }

    await act(async () => {
      result.current.mutate(submission)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows',
      expect.objectContaining({
        body: expect.stringContaining('"is_private":true'),
      })
    )
  })
})

describe('useShowUpdate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateArtists.mockReset()
    mockInvalidateVenues.mockReset()
  })

  it('updates a show with correct endpoint and payload', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 123,
      title: 'Updated Show',
    })

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 123,
        updates: {
          title: 'Updated Show',
          description: 'New description',
        },
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/123',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({
          title: 'Updated Show',
          description: 'New description',
        }),
      })
    )
  })

  it('returns updated show data', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 456,
      title: 'Modified Title',
      event_date: '2025-06-01T21:00:00Z',
    })

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 456,
        updates: { title: 'Modified Title' },
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.title).toBe('Modified Title')
  })

  it('invalidates shows, artists, and venues on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 789 })

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({
        showId: 789,
        updates: { price: 25 },
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateArtists).toHaveBeenCalled()
    expect(mockInvalidateVenues).toHaveBeenCalled()
  })

  it('handles update errors', async () => {
    const error = new Error('Show not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 999,
        updates: { title: 'New Title' },
      })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('supports partial updates', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 100, price: 15 })

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 100,
        updates: { price: 15 },
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Should only include price in the payload
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/100',
      expect.objectContaining({
        body: JSON.stringify({ price: 15 }),
      })
    )
  })

  it('can update venues and artists', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 200 })

    const { result } = renderHook(() => useShowUpdate(), {
      wrapper: createWrapper(),
    })

    const updates = {
      venues: [{ id: 1, name: 'New Venue' }],
      artists: [{ id: 5, is_headliner: true }],
    }

    await act(async () => {
      result.current.mutate({
        showId: 200,
        updates,
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/200',
      expect.objectContaining({
        body: JSON.stringify(updates),
      })
    )
  })
})

describe('useShowDelete', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('deletes a show with correct endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useShowDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(123)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/123',
      expect.objectContaining({
        method: 'DELETE',
      })
    )
  })

  it('invalidates shows and savedShows on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useShowDelete(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate(456)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateSavedShows).toHaveBeenCalled()
  })

  it('handles delete errors', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(789)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('handles not found errors', async () => {
    const error = new Error('Show not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(999)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Show not found')
  })
})
