import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShows = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      SHOWS: {
        IMPORT_PREVIEW: '/admin/shows/import/preview',
        IMPORT_CONFIRM: '/admin/shows/import/confirm',
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  createInvalidateQueries: () => ({
    shows: mockInvalidateShows,
  }),
}))

// Import hooks after mocks are set up
import { useShowImportPreview, useShowImportConfirm } from './useShowImport'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useShowImport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
  })

  describe('useShowImportPreview', () => {
    it('previews a show import with base64-encoded content', async () => {
      const mockResponse = {
        show: {
          title: 'Imported Show',
          event_date: '2025-04-15T20:00:00Z',
          city: 'Phoenix',
          state: 'AZ',
          status: 'draft',
        },
        venues: [
          {
            name: 'The Rebel Lounge',
            city: 'Phoenix',
            state: 'AZ',
            existing_id: 1,
            will_create: false,
          },
        ],
        artists: [
          {
            name: 'Test Artist',
            position: 1,
            set_type: 'headliner',
            existing_id: null,
            will_create: true,
          },
        ],
        warnings: [],
        can_import: true,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useShowImportPreview(), {
        wrapper: createWrapper(),
      })

      const markdownContent = '# Test Show\n\nVenue: The Rebel Lounge'

      await act(async () => {
        result.current.mutate(markdownContent)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // Verify base64 encoding
      const expectedBase64 = btoa(markdownContent)
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/import/preview',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ content: expectedBase64 }),
        })
      )
    })

    it('returns preview data with venue and artist matches', async () => {
      const mockResponse = {
        show: {
          title: 'Preview Show',
          event_date: '2025-05-01T19:00:00Z',
          city: 'Tempe',
          state: 'AZ',
          status: 'draft',
        },
        venues: [
          {
            name: 'Yucca Tap Room',
            city: 'Tempe',
            state: 'AZ',
            existing_id: 5,
            will_create: false,
          },
        ],
        artists: [
          {
            name: 'Band A',
            position: 1,
            set_type: 'headliner',
            existing_id: 10,
            will_create: false,
          },
          {
            name: 'Band B',
            position: 2,
            set_type: 'support',
            existing_id: null,
            will_create: true,
          },
        ],
        warnings: ['Band B will be created as a new artist'],
        can_import: true,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useShowImportPreview(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('Test content')
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.venues).toHaveLength(1)
      expect(result.current.data?.artists).toHaveLength(2)
      expect(result.current.data?.warnings).toHaveLength(1)
      expect(result.current.data?.can_import).toBe(true)
    })

    it('returns can_import false with warnings', async () => {
      const mockResponse = {
        show: { title: 'Invalid Show', status: 'draft' },
        venues: [],
        artists: [],
        warnings: ['No venue specified', 'No date specified'],
        can_import: false,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useShowImportPreview(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('Invalid content')
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.can_import).toBe(false)
      expect(result.current.data?.warnings).toHaveLength(2)
    })

    it('handles parsing errors', async () => {
      const error = new Error('Invalid markdown format')
      Object.assign(error, { status: 400 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useShowImportPreview(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('Malformed content')
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe(
        'Invalid markdown format'
      )
    })

    it('handles unauthorized error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useShowImportPreview(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('Test content')
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useShowImportConfirm', () => {
    it('confirms a show import with base64-encoded content', async () => {
      const mockResponse = {
        id: 123,
        title: 'Imported Show',
        event_date: '2025-04-15T20:00:00Z',
        status: 'approved',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useShowImportConfirm(), {
        wrapper: createWrapper(),
      })

      const markdownContent = '# Test Show\n\nVenue: The Rebel Lounge'

      await act(async () => {
        result.current.mutate(markdownContent)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // Verify base64 encoding
      const expectedBase64 = btoa(markdownContent)
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/import/confirm',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ content: expectedBase64 }),
        })
      )
    })

    it('returns created show data', async () => {
      const mockShow = {
        id: 456,
        title: 'New Imported Show',
        event_date: '2025-06-01T20:00:00Z',
        venues: [{ id: 1, name: 'Crescent Ballroom' }],
        artists: [{ id: 1, name: 'Test Artist' }],
        status: 'approved',
      }
      mockApiRequest.mockResolvedValueOnce(mockShow)

      const { result } = renderHook(() => useShowImportConfirm(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('Show content')
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.id).toBe(456)
      expect(result.current.data?.title).toBe('New Imported Show')
    })

    it('invalidates shows on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 789 })

      const queryClient = createTestQueryClient()

      const { result } = renderHook(() => useShowImportConfirm(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate('Test content')
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockInvalidateShows).toHaveBeenCalled()
    })

    it('handles import errors', async () => {
      const error = new Error('Import failed: duplicate show')
      Object.assign(error, { status: 400 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useShowImportConfirm(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('Duplicate content')
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe(
        'Import failed: duplicate show'
      )
    })

    it('handles unauthorized error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useShowImportConfirm(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('Test content')
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('handles server errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useShowImportConfirm(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('Test content')
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
