import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapper, createWrapperWithClient, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShows = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
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
vi.mock('@/lib/queryClient', () => ({
  createInvalidateQueries: () => ({
    shows: mockInvalidateShows,
  }),
}))

// Import hooks after mocks are set up
import { useShowImportPreview, useShowImportConfirm } from './useShowImport'


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

  })
})
